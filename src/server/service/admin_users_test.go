package service

import (
	"context"
	"fmt"
	"testing"
)

// mustHashPassword hashes a password via the real Argon2id code path so
// tests never fabricate a hash value.
func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := hashPasswordArgon2id(password)
	if err != nil {
		t.Fatalf("hashPasswordArgon2id failed: %v", err)
	}
	return hash
}

func TestListUsers(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	svc := NewUserAdminService(st)

	hash := mustHashPassword(t, "correct-horse-battery-staple")
	for _, u := range []struct{ username, email string }{
		{"alice", "alice@example.com"},
		{"bob", "bob@example.com"},
		{"carol", "carol@example.com"},
	} {
		if _, err := st.UsersDB.ExecContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
			u.username, u.email, hash,
		); err != nil {
			t.Fatalf("failed to insert user %s: %v", u.username, err)
		}
	}

	t.Run("lists all users with default paging", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 0, 0, "")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(users) != 3 {
			t.Errorf("len(users) = %d, want 3", len(users))
		}
	})

	t.Run("search filters by username substring", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 1, 50, "ali")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if total != 1 || len(users) != 1 {
			t.Fatalf("got total=%d len=%d, want 1/1", total, len(users))
		}
		if users[0].Username != "alice" {
			t.Errorf("username = %q, want alice", users[0].Username)
		}
	})

	t.Run("search filters by email substring", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 1, 50, "bob@example.com")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if total != 1 || len(users) != 1 {
			t.Fatalf("got total=%d len=%d, want 1/1", total, len(users))
		}
	})

	t.Run("search with no matches returns empty, not error", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 1, 50, "nonexistent-user-xyz")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if total != 0 || len(users) != 0 {
			t.Fatalf("got total=%d len=%d, want 0/0", total, len(users))
		}
	})

	t.Run("search wildcard characters are escaped, not treated as patterns", func(t *testing.T) {
		// A literal "%" or "_" in the search term must not match every row.
		users, total, err := svc.ListUsers(ctx, 1, 50, "%")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if total != 0 || len(users) != 0 {
			t.Fatalf("got total=%d len=%d, want 0/0 (literal %% should not match)", total, len(users))
		}
	})

	t.Run("negative page normalizes to page 1", func(t *testing.T) {
		users, _, err := svc.ListUsers(ctx, -5, 50, "")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if len(users) != 3 {
			t.Errorf("len(users) = %d, want 3 (page should normalize to 1)", len(users))
		}
	})

	t.Run("out of range limit normalizes to default", func(t *testing.T) {
		users, _, err := svc.ListUsers(ctx, 1, 5000, "")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if len(users) != 3 {
			t.Errorf("len(users) = %d, want 3", len(users))
		}
	})

	t.Run("pagination limit=1 returns single page slices", func(t *testing.T) {
		users, total, err := svc.ListUsers(ctx, 1, 1, "")
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(users) != 1 {
			t.Errorf("len(users) = %d, want 1", len(users))
		}
	})
}

func TestListUsersEmpty(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewUserAdminService(st)

	users, total, err := svc.ListUsers(context.Background(), 1, 50, "")
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(users) != 0 {
		t.Errorf("len(users) = %d, want 0", len(users))
	}
}

func TestGetUser(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	svc := NewUserAdminService(st)

	hash := mustHashPassword(t, "another-strong-password")
	res, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		"dave", "dave@example.com", hash,
	)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId failed: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		u, err := svc.GetUser(ctx, id)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		if u.Username != "dave" {
			t.Errorf("username = %q, want dave", u.Username)
		}
		if u.Email != "dave@example.com" {
			t.Errorf("email = %q, want dave@example.com", u.Email)
		}
		if u.Suspended {
			t.Error("expected new user to not be suspended")
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := svc.GetUser(ctx, 99999); err == nil {
			t.Error("expected error for nonexistent user ID")
		}
	})
}

func TestSuspendAndActivateUser(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	svc := NewUserAdminService(st)

	hash := mustHashPassword(t, "suspend-me-password")
	res, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		"erin", "erin@example.com", hash,
	)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	id, _ := res.LastInsertId()

	t.Run("suspend sets suspended flag and reason", func(t *testing.T) {
		if err := svc.SuspendUser(ctx, id, "terms of service violation"); err != nil {
			t.Fatalf("SuspendUser failed: %v", err)
		}
		u, err := svc.GetUser(ctx, id)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		if !u.Suspended {
			t.Error("expected user to be suspended")
		}
		if u.SuspendReason != "terms of service violation" {
			t.Errorf("SuspendReason = %q, want %q", u.SuspendReason, "terms of service violation")
		}
	})

	t.Run("activate clears suspended flag and reason", func(t *testing.T) {
		if err := svc.ActivateUser(ctx, id); err != nil {
			t.Fatalf("ActivateUser failed: %v", err)
		}
		u, err := svc.GetUser(ctx, id)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		if u.Suspended {
			t.Error("expected user to no longer be suspended")
		}
		if u.SuspendReason != "" {
			t.Errorf("SuspendReason = %q, want empty", u.SuspendReason)
		}
	})

	t.Run("suspend nonexistent user errors", func(t *testing.T) {
		if err := svc.SuspendUser(ctx, 99999, "reason"); err == nil {
			t.Error("expected error suspending nonexistent user")
		}
	})

	t.Run("activate nonexistent user errors", func(t *testing.T) {
		if err := svc.ActivateUser(ctx, 99999); err == nil {
			t.Error("expected error activating nonexistent user")
		}
	})
}

