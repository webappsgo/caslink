package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheckNameAvailableFree(t *testing.T) {
	st := newFullSchemaStore(t)
	if err := CheckNameAvailable(context.Background(), st, "brandnew"); err != nil {
		t.Fatalf("expected available, got %v", err)
	}
}

func TestCheckNameAvailableReserved(t *testing.T) {
	st := newFullSchemaStore(t)
	// "admin" is in the username blocklist.
	if err := CheckNameAvailable(context.Background(), st, "admin"); !errors.Is(err, ErrNameReserved) {
		t.Fatalf("err = %v, want ErrNameReserved", err)
	}
}

func TestCheckNameAvailableUsernameTaken(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, email_verified, created_at)
		 VALUES ('taken', 'taken@example.com', 'x', 0, ?)`, time.Now()); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	// A username blocks the same name from being claimed as an org slug.
	if err := CheckNameAvailable(ctx, st, "TAKEN"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken (case-insensitive)", err)
	}
}

func TestCheckNameAvailableSlugTaken(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, email_verified, created_at)
		 VALUES ('owner', 'owner@example.com', 'x', 0, ?)`, time.Now()); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var ownerID int64
	if err := st.UsersDB.QueryRowContext(ctx,
		`SELECT id FROM users WHERE username = 'owner'`).Scan(&ownerID); err != nil {
		t.Fatalf("select owner: %v", err)
	}
	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO organizations (name, slug, owner_id, created_at, updated_at)
		 VALUES ('Acme', 'acme', ?, ?, ?)`, ownerID, time.Now(), time.Now()); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	// An org slug blocks the same name from being claimed as a username.
	if err := CheckNameAvailable(ctx, st, "acme"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
}

func TestCreateOrganizationRejectsUsernameCollision(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	userID := insertOrgTestUser(t, st, "alice", "alice@example.com")
	// Register a plain user whose name will collide with a future org slug.
	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, email_verified, created_at)
		 VALUES ('devteam', 'devteam@example.com', 'x', 0, ?)`, time.Now()); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	svc := NewOrgService(st)
	if _, err := svc.CreateOrganization(ctx, userID, "Dev Team", "devteam"); err == nil {
		t.Fatal("expected org creation to fail on username collision")
	}
}

func TestRegisterUserRejectsOrgSlugCollision(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	// Seed an owner and an organization whose slug will collide with a future
	// username registration, exercising the reverse shared-namespace direction.
	ownerID := insertOrgTestUser(t, st, "owner", "owner@example.com")
	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO organizations (name, slug, owner_id, created_at, updated_at)
		 VALUES ('Widgets', 'widgets', ?, ?, ?)`, ownerID, time.Now(), time.Now()); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	svc := NewAuthService(st)
	if _, err := svc.RegisterUser(ctx, "widgets", "widgets@example.com", "correct-password"); err == nil {
		t.Fatal("expected user registration to fail on org-slug collision")
	}
}
