package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newClusterTestStore builds an in-memory store that also carries the
// srv_cluster_join_tokens table (newTestStore only creates the tokens table).
func newClusterTestStore(t *testing.T) *ClusterService {
	t.Helper()
	st := newTestStore(t)
	_, err := st.ServerDB.Exec(`CREATE TABLE IF NOT EXISTS srv_cluster_join_tokens (
		id            TEXT PRIMARY KEY,
		token_hash    TEXT NOT NULL UNIQUE,
		token_prefix  TEXT NOT NULL,
		label         TEXT,
		created_by    TEXT,
		created_at    DATETIME NOT NULL,
		expires_at    DATETIME NOT NULL,
		used_at       DATETIME,
		used_by       TEXT,
		lockout_until DATETIME
	)`)
	if err != nil {
		t.Fatalf("failed to create cluster table: %v", err)
	}
	return NewClusterService(st)
}

func TestGenerateJoinTokenFormat(t *testing.T) {
	svc := newClusterTestStore(t)
	plaintext, err := svc.GenerateJoinToken(context.Background(), "eu-west", "primary")
	if err != nil {
		t.Fatalf("GenerateJoinToken failed: %v", err)
	}
	if !strings.HasPrefix(plaintext, "node_") {
		t.Errorf("token %q missing node_ prefix", plaintext)
	}
	// node_ (5) + 32 random chars.
	if len(plaintext) != len("node_")+32 {
		t.Errorf("token %q has unexpected length %d", plaintext, len(plaintext))
	}
}

func TestGenerateAndConsumeJoinToken(t *testing.T) {
	svc := newClusterTestStore(t)
	ctx := context.Background()

	plaintext, err := svc.GenerateJoinToken(ctx, "node-2", "primary")
	if err != nil {
		t.Fatalf("GenerateJoinToken failed: %v", err)
	}

	if err := svc.ValidateAndConsume(ctx, plaintext, "node-2"); err != nil {
		t.Fatalf("first ValidateAndConsume should succeed: %v", err)
	}

	// Second redemption must be rejected — the token is single-use and now in
	// its reuse-lockout window.
	if err := svc.ValidateAndConsume(ctx, plaintext, "node-2"); err == nil {
		t.Fatal("second ValidateAndConsume should fail (single-use)")
	}
}

func TestExpiredJoinTokenRejected(t *testing.T) {
	svc := newClusterTestStore(t)
	ctx := context.Background()

	plaintext, err := svc.GenerateJoinToken(ctx, "", "primary")
	if err != nil {
		t.Fatalf("GenerateJoinToken failed: %v", err)
	}
	// Force the single token past its expiry.
	if _, err := svc.store.ServerDB.Exec(
		`UPDATE srv_cluster_join_tokens SET expires_at = ?`,
		time.Now().Add(-1*time.Minute).UTC(),
	); err != nil {
		t.Fatalf("failed to age token: %v", err)
	}

	err = svc.ValidateAndConsume(ctx, plaintext, "node-2")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestInvalidJoinTokenRejected(t *testing.T) {
	svc := newClusterTestStore(t)
	err := svc.ValidateAndConsume(context.Background(), "node_notarealtokenatall000000000000", "node-2")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid error, got %v", err)
	}
}

func TestListAndRevokeJoinTokens(t *testing.T) {
	svc := newClusterTestStore(t)
	ctx := context.Background()

	if _, err := svc.GenerateJoinToken(ctx, "a", "primary"); err != nil {
		t.Fatalf("generate a failed: %v", err)
	}
	if _, err := svc.GenerateJoinToken(ctx, "b", "primary"); err != nil {
		t.Fatalf("generate b failed: %v", err)
	}

	list, err := svc.ListJoinTokens(ctx, 50)
	if err != nil {
		t.Fatalf("ListJoinTokens failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(list))
	}
	for _, rec := range list {
		if !strings.HasPrefix(rec.TokenPrefix, "node_") {
			t.Errorf("display prefix %q should start with node_", rec.TokenPrefix)
		}
		if rec.Status() != "pending" {
			t.Errorf("fresh token should be pending, got %q", rec.Status())
		}
	}

	// Revoke a pending token, then confirm it is gone and can't be redeemed.
	id := list[0].ID
	if err := svc.RevokeJoinToken(ctx, id); err != nil {
		t.Fatalf("RevokeJoinToken failed: %v", err)
	}
	after, _ := svc.ListJoinTokens(ctx, 50)
	if len(after) != 1 {
		t.Fatalf("expected 1 token after revoke, got %d", len(after))
	}
	// Revoking again is a no-op error (already removed).
	if err := svc.RevokeJoinToken(ctx, id); err == nil {
		t.Fatal("revoking a missing token should error")
	}
}

func TestRevokeUsedJoinTokenFails(t *testing.T) {
	svc := newClusterTestStore(t)
	ctx := context.Background()

	plaintext, err := svc.GenerateJoinToken(ctx, "", "primary")
	if err != nil {
		t.Fatalf("GenerateJoinToken failed: %v", err)
	}
	if err := svc.ValidateAndConsume(ctx, plaintext, "node-2"); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	list, _ := svc.ListJoinTokens(ctx, 50)
	if len(list) != 1 || list[0].Status() != "used" {
		t.Fatalf("expected 1 used token, got %+v", list)
	}
	// A used token must be retained (its lockout keeps defeating replay), so
	// revocation must refuse to delete it.
	if err := svc.RevokeJoinToken(ctx, list[0].ID); err == nil {
		t.Fatal("revoking a used token should error")
	}
}
