package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/casjaysdevdocker/caslink/src/server/store"
	_ "modernc.org/sqlite"
)

// newTestStore creates an in-memory SQLite store for tests.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()

	// Each test gets its own uniquely named in-memory database so parallel
	// tests do not share state.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := []string{
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			user_type TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			token_prefix TEXT,
			name TEXT,
			permissions TEXT,
			last_used DATETIME,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("schema exec failed: %v", err)
		}
	}

	return store.NewTestStore(db)
}

func TestCreateAndValidateToken(t *testing.T) {
	st := newTestStore(t)
	svc := NewTokenService(st)
	ctx := context.Background()

	plaintext, err := svc.CreateToken(ctx, 1, "user", "test-token", []string{"read"}, nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected non-empty plaintext token")
	}

	// Valid plaintext succeeds
	rec, err := svc.ValidateToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("ValidateToken failed for correct plaintext: %v", err)
	}
	if rec.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", rec.UserID)
	}

	// Wrong plaintext fails
	if _, err := svc.ValidateToken(ctx, "wrong-token-value"); err == nil {
		t.Error("ValidateToken should fail for wrong plaintext")
	}
}

func TestRevokedTokenInvalid(t *testing.T) {
	st := newTestStore(t)
	svc := NewTokenService(st)
	ctx := context.Background()

	plaintext, err := svc.CreateToken(ctx, 2, "user", "revoke-test", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// Validate before revocation
	rec, err := svc.ValidateToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("ValidateToken before revocation failed: %v", err)
	}

	// Revoke
	if err := svc.RevokeToken(ctx, rec.ID, 2); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// Validate after revocation must fail
	if _, err := svc.ValidateToken(ctx, plaintext); err == nil {
		t.Error("ValidateToken should fail after revocation")
	}
}
