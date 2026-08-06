package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/validate"
)

// Namespace-availability sentinel errors.
var (
	// ErrNameReserved indicates the name is on the reserved/blocklist.
	ErrNameReserved = errors.New("name is reserved")
	// ErrNameTaken indicates the name already exists as a username or org slug.
	ErrNameTaken = errors.New("name is already taken")
)

// CheckNameAvailable enforces the shared username/org-slug namespace required by
// AI.md ("Global uniqueness: Usernames and org slugs share the SAME namespace.
// A name cannot exist as both user AND org."). It returns nil when the
// (already format-validated) name is free to claim as either a regular-user
// username or an organization slug, or a sentinel error otherwise.
//
// It intentionally consults only the users and organizations tables — Server
// Admin usernames live in a separate table and a separate namespace, and the
// admin blocklist does not apply to them.
func CheckNameAvailable(ctx context.Context, st *store.Store, name string) error {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, blocked := range validate.UsernameBlocklist {
		if name == strings.ToLower(blocked) {
			return ErrNameReserved
		}
	}

	var count int
	if err := st.UsersDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE username = ?", name).Scan(&count); err != nil {
		return fmt.Errorf("failed to check username namespace: %w", err)
	}
	if count > 0 {
		return ErrNameTaken
	}

	if err := st.UsersDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM organizations WHERE slug = ?", name).Scan(&count); err != nil {
		return fmt.Errorf("failed to check org-slug namespace: %w", err)
	}
	if count > 0 {
		return ErrNameTaken
	}

	return nil
}
