package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// UserAdminService provides admin-level user management operations.
type UserAdminService struct {
	store *store.Store
}

// NewUserAdminService creates a new UserAdminService.
func NewUserAdminService(st *store.Store) *UserAdminService {
	return &UserAdminService{store: st}
}

// AdminUser is the admin view of a user account, including moderation state.
type AdminUser struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	EmailVerified bool       `json:"email_verified"`
	TOTPEnabled   bool       `json:"totp_enabled"`
	Suspended     bool       `json:"suspended"`
	SuspendReason string     `json:"suspend_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLogin     *time.Time `json:"last_login,omitempty"`
}

// ListUsers returns a paginated (and optionally searched) list of users.
// Returns the slice, total count, and any error.
func (s *UserAdminService) ListUsers(ctx context.Context, page, limit int, search string) ([]*AdminUser, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	var total int
	var rows *sql.Rows
	var err error

	if search != "" {
		pattern := "%" + search + "%"
		if err = s.store.UsersDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE username LIKE ? OR email LIKE ?`,
			pattern, pattern,
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count users: %w", err)
		}
		rows, err = s.store.UsersDB.QueryContext(ctx,
			`SELECT id, username, email, email_verified, totp_enabled,
			        COALESCE(suspended, 0), COALESCE(suspend_reason,''), created_at, last_login
			 FROM users
			 WHERE username LIKE ? OR email LIKE ?
			 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			pattern, pattern, limit, offset,
		)
	} else {
		if err = s.store.UsersDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users`,
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count users: %w", err)
		}
		rows, err = s.store.UsersDB.QueryContext(ctx,
			`SELECT id, username, email, email_verified, totp_enabled,
			        COALESCE(suspended, 0), COALESCE(suspend_reason,''), created_at, last_login
			 FROM users
			 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*AdminUser
	for rows.Next() {
		u, err := scanAdminUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("user row iteration error: %w", err)
	}

	return users, total, nil
}

// GetUser returns a single user by ID.
func (s *UserAdminService) GetUser(ctx context.Context, id int64) (*AdminUser, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.store.UsersDB.QueryContext(ctx,
		`SELECT id, username, email, email_verified, totp_enabled,
		        COALESCE(suspended, 0), COALESCE(suspend_reason,''), created_at, last_login
		 FROM users WHERE id = ?`, id,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("user not found")
	}
	u, err := scanAdminUser(rows)
	if err != nil {
		return nil, err
	}
	return u, rows.Err()
}

// SuspendUser marks a user as suspended with an optional reason.
func (s *UserAdminService) SuspendUser(ctx context.Context, id int64, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE users SET suspended = 1, suspend_reason = ? WHERE id = ?`,
		reason, id,
	)
	if err != nil {
		return fmt.Errorf("failed to suspend user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// ActivateUser clears the suspended flag on a user account.
func (s *UserAdminService) ActivateUser(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE users SET suspended = 0, suspend_reason = '' WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("failed to activate user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// ForceRegenerateRecoveryKeys replaces all unused recovery keys for the given
// user with a fresh set. This is the admin-override path described in PART 17/34.
// The plain-text keys are returned exactly once; subsequent calls produce a new set.
func (s *UserAdminService) ForceRegenerateRecoveryKeys(ctx context.Context, userID int64) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	plainKeys := make([]string, recoveryKeyCount)
	for i := range plainKeys {
		key, err := generateRecoveryKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate recovery key %d: %w", i+1, err)
		}
		plainKeys[i] = key
	}

	userIDStr := fmt.Sprintf("%d", userID)

	tx, err := s.store.UsersDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Remove existing unused keys for this user.
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM recovery_keys WHERE user_id = ? AND used = 0`,
		userIDStr,
	); err != nil {
		return nil, fmt.Errorf("failed to clear recovery keys: %w", err)
	}

	for _, key := range plainKeys {
		hash := hashRecoveryKey(key)
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO recovery_keys (id, user_id, key_hash, used, created_at) VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP)`,
			uuid.New().String(), userIDStr, hash,
		); err != nil {
			return nil, fmt.Errorf("failed to insert recovery key: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit recovery keys: %w", err)
	}

	return plainKeys, nil
}

// scanAdminUser scans one row from a users query into an AdminUser.
func scanAdminUser(rows *sql.Rows) (*AdminUser, error) {
	var u AdminUser
	var lastLogin sql.NullTime
	if err := rows.Scan(
		&u.ID, &u.Username, &u.Email, &u.EmailVerified, &u.TOTPEnabled,
		&u.Suspended, &u.SuspendReason, &u.CreatedAt, &lastLogin,
	); err != nil {
		return nil, fmt.Errorf("failed to scan user row: %w", err)
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		u.LastLogin = &t
	}
	return &u, nil
}
