package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/webappsgo/caslink/src/server/store"
)

// newWebAuthnTestService builds a WebAuthnService backed by a full-schema
// in-memory store, ready for direct exercise of its DB-facing methods
// (saveCredential/loadCredentials/GetCredentials/DeleteCredential/recovery
// keys) without performing a real browser WebAuthn ceremony.
func newWebAuthnTestService(t *testing.T) (*WebAuthnService, *store.Store) {
	t.Helper()

	st := newFullSchemaStore(t)
	svc, err := NewWebAuthnService(st, "example.com", "https://example.com")
	if err != nil {
		t.Fatalf("NewWebAuthnService failed: %v", err)
	}
	return svc, st
}

// insertUser inserts a minimal row into the users table so foreign-key
// constrained inserts into passkey_credentials/recovery_keys succeed.
func insertUser(t *testing.T, st *store.Store, id, username string) {
	t.Helper()
	_, err := st.UsersDB.Exec(
		`INSERT INTO users (id, username, email, password_hash, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		id, username, username+"@example.com", "argon2id$dummy",
	)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
}

// newFakeCredential builds a minimal webauthn.Credential with the given raw
// ID and sign count, suitable for exercising saveCredential/loadCredentials
// round-tripping without a real attestation ceremony.
func newFakeCredential(rawID []byte, signCount uint32) *webauthn.Credential {
	return &webauthn.Credential{
		ID:              rawID,
		PublicKey:       []byte("fake-cbor-public-key-bytes"),
		AttestationType: "none",
		Authenticator: webauthn.Authenticator{
			AAGUID:    []byte{1, 2, 3, 4},
			SignCount: signCount,
		},
	}
}

// TestSaveAndLoadCredentialRoundTrip verifies a saved credential can be
// reloaded via loadCredentials with the same ID and sign count, and shows
// up through the public GetCredentials API with correct field mapping.
func TestSaveAndLoadCredentialRoundTrip(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	cred := newFakeCredential([]byte("credential-id-one"), 0)
	if err := svc.saveCredential("1", "My Laptop", cred); err != nil {
		t.Fatalf("saveCredential failed: %v", err)
	}

	loaded, err := svc.loadCredentials("1")
	if err != nil {
		t.Fatalf("loadCredentials failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loadCredentials returned %d credentials, want 1", len(loaded))
	}
	if string(loaded[0].ID) != string(cred.ID) {
		t.Errorf("loaded credential ID = %q, want %q", loaded[0].ID, cred.ID)
	}
	if loaded[0].Authenticator.SignCount != 0 {
		t.Errorf("loaded SignCount = %d, want 0", loaded[0].Authenticator.SignCount)
	}

	list, err := svc.GetCredentials("1")
	if err != nil {
		t.Fatalf("GetCredentials failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("GetCredentials returned %d records, want 1", len(list))
	}
	if list[0].Name != "My Laptop" {
		t.Errorf("Name = %q, want %q", list[0].Name, "My Laptop")
	}
	if list[0].UserID != "1" {
		t.Errorf("UserID = %q, want %q", list[0].UserID, "1")
	}
	wantCredID := base64.URLEncoding.EncodeToString(cred.ID)
	if list[0].CredentialID != wantCredID {
		t.Errorf("CredentialID = %q, want %q", list[0].CredentialID, wantCredID)
	}
	if list[0].LastUsed != nil {
		t.Errorf("LastUsed = %v, want nil for a never-used credential", list[0].LastUsed)
	}
}

// TestSaveDuplicateCredentialIDRejected verifies the credential_id UNIQUE
// constraint actually rejects registering the same authenticator (same raw
// credential ID) twice, even for two different users — the DB layer is the
// last line of defense against a duplicate/replayed registration slipping
// through the exclusion-list check in BeginRegistration.
func TestSaveDuplicateCredentialIDRejected(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")
	insertUser(t, st, "2", "bob")

	cred := newFakeCredential([]byte("shared-credential-id"), 0)
	if err := svc.saveCredential("1", "Device A", cred); err != nil {
		t.Fatalf("first saveCredential failed: %v", err)
	}

	dup := newFakeCredential([]byte("shared-credential-id"), 0)
	if err := svc.saveCredential("2", "Device B", dup); err == nil {
		t.Fatal("expected saveCredential to reject a duplicate credential_id, got nil error")
	}
}

// TestFinishLoginRejectsNonIncreasingSignCount is the core replay-attack
// regression test: per WebAuthn §7.2 step 17, an assertion whose sign
// counter is less than or equal to the previously stored value is the
// spec-defined signal for a cloned/replayed authenticator, and must be
// rejected rather than silently accepted and persisted.
func TestFinishLoginRejectsNonIncreasingSignCount(t *testing.T) {
	cases := []struct {
		name        string
		storedCount uint32
		assertCount uint32
		wantReject  bool
	}{
		{"strictly greater counter is accepted", 5, 6, false},
		{"equal counter is replay and rejected", 5, 5, true},
		{"lower counter is replay and rejected", 5, 3, true},
		{"both zero (authenticator without a counter) is accepted", 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred := &webauthn.Credential{
				ID: []byte("cred-under-test"),
				Authenticator: webauthn.Authenticator{
					SignCount: tc.storedCount,
				},
			}

			// UpdateCounter is the exact library call the go-webauthn
			// FinishLogin ceremony performs internally to compare the
			// authenticator's reported counter against the stored value.
			cred.Authenticator.UpdateCounter(tc.assertCount)

			err := cloneWarningError(cred)
			gotReject := err != nil
			if gotReject != tc.wantReject {
				t.Errorf("cloneWarningError() rejected=%v, want %v (err=%v)", gotReject, tc.wantReject, err)
			}
		})
	}
}

// TestUpdateCredentialAfterLoginPersistsIncreasedSignCount verifies the
// legitimate (non-replay) path: after a genuine login with a strictly
// higher sign count, the stored row's sign_count and last_used are updated.
func TestUpdateCredentialAfterLoginPersistsIncreasedSignCount(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	cred := newFakeCredential([]byte("cred-x"), 1)
	if err := svc.saveCredential("1", "Phone", cred); err != nil {
		t.Fatalf("saveCredential failed: %v", err)
	}

	updated := newFakeCredential([]byte("cred-x"), 2)
	if err := svc.updateCredentialAfterLogin("1", updated); err != nil {
		t.Fatalf("updateCredentialAfterLogin failed: %v", err)
	}

	list, err := svc.GetCredentials("1")
	if err != nil {
		t.Fatalf("GetCredentials failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(list))
	}
	if list[0].SignCount != 2 {
		t.Errorf("SignCount = %d, want 2", list[0].SignCount)
	}
	if list[0].LastUsed == nil {
		t.Error("expected LastUsed to be set after a login update")
	}
}

// TestGetCredentialsScopedToUser verifies credentials belonging to one user
// are never returned when querying another user's credential list.
func TestGetCredentialsScopedToUser(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")
	insertUser(t, st, "2", "bob")

	if err := svc.saveCredential("1", "Alice Key", newFakeCredential([]byte("cred-alice"), 0)); err != nil {
		t.Fatalf("saveCredential (alice) failed: %v", err)
	}
	if err := svc.saveCredential("2", "Bob Key", newFakeCredential([]byte("cred-bob"), 0)); err != nil {
		t.Fatalf("saveCredential (bob) failed: %v", err)
	}

	aliceCreds, err := svc.GetCredentials("1")
	if err != nil {
		t.Fatalf("GetCredentials(user-1) failed: %v", err)
	}
	if len(aliceCreds) != 1 || aliceCreds[0].Name != "Alice Key" {
		t.Fatalf("GetCredentials(user-1) = %+v, want exactly Alice's key", aliceCreds)
	}

	bobCreds, err := svc.GetCredentials("2")
	if err != nil {
		t.Fatalf("GetCredentials(user-2) failed: %v", err)
	}
	if len(bobCreds) != 1 || bobCreds[0].Name != "Bob Key" {
		t.Fatalf("GetCredentials(user-2) = %+v, want exactly Bob's key", bobCreds)
	}
}

// TestDeleteCredential verifies successful deletion of an owned credential,
// that the deletion is scoped to the owning user (ownership enforcement),
// and that deleting a nonexistent record returns an error.
func TestDeleteCredential(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")
	insertUser(t, st, "2", "bob")

	if err := svc.saveCredential("1", "Alice Key", newFakeCredential([]byte("cred-alice"), 0)); err != nil {
		t.Fatalf("saveCredential failed: %v", err)
	}

	creds, err := svc.GetCredentials("1")
	if err != nil {
		t.Fatalf("GetCredentials failed: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential before delete, got %d", len(creds))
	}
	recordID := creds[0].ID

	t.Run("wrong owner cannot delete", func(t *testing.T) {
		if err := svc.DeleteCredential("2", recordID); err == nil {
			t.Error("expected DeleteCredential to fail when the requester does not own the credential")
		}
	})

	t.Run("owner can delete", func(t *testing.T) {
		if err := svc.DeleteCredential("1", recordID); err != nil {
			t.Fatalf("DeleteCredential failed for owner: %v", err)
		}
		remaining, err := svc.GetCredentials("1")
		if err != nil {
			t.Fatalf("GetCredentials after delete failed: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("expected 0 credentials after delete, got %d", len(remaining))
		}
	})

	t.Run("deleting a nonexistent record errors", func(t *testing.T) {
		if err := svc.DeleteCredential("1", recordID); err == nil {
			t.Error("expected DeleteCredential to fail for an already-deleted record")
		}
	})
}

// TestGenerateRecoveryKeysCountAndFormat verifies exactly 10 unique keys are
// generated in the documented "{8-hex}-{4-hex}" format.
func TestGenerateRecoveryKeysCountAndFormat(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	keys, err := svc.GenerateRecoveryKeys("1")
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys failed: %v", err)
	}
	if len(keys) != recoveryKeyCount {
		t.Fatalf("got %d keys, want %d", len(keys), recoveryKeyCount)
	}

	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate recovery key generated: %q", k)
		}
		seen[k] = true
		if len(k) != 13 || k[8] != '-' {
			t.Errorf("key %q does not match the {8-hex}-{4-hex} format", k)
		}
	}

	count, err := svc.GetRecoveryKeyCount("1")
	if err != nil {
		t.Fatalf("GetRecoveryKeyCount failed: %v", err)
	}
	if count != recoveryKeyCount {
		t.Errorf("GetRecoveryKeyCount = %d, want %d", count, recoveryKeyCount)
	}
}

// TestGenerateRecoveryKeysReplacesUnusedKeys verifies a second call to
// GenerateRecoveryKeys invalidates the previous unused batch (old keys no
// longer validate) rather than accumulating keys forever.
func TestGenerateRecoveryKeysReplacesUnusedKeys(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	firstBatch, err := svc.GenerateRecoveryKeys("1")
	if err != nil {
		t.Fatalf("first GenerateRecoveryKeys failed: %v", err)
	}

	if _, err := svc.GenerateRecoveryKeys("1"); err != nil {
		t.Fatalf("second GenerateRecoveryKeys failed: %v", err)
	}

	ok, err := svc.ValidateRecoveryKey("1", firstBatch[0])
	if err != nil {
		t.Fatalf("ValidateRecoveryKey failed: %v", err)
	}
	if ok {
		t.Error("expected a key from the replaced first batch to no longer validate")
	}

	count, err := svc.GetRecoveryKeyCount("1")
	if err != nil {
		t.Fatalf("GetRecoveryKeyCount failed: %v", err)
	}
	if count != recoveryKeyCount {
		t.Errorf("GetRecoveryKeyCount = %d, want %d (only the second batch should remain)", count, recoveryKeyCount)
	}
}

// TestValidateRecoveryKeySingleUse is the core single-use invariant test:
// a valid recovery key succeeds exactly once; reusing it afterward (a
// replay of a consumed recovery key) must fail.
func TestValidateRecoveryKeySingleUse(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	keys, err := svc.GenerateRecoveryKeys("1")
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys failed: %v", err)
	}
	target := keys[3]

	ok, err := svc.ValidateRecoveryKey("1", target)
	if err != nil {
		t.Fatalf("ValidateRecoveryKey (first use) failed: %v", err)
	}
	if !ok {
		t.Fatal("expected first use of a fresh recovery key to succeed")
	}

	remaining, err := svc.GetRecoveryKeyCount("1")
	if err != nil {
		t.Fatalf("GetRecoveryKeyCount failed: %v", err)
	}
	if remaining != recoveryKeyCount-1 {
		t.Errorf("remaining unused keys = %d, want %d", remaining, recoveryKeyCount-1)
	}

	ok, err = svc.ValidateRecoveryKey("1", target)
	if err != nil {
		t.Fatalf("ValidateRecoveryKey (replay) failed: %v", err)
	}
	if ok {
		t.Error("expected a already-consumed recovery key to be rejected on reuse")
	}
}

// TestValidateRecoveryKeyCaseInsensitiveAndInvalid verifies the documented
// case-insensitive comparison, whitespace trimming, and outright-invalid
// key rejection without erroring.
func TestValidateRecoveryKeyCaseInsensitiveAndInvalid(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	keys, err := svc.GenerateRecoveryKeys("1")
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys failed: %v", err)
	}

	t.Run("uppercase and padded whitespace still validate", func(t *testing.T) {
		mangled := "  " + strings.ToUpper(keys[0]) + "  "
		ok, err := svc.ValidateRecoveryKey("1", mangled)
		if err != nil {
			t.Fatalf("ValidateRecoveryKey failed: %v", err)
		}
		if !ok {
			t.Error("expected uppercase/whitespace-padded key to still validate")
		}
	})

	t.Run("completely invalid key is rejected without error", func(t *testing.T) {
		ok, err := svc.ValidateRecoveryKey("1", "not-a-real-key")
		if err != nil {
			t.Fatalf("ValidateRecoveryKey returned an error for a garbage key: %v", err)
		}
		if ok {
			t.Error("expected a garbage recovery key to be rejected")
		}
	})

	t.Run("valid key for a different user is rejected", func(t *testing.T) {
		insertUser(t, st, "2", "bob")
		ok, err := svc.ValidateRecoveryKey("2", keys[1])
		if err != nil {
			t.Fatalf("ValidateRecoveryKey failed: %v", err)
		}
		if ok {
			t.Error("expected another user's recovery key to be rejected")
		}
	})
}

// TestStoreAndLoadWebAuthnSession verifies ceremony session round-tripping:
// StoreSession followed by LoadSession returns the same data exactly once
// (single-use, since LoadSession deletes on read), and an expired session
// is treated as absent.
func TestStoreAndLoadWebAuthnSession(t *testing.T) {
	svc, _ := newWebAuthnTestService(t)

	data := &webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("1"),
	}

	sessID, err := svc.StoreSession("1", data)
	if err != nil {
		t.Fatalf("StoreSession failed: %v", err)
	}
	if sessID == "" {
		t.Fatal("expected a non-empty session ID")
	}

	loaded, userID, err := svc.LoadSession(sessID)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loaded == nil || loaded.Challenge != "test-challenge" {
		t.Fatalf("LoadSession returned %+v, want challenge %q", loaded, "test-challenge")
	}
	if userID != "1" {
		t.Errorf("userID = %q, want %q", userID, "1")
	}

	// Session must be single-use: loading it again must return nothing.
	second, _, err := svc.LoadSession(sessID)
	if err != nil {
		t.Fatalf("second LoadSession failed: %v", err)
	}
	if second != nil {
		t.Error("expected LoadSession to be single-use (nil on second call)")
	}
}

// TestLoadSessionUnknownIDReturnsNil verifies looking up a session ID that
// was never stored returns a nil result rather than an error.
func TestLoadSessionUnknownIDReturnsNil(t *testing.T) {
	svc, _ := newWebAuthnTestService(t)

	data, userID, err := svc.LoadSession("does-not-exist")
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if data != nil || userID != "" {
		t.Errorf("LoadSession(unknown) = (%+v, %q), want (nil, \"\")", data, userID)
	}
}

// TestPruneExpiredSessions verifies expired ceremony sessions are dropped
// on the next StoreSession call (which triggers pruning) and are not
// returned by LoadSession.
func TestPruneExpiredSessions(t *testing.T) {
	svc, _ := newWebAuthnTestService(t)

	svc.sessMu.Lock()
	svc.sessions["expired-session"] = &webauthnSessionEntry{
		SessionData: &webauthn.SessionData{Challenge: "stale"},
		UserID:      "1",
		Expiry:      time.Now().Add(-time.Minute),
	}
	svc.sessMu.Unlock()

	// StoreSession runs pruneExpiredSessions internally before inserting.
	if _, err := svc.StoreSession("2", &webauthn.SessionData{Challenge: "fresh"}); err != nil {
		t.Fatalf("StoreSession failed: %v", err)
	}

	data, _, err := svc.LoadSession("expired-session")
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if data != nil {
		t.Error("expected the expired session to have been pruned")
	}
}

// TestLoadCredentialsDeserialisesJSON confirms loadCredentials correctly
// round-trips the JSON blob stored in public_key by directly inserting a
// raw row, guarding against any drift between saveCredential's json.Marshal
// and loadCredentials' json.Unmarshal expectations.
func TestLoadCredentialsDeserialisesJSON(t *testing.T) {
	svc, st := newWebAuthnTestService(t)
	insertUser(t, st, "1", "alice")

	cred := newFakeCredential([]byte("raw-insert-cred"), 7)
	blob, err := json.Marshal(cred)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	_, err = st.UsersDB.Exec(
		`INSERT INTO passkey_credentials
		 (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"row-1", "1", base64.URLEncoding.EncodeToString(cred.ID), blob,
		"none", "", 7, "Manually Inserted",
	)
	if err != nil {
		t.Fatalf("manual insert failed: %v", err)
	}

	loaded, err := svc.loadCredentials("1")
	if err != nil {
		t.Fatalf("loadCredentials failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(loaded))
	}
	if loaded[0].Authenticator.SignCount != 7 {
		t.Errorf("SignCount = %d, want 7", loaded[0].Authenticator.SignCount)
	}
}
