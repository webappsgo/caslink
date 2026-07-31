package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	appcrypto "github.com/webappsgo/caslink/src/common/crypto"
)

// computeTOTPCode reimplements RFC 6238/4226 independently of
// generateTOTPCode so the "valid code accepted" test exercises real crypto
// against an independently derived expected value, not a copy of the
// production algorithm.
func computeTOTPCode(t *testing.T, secretB32 string, timeStep int64) string {
	t.Helper()
	secretBytes, err := base32.StdEncoding.DecodeString(strings.ToUpper(secretB32))
	if err != nil {
		t.Fatalf("failed to decode secret: %v", err)
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(timeStep))
	h := hmac.New(sha1.New, secretBytes)
	h.Write(buf)
	hash := h.Sum(nil)
	offset := hash[len(hash)-1] & 0x0F
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7FFFFFFF
	return fmt.Sprintf("%06d", truncated%1000000)
}

// testEncryptionKey returns a real 32-byte AES-256-GCM key via the same
// generate/decode path production config loading uses.
func testEncryptionKey(t *testing.T) []byte {
	t.Helper()
	encoded, err := appcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("appcrypto.GenerateKey failed: %v", err)
	}
	key, err := appcrypto.DecodeKey(encoded)
	if err != nil {
		t.Fatalf("appcrypto.DecodeKey failed: %v", err)
	}
	return key
}

func TestGenerateTOTPSecret(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	secret, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if strings.Contains(secret, "=") {
		t.Error("secret should have padding stripped")
	}
	if _, err := base32.StdEncoding.DecodeString(secret); err != nil {
		t.Errorf("secret is not valid base32: %v", err)
	}

	secret2, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if secret == secret2 {
		t.Error("expected two calls to produce different secrets")
	}
}

func TestGenerateRecoveryKeys(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	keys, err := svc.GenerateRecoveryKeys()
	if err != nil {
		t.Fatalf("GenerateRecoveryKeys failed: %v", err)
	}
	if len(keys) != 10 {
		t.Fatalf("len(keys) = %d, want 10", len(keys))
	}

	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if len(k) != 13 || k[8] != '-' {
			t.Errorf("key %q does not match {8-hex}-{4-hex} format", k)
		}
		if seen[k] {
			t.Errorf("duplicate recovery key: %q", k)
		}
		seen[k] = true
	}
}

func TestHashAndVerifyRecoveryKey(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	const key = "a1b2c3d4-e5f6"
	hash, err := svc.HashRecoveryKey(key)
	if err != nil {
		t.Fatalf("HashRecoveryKey failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if !svc.VerifyRecoveryKey(key, hash) {
		t.Error("VerifyRecoveryKey returned false for correct key")
	}

	// Case-insensitive per PART 23.
	if !svc.VerifyRecoveryKey(strings.ToUpper(key), hash) {
		t.Error("VerifyRecoveryKey should be case-insensitive")
	}

	// Leading/trailing whitespace is trimmed before comparison.
	if !svc.VerifyRecoveryKey("  "+key+"  ", hash) {
		t.Error("VerifyRecoveryKey should trim surrounding whitespace")
	}

	if svc.VerifyRecoveryKey("wrong-key-000", hash) {
		t.Error("VerifyRecoveryKey returned true for wrong key")
	}
}

func TestGenerateQRCodeURL(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	url := svc.GenerateQRCodeURL("JBSWY3DPEHPK3PXP", "Caslink", "user@example.com")

	if !strings.HasPrefix(url, "otpauth://totp/") {
		t.Errorf("URL %q does not have expected otpauth prefix", url)
	}
	if !strings.Contains(url, "secret=JBSWY3DPEHPK3PXP") {
		t.Errorf("URL %q missing expected secret param", url)
	}
	if !strings.Contains(url, "issuer=Caslink") {
		t.Errorf("URL %q missing expected issuer param", url)
	}
}

func TestVerifyTOTPCode(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	secret, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	currentStep := time.Now().Unix() / 30

	t.Run("current-step code is accepted", func(t *testing.T) {
		code := computeTOTPCode(t, secret, currentStep)
		if !svc.VerifyTOTPCode(secret, code) {
			t.Error("expected current-step code to verify")
		}
	})

	t.Run("code from one step earlier is accepted (clock drift window)", func(t *testing.T) {
		code := computeTOTPCode(t, secret, currentStep-1)
		if !svc.VerifyTOTPCode(secret, code) {
			t.Error("expected code from -1 step to verify within drift window")
		}
	})

	t.Run("code from one step later is accepted (clock drift window)", func(t *testing.T) {
		code := computeTOTPCode(t, secret, currentStep+1)
		if !svc.VerifyTOTPCode(secret, code) {
			t.Error("expected code from +1 step to verify within drift window")
		}
	})

	t.Run("code from two steps earlier is rejected (outside drift window)", func(t *testing.T) {
		code := computeTOTPCode(t, secret, currentStep-2)
		if svc.VerifyTOTPCode(secret, code) {
			t.Error("expected code from -2 steps to be rejected")
		}
	})

	t.Run("wrong code rejected", func(t *testing.T) {
		code := computeTOTPCode(t, secret, currentStep)
		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}
		if svc.VerifyTOTPCode(secret, wrong) {
			t.Error("expected mismatched code to be rejected")
		}
	})

	t.Run("wrong length code rejected", func(t *testing.T) {
		if svc.VerifyTOTPCode(secret, "12345") {
			t.Error("expected 5-digit code to be rejected")
		}
		if svc.VerifyTOTPCode(secret, "1234567") {
			t.Error("expected 7-digit code to be rejected")
		}
	})

	t.Run("empty code rejected", func(t *testing.T) {
		if svc.VerifyTOTPCode(secret, "") {
			t.Error("expected empty code to be rejected")
		}
	})

	t.Run("malformed base32 secret rejected, not a panic", func(t *testing.T) {
		if svc.VerifyTOTPCode("not-valid-base32!!!", "123456") {
			t.Error("expected malformed secret to fail verification")
		}
	})

	t.Run("secret is case-insensitive", func(t *testing.T) {
		code := computeTOTPCode(t, secret, currentStep)
		if !svc.VerifyTOTPCode(strings.ToLower(secret), code) {
			t.Error("expected lowercase secret to still verify")
		}
	})
}