// TestUsersTableEnforcesUniqueUsernameAndEmail exercises the DB-level
// uniqueness constraints that ListUsers/GetUser rely on to keep one row per
// account. admin_users.go has no separate CreateUser path of its own — the
// users table's UNIQUE constraints are the only duplicate guard in this
// package, so this test targets those constraints directly.
func TestUsersTableEnforcesUniqueUsernameAndEmail(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	hash := mustHashPassword(t, "duplicate-guard-password")

	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		"frank", "frank@example.com", hash,
	); err != nil {
		t.Fatalf("failed to insert first user: %v", err)
	}

	t.Run("duplicate username rejected", func(t *testing.T) {
		_, err := st.UsersDB.ExecContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
			"frank", "different@example.com", hash,
		)
		if err == nil {
			t.Error("expected duplicate username insert to fail")
		}
	})

	t.Run("duplicate email rejected", func(t *testing.T) {
		_, err := st.UsersDB.ExecContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
			"franklin", "frank@example.com", hash,
		)
		if err == nil {
			t.Error("expected duplicate email insert to fail")
		}
	})
}

func TestForceRegenerateRecoveryKeys(t *testing.T) {
	st := newFullSchemaStore(t)
	ctx := context.Background()
	svc := NewUserAdminService(st)

	hash := mustHashPassword(t, "recovery-owner-password")
	res, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		"grace", "grace@example.com", hash,
	)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	userID, _ := res.LastInsertId()

	t.Run("generates the configured key count, all unique", func(t *testing.T) {
		keys, err := svc.ForceRegenerateRecoveryKeys(ctx, userID)
		if err != nil {
			t.Fatalf("ForceRegenerateRecoveryKeys failed: %v", err)
		}
		if len(keys) != recoveryKeyCount {
			t.Fatalf("len(keys) = %d, want %d", len(keys), recoveryKeyCount)
		}
		seen := make(map[string]bool, len(keys))
		for _, k := range keys {
			if k == "" {
				t.Error("got empty recovery key")
			}
			if seen[k] {
				t.Errorf("duplicate recovery key generated: %q", k)
			}
			seen[k] = true
		}

		var count int
		userIDStr := fmt.Sprintf("%d", userID)
		if err := st.UsersDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM recovery_keys WHERE user_id = ? AND used = 0`,
			userIDStr,
		).Scan(&count); err != nil {
			t.Fatalf("failed to count recovery keys: %v", err)
		}
		if count != recoveryKeyCount {
			t.Errorf("unused recovery key count = %d, want %d", count, recoveryKeyCount)
		}
	})

	t.Run("regenerating replaces the previous unused set", func(t *testing.T) {
		firstBatch, err := svc.ForceRegenerateRecoveryKeys(ctx, userID)
		if err != nil {
			t.Fatalf("first ForceRegenerateRecoveryKeys failed: %v", err)
		}
		secondBatch, err := svc.ForceRegenerateRecoveryKeys(ctx, userID)
		if err != nil {
			t.Fatalf("second ForceRegenerateRecoveryKeys failed: %v", err)
		}

		firstSet := make(map[string]bool, len(firstBatch))
		for _, k := range firstBatch {
			firstSet[k] = true
		}
		overlap := 0
		for _, k := range secondBatch {
			if firstSet[k] {
				overlap++
			}
		}
		if overlap != 0 {
			t.Errorf("expected no overlap between regenerated key batches, got %d", overlap)
		}

		var count int
		if err := st.UsersDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM recovery_keys WHERE used = 0`,
		).Scan(&count); err != nil {
			t.Fatalf("failed to count recovery keys: %v", err)
		}
		if count != recoveryKeyCount {
			t.Errorf("unused recovery key count = %d, want %d (old batch should be removed)", count, recoveryKeyCount)
		}
	})
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text unchanged", "alice", "alice"},
		{"percent escaped", "50%off", "50\\%off"},
		{"underscore escaped", "a_b", "a\\_b"},
		{"backslash escaped first", `a\b`, `a\\b`},
		{"empty string", "", ""},
		{"mixed wildcards", "a%b_c\\d", `a\%b\_c\\d`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLikePattern(tt.input); got != tt.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