func TestEnableDisableAndHasTOTP(t *testing.T) {
	st := newFullSchemaStore(t)
	key := testEncryptionKey(t)
	svc := NewTOTPService(st, key, 1)

	const userID = int64(1)

	if svc.HasTOTP(userID) {
		t.Error("expected HasTOTP to be false before enabling")
	}

	secret, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}

	recoveryKeys, err := svc.EnableTOTP(userID, secret)
	if err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}
	if len(recoveryKeys) != 10 {
		t.Fatalf("len(recoveryKeys) = %d, want 10", len(recoveryKeys))
	}

	if !svc.HasTOTP(userID) {
		t.Error("expected HasTOTP to be true after enabling")
	}

	t.Run("stored secret round-trips through encryption", func(t *testing.T) {
		got, err := svc.GetTOTPSecret(userID)
		if err != nil {
			t.Fatalf("GetTOTPSecret failed: %v", err)
		}
		if got != secret {
			t.Error("decrypted secret does not match the original secret")
		}
	})

	t.Run("stored secret is not persisted as plaintext when a key is configured", func(t *testing.T) {
		var stored string
		if err := st.UsersDB.QueryRow(
			`SELECT secret FROM totp_secrets WHERE user_type = 'user' AND user_id = ?`, userID,
		).Scan(&stored); err != nil {
			t.Fatalf("failed to read stored secret: %v", err)
		}
		if stored == secret {
			t.Error("secret was stored in plaintext despite an encryption key being configured")
		}
	})

	t.Run("remaining recovery key count starts at 10", func(t *testing.T) {
		n, err := svc.GetRemainingRecoveryKeyCount(userID)
		if err != nil {
			t.Fatalf("GetRemainingRecoveryKeyCount failed: %v", err)
		}
		if n != 10 {
			t.Errorf("remaining count = %d, want 10", n)
		}
	})

	t.Run("valid TOTP code accepted after enabling", func(t *testing.T) {
		code := computeTOTPCode(t, secret, time.Now().Unix()/30)
		if !svc.VerifyTOTPCode(secret, code) {
			t.Error("expected valid code to verify after enabling")
		}
	})

	t.Run("recovery key can be used exactly once", func(t *testing.T) {
		key := recoveryKeys[0]
		if err := svc.UseRecoveryKey(userID, key); err != nil {
			t.Fatalf("UseRecoveryKey failed on first use: %v", err)
		}
		if err := svc.UseRecoveryKey(userID, key); err == nil {
			t.Error("expected second use of the same recovery key to fail")
		}

		n, err := svc.GetRemainingRecoveryKeyCount(userID)
		if err != nil {
			t.Fatalf("GetRemainingRecoveryKeyCount failed: %v", err)
		}
		if n != 9 {
			t.Errorf("remaining count = %d, want 9 after consuming one key", n)
		}
	})

	t.Run("unknown recovery key rejected", func(t *testing.T) {
		if err := svc.UseRecoveryKey(userID, "ffffffff-ffff"); err == nil {
			t.Error("expected unknown recovery key to be rejected")
		}
	})

	t.Run("recovery key use is case-insensitive", func(t *testing.T) {
		key := strings.ToUpper(recoveryKeys[1])
		if err := svc.UseRecoveryKey(userID, key); err != nil {
			t.Fatalf("UseRecoveryKey failed for uppercase key: %v", err)
		}
	})

	t.Run("disable removes the TOTP row and HasTOTP becomes false", func(t *testing.T) {
		if err := svc.DisableTOTP(userID); err != nil {
			t.Fatalf("DisableTOTP failed: %v", err)
		}
		if svc.HasTOTP(userID) {
			t.Error("expected HasTOTP to be false after disabling")
		}
		if _, err := svc.GetTOTPSecret(userID); err == nil {
			t.Error("expected GetTOTPSecret to fail after disabling")
		}
	})

	t.Run("UseRecoveryKey fails once 2FA is disabled", func(t *testing.T) {
		if err := svc.UseRecoveryKey(userID, recoveryKeys[2]); err == nil {
			t.Error("expected UseRecoveryKey to fail with no active 2FA")
		}
	})
}

func TestEnableTOTPWithoutEncryptionKeyStoresPlaintext(t *testing.T) {
	st := newFullSchemaStore(t)
	// No encryption key configured — must fall back to plaintext storage
	// with key_version 0 rather than failing enrollment.
	svc := NewTOTPService(st, nil, 0)

	const userID = int64(2)
	secret, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}

	if _, err := svc.EnableTOTP(userID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	var stored string
	var keyVersion int
	if err := st.UsersDB.QueryRow(
		`SELECT secret, key_version FROM totp_secrets WHERE user_type = 'user' AND user_id = ?`, userID,
	).Scan(&stored, &keyVersion); err != nil {
		t.Fatalf("failed to read stored secret: %v", err)
	}
	if stored != secret {
		t.Error("expected plaintext fallback to store the secret as-is")
	}
	if keyVersion != 0 {
		t.Errorf("keyVersion = %d, want 0 for plaintext fallback", keyVersion)
	}

	got, err := svc.GetTOTPSecret(userID)
	if err != nil {
		t.Fatalf("GetTOTPSecret failed: %v", err)
	}
	if got != secret {
		t.Error("GetTOTPSecret did not return the original plaintext secret")
	}
}

func TestGetTOTPSecretEncryptedWithoutKeyConfiguredErrors(t *testing.T) {
	st := newFullSchemaStore(t)
	key := testEncryptionKey(t)

	// Enable with an encryption key present so the row is stored encrypted.
	enableSvc := NewTOTPService(st, key, 1)
	const userID = int64(3)
	secret, err := enableSvc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if _, err := enableSvc.EnableTOTP(userID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	// A service instance with no encryption key must fail to decrypt rather
	// than return a corrupted or empty secret.
	readSvc := NewTOTPService(st, nil, 0)
	if _, err := readSvc.GetTOTPSecret(userID); err == nil {
		t.Error("expected GetTOTPSecret to fail when the encryption key is missing")
	}
}

func TestEnableTOTPOverwritesExistingSecret(t *testing.T) {
	st := newFullSchemaStore(t)
	key := testEncryptionKey(t)
	svc := NewTOTPService(st, key, 1)

	const userID = int64(4)
	firstSecret, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if _, err := svc.EnableTOTP(userID, firstSecret); err != nil {
		t.Fatalf("first EnableTOTP failed: %v", err)
	}

	secondSecret, err := svc.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if secondSecret == firstSecret {
		t.Skip("generated identical secrets, cannot verify overwrite")
	}
	newKeys, err := svc.EnableTOTP(userID, secondSecret)
	if err != nil {
		t.Fatalf("second EnableTOTP failed: %v", err)
	}
	if len(newKeys) != 10 {
		t.Fatalf("len(newKeys) = %d, want 10", len(newKeys))
	}

	got, err := svc.GetTOTPSecret(userID)
	if err != nil {
		t.Fatalf("GetTOTPSecret failed: %v", err)
	}
	if got != secondSecret {
		t.Error("expected re-enabling to overwrite the stored secret")
	}

	// Only one row should exist per (user_type, user_id) per the UNIQUE
	// constraint and ON CONFLICT DO UPDATE clause.
	var count int
	if err := st.UsersDB.QueryRow(
		`SELECT COUNT(*) FROM totp_secrets WHERE user_type = 'user' AND user_id = ?`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("failed to count totp_secrets rows: %v", err)
	}
	if count != 1 {
		t.Errorf("totp_secrets row count = %d, want 1", count)
	}
}

func TestGetRemainingRecoveryKeyCountNoActiveTOTP(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	if _, err := svc.GetRemainingRecoveryKeyCount(999); err == nil {
		t.Error("expected error for user with no active 2FA")
	}
}

func TestHasTOTPUnknownUser(t *testing.T) {
	svc := NewTOTPService(newFullSchemaStore(t), nil, 0)

	if svc.HasTOTP(12345) {
		t.Error("expected HasTOTP to be false for unknown user")
	}
}
