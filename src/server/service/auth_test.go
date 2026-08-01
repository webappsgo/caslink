package service

import (
	"context"
	"testing"
	"time"
)

// Admin authentication: covers the primary-admin bootstrap path, successful
// login with a real Argon2id hash/verify round trip, wrong-password
// rejection, and last_login bookkeeping.
func TestAuthenticateAdmin_Success(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-horse-battery", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	admin, err := svc.AuthenticateAdmin(ctx, "root", "correct-horse-battery")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}
	if admin.Username != "root" {
		t.Errorf("expected username %q, got %q", "root", admin.Username)
	}
	if !admin.IsPrimary {
		t.Error("expected primary admin flag to be true")
	}
	if admin.LastLogin != nil {
		t.Error("expected LastLogin to be nil before first successful auth")
	}

	// Re-fetch to confirm last_login was updated as a side effect of the
	// successful authentication above.
	admin2, err := svc.AuthenticateAdmin(ctx, "root", "correct-horse-battery")
	if err != nil {
		t.Fatalf("second AuthenticateAdmin failed: %v", err)
	}
	if admin2.LastLogin == nil {
		t.Error("expected LastLogin to be set after a prior successful login")
	}
}

// CreatePrimaryAdmin must refuse to create a second admin once one exists.
func TestCreatePrimaryAdmin_AlreadyExists(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "password1", "root@example.com"); err != nil {
		t.Fatalf("first CreatePrimaryAdmin failed: %v", err)
	}
	if err := svc.CreatePrimaryAdmin(ctx, "second", "password2", "second@example.com"); err == nil {
		t.Error("expected error when creating a second admin, got nil")
	}
}

// Wrong password and a nonexistent username must produce indistinguishable
// errors — auth.go runs Argon2id against a dummy hash on the no-such-user
// path specifically so the two cases cannot be told apart.
func TestAuthenticateAdmin_WrongPasswordAndNoSuchUser(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	_, wrongPassErr := svc.AuthenticateAdmin(ctx, "root", "wrong-password")
	if wrongPassErr == nil {
		t.Fatal("expected error for wrong password, got nil")
	}

	_, noSuchUserErr := svc.AuthenticateAdmin(ctx, "does-not-exist", "whatever-password")
	if noSuchUserErr == nil {
		t.Fatal("expected error for nonexistent admin, got nil")
	}

	if wrongPassErr.Error() != noSuchUserErr.Error() {
		t.Errorf("wrong-password and no-such-user errors must match to avoid leaking account existence: %q vs %q",
			wrongPassErr.Error(), noSuchUserErr.Error())
	}
}

// Empty credentials are handled the same way as any other non-matching
// input — no special-cased crash or bypass.
func TestAuthenticateAdmin_EmptyCredentials(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	if _, err := svc.AuthenticateAdmin(ctx, "", ""); err == nil {
		t.Error("expected error for empty username/password, got nil")
	}
	if _, err := svc.AuthenticateAdmin(ctx, "root", ""); err == nil {
		t.Error("expected error for empty password, got nil")
	}
}

// Admin session lifecycle: create, validate, delete (logout), and confirm a
// deleted session no longer validates.
func TestAdminSession_CreateValidateDelete(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}
	admin, err := svc.AuthenticateAdmin(ctx, "root", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}

	sessionID, err := svc.CreateSession(ctx, admin.ID, false)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	got, err := svc.ValidateSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateSession failed: %v", err)
	}
	if got.ID != admin.ID {
		t.Errorf("expected admin ID %d, got %d", admin.ID, got.ID)
	}

	if err := svc.DeleteSession(ctx, sessionID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	if _, err := svc.ValidateSession(ctx, sessionID); err == nil {
		t.Error("expected error validating a deleted (logged-out) session")
	}
}

// An unknown session ID must fail validation.
func TestAdminSession_ValidateUnknown(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if _, err := svc.ValidateSession(ctx, "does-not-exist"); err == nil {
		t.Error("expected error validating an unknown session ID")
	}
}

// An expired admin session must fail validation even though the row still
// exists — expiry is enforced in the SQL predicate, not by a cleanup job.
func TestAdminSession_Expired(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}
	admin, err := svc.AuthenticateAdmin(ctx, "root", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}

	expiredSessionID := "expired-admin-session"
	pastExpiry := time.Now().Add(-1 * time.Hour).Unix()
	_, err = st.ServerDB.ExecContext(ctx,
		`INSERT INTO admin_sessions (id, admin_id, expires_at) VALUES (?, ?, ?)`,
		expiredSessionID, admin.ID, pastExpiry)
	if err != nil {
		t.Fatalf("failed to insert expired session: %v", err)
	}

	if _, err := svc.ValidateSession(ctx, expiredSessionID); err == nil {
		t.Error("expected error validating an expired session")
	}
}

// rememberMe must extend the admin session lifetime (90 days) beyond the
// default (30 days).
func TestAdminSession_RememberMeExtendsExpiry(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}
	admin, err := svc.AuthenticateAdmin(ctx, "root", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}

	shortID, err := svc.CreateSession(ctx, admin.ID, false)
	if err != nil {
		t.Fatalf("CreateSession (no remember) failed: %v", err)
	}
	longID, err := svc.CreateSession(ctx, admin.ID, true)
	if err != nil {
		t.Fatalf("CreateSession (remember) failed: %v", err)
	}

	var shortExpiry, longExpiry int64
	if err := st.ServerDB.QueryRowContext(ctx,
		`SELECT expires_at FROM admin_sessions WHERE id = ?`, shortID).Scan(&shortExpiry); err != nil {
		t.Fatalf("failed to read short session expiry: %v", err)
	}
	if err := st.ServerDB.QueryRowContext(ctx,
		`SELECT expires_at FROM admin_sessions WHERE id = ?`, longID).Scan(&longExpiry); err != nil {
		t.Fatalf("failed to read long session expiry: %v", err)
	}

	if longExpiry <= shortExpiry {
		t.Errorf("expected rememberMe session to expire later: remembered=%d, default=%d", longExpiry, shortExpiry)
	}
}

// NeedsSetup reports true with no admins and false once one exists.
func TestNeedsSetup(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	needs, err := svc.NeedsSetup(ctx)
	if err != nil {
		t.Fatalf("NeedsSetup failed: %v", err)
	}
	if !needs {
		t.Error("expected NeedsSetup to be true with no admins")
	}

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	needs, err = svc.NeedsSetup(ctx)
	if err != nil {
		t.Fatalf("NeedsSetup failed: %v", err)
	}
	if needs {
		t.Error("expected NeedsSetup to be false once an admin exists")
	}
}

// RegisterUser + AuthenticateUser round trip, including case-insensitive
// username/email normalization.
func TestRegisterAndAuthenticateUser(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user, err := svc.RegisterUser(ctx, "Alice", "Alice@Example.com", "hunter2-hunter2")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("expected normalized username %q, got %q", "alice", user.Username)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("expected normalized email %q, got %q", "alice@example.com", user.Email)
	}

	// Login by username.
	byUsername, err := svc.AuthenticateUser(ctx, "alice", "hunter2-hunter2")
	if err != nil {
		t.Fatalf("AuthenticateUser by username failed: %v", err)
	}
	if byUsername.ID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, byUsername.ID)
	}

	// Login by email, mixed case, to also exercise the identifier
	// normalization on the login path.
	byEmail, err := svc.AuthenticateUser(ctx, "ALICE@example.com", "hunter2-hunter2")
	if err != nil {
		t.Fatalf("AuthenticateUser by email failed: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, byEmail.ID)
	}
}

// Registering a duplicate username or email must fail with a generic
// error that does not reveal which field collided.
func TestRegisterUser_DuplicateUsernameAndEmail(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if _, err := svc.RegisterUser(ctx, "bob", "bob@example.com", "password123"); err != nil {
		t.Fatalf("initial RegisterUser failed: %v", err)
	}

	if _, err := svc.RegisterUser(ctx, "bob", "someone-else@example.com", "password456"); err == nil {
		t.Error("expected error registering a duplicate username")
	}
	if _, err := svc.RegisterUser(ctx, "someone-else", "bob@example.com", "password789"); err == nil {
		t.Error("expected error registering a duplicate email")
	}
}

// Wrong password and a nonexistent identifier must be indistinguishable,
// mirroring the admin no-enumeration guarantee.
func TestAuthenticateUser_WrongPasswordAndNoSuchUser(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if _, err := svc.RegisterUser(ctx, "carol", "carol@example.com", "correct-password"); err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	_, wrongPassErr := svc.AuthenticateUser(ctx, "carol", "wrong-password")
	if wrongPassErr == nil {
		t.Fatal("expected error for wrong password, got nil")
	}

	_, noSuchUserErr := svc.AuthenticateUser(ctx, "no-such-user", "whatever-password")
	if noSuchUserErr == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}

	if wrongPassErr.Error() != noSuchUserErr.Error() {
		t.Errorf("wrong-password and no-such-user errors must match to avoid leaking account existence: %q vs %q",
			wrongPassErr.Error(), noSuchUserErr.Error())
	}
}

// User session lifecycle: create, validate, expiry via the JOIN-based
// ValidateUserSession query.
func TestUserSession_CreateValidateExpiry(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user, err := svc.RegisterUser(ctx, "dave", "dave@example.com", "correct-password")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	sessionID, err := svc.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}

	got, err := svc.ValidateUserSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateUserSession failed: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, got.ID)
	}

	expiredSessionID := "expired-user-session"
	pastExpiry := time.Now().Add(-1 * time.Hour).Unix()
	_, err = st.UsersDB.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		expiredSessionID, user.ID, pastExpiry)
	if err != nil {
		t.Fatalf("failed to insert expired user session: %v", err)
	}

	if _, err := svc.ValidateUserSession(ctx, expiredSessionID); err == nil {
		t.Error("expected error validating an expired user session")
	}
}

// GetUserByID resolves a known user and errors on an unknown one — used by
// the Bearer-auth middleware to hydrate the full User from a TokenRecord.
func TestGetUserByID(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user, err := svc.RegisterUser(ctx, "erin", "erin@example.com", "correct-password")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	got, err := svc.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if got.Username != "erin" {
		t.Errorf("expected username %q, got %q", "erin", got.Username)
	}

	if _, err := svc.GetUserByID(ctx, 999999); err == nil {
		t.Error("expected error for unknown user ID")
	}
}

// TOTPEnabled is surfaced on the Admin/User structs by the login path so
// callers can gate a follow-up 2FA challenge — auth.go itself does not
// block the returned Admin/User on totp_enabled, so this only verifies the
// flag round-trips correctly through AuthenticateAdmin/AuthenticateUser.
func TestAuthenticate_TOTPEnabledFlagRoundTrips(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}
	if _, err := st.UsersDB.ExecContext(ctx,
		`UPDATE admins SET totp_enabled = 1 WHERE username = ?`, "root"); err != nil {
		t.Fatalf("failed to enable TOTP for admin: %v", err)
	}

	admin, err := svc.AuthenticateAdmin(ctx, "root", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}
	if !admin.TOTPEnabled {
		t.Error("expected TOTPEnabled to be true after enabling TOTP for the admin")
	}
}

// Password reset: happy path issues a token, validates it, resets the
// password with the real Argon2id hashing path, invalidates the token
// against reuse, and revokes existing sessions.
func TestPasswordReset_FullFlow(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user, err := svc.RegisterUser(ctx, "frank", "frank@example.com", "old-password-123")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	sessionID, err := svc.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}

	token, err := svc.CreatePasswordResetToken(ctx, "frank@example.com", "user")
	if err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty reset token")
	}

	gotUserID, gotUserType, err := svc.ValidatePasswordResetToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidatePasswordResetToken failed: %v", err)
	}
	if gotUserID != user.ID || gotUserType != "user" {
		t.Errorf("expected (%d, user), got (%d, %s)", user.ID, gotUserID, gotUserType)
	}

	if err := svc.ResetPassword(ctx, token, "new-password-456"); err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// Old password must no longer work; new password must.
	if _, err := svc.AuthenticateUser(ctx, "frank", "old-password-123"); err == nil {
		t.Error("expected old password to be rejected after reset")
	}
	if _, err := svc.AuthenticateUser(ctx, "frank", "new-password-456"); err != nil {
		t.Errorf("expected new password to authenticate, got error: %v", err)
	}

	// Existing session must be invalidated by the reset.
	if _, err := svc.ValidateUserSession(ctx, sessionID); err == nil {
		t.Error("expected prior session to be invalidated after password reset")
	}

	// Token is single-use.
	if err := svc.ResetPassword(ctx, token, "another-password-789"); err == nil {
		t.Error("expected reused reset token to fail")
	}
}

// Requesting a reset token for an unknown email must not error and must
// not reveal whether the address exists — it returns an empty token with a
// nil error to defeat enumeration.
func TestCreatePasswordResetToken_UnknownEmail(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	token, err := svc.CreatePasswordResetToken(ctx, "nobody@example.com", "user")
	if err != nil {
		t.Fatalf("expected nil error for unknown email, got: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token for unknown email, got %q", token)
	}
}

// An unknown or malformed reset token must fail validation.
func TestValidatePasswordResetToken_Invalid(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if _, _, err := svc.ValidatePasswordResetToken(ctx, "not-a-real-token"); err == nil {
		t.Error("expected error validating a bogus reset token")
	}
}

// VerifyPassword and ChangePassword operate on the real Argon2id hash, and
// ChangePassword's new hash must verify while the old one no longer does.
func TestVerifyAndChangePassword(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user, err := svc.RegisterUser(ctx, "grace", "grace@example.com", "original-password")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	if err := svc.VerifyPassword(user.ID, "original-password"); err != nil {
		t.Errorf("VerifyPassword failed for correct password: %v", err)
	}
	if err := svc.VerifyPassword(user.ID, "wrong-password"); err == nil {
		t.Error("expected VerifyPassword to fail for wrong password")
	}

	if err := svc.ChangePassword(user.ID, "brand-new-password"); err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	if err := svc.VerifyPassword(user.ID, "original-password"); err == nil {
		t.Error("expected old password to fail VerifyPassword after change")
	}
	if err := svc.VerifyPassword(user.ID, "brand-new-password"); err != nil {
		t.Errorf("expected new password to pass VerifyPassword after change: %v", err)
	}
}

// UpdateUserProfile applies partial updates and rejects an email already
// used by a different account.
func TestUpdateUserProfile(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user1, err := svc.RegisterUser(ctx, "heidi", "heidi@example.com", "password123")
	if err != nil {
		t.Fatalf("RegisterUser (user1) failed: %v", err)
	}
	if _, err := svc.RegisterUser(ctx, "ivan", "ivan@example.com", "password123"); err != nil {
		t.Fatalf("RegisterUser (user2) failed: %v", err)
	}

	displayName := "Heidi H"
	bio := "hello world"
	updated, err := svc.UpdateUserProfile(ctx, user1.ID, &displayName, &bio, nil)
	if err != nil {
		t.Fatalf("UpdateUserProfile failed: %v", err)
	}
	if updated.DisplayName == nil || *updated.DisplayName != displayName {
		t.Errorf("expected display name %q, got %v", displayName, updated.DisplayName)
	}
	if updated.Bio == nil || *updated.Bio != bio {
		t.Errorf("expected bio %q, got %v", bio, updated.Bio)
	}

	// Changing to an email already used by a different account must fail.
	takenEmail := "ivan@example.com"
	if _, err := svc.UpdateUserProfile(ctx, user1.ID, nil, nil, &takenEmail); err != ErrEmailAlreadyInUse {
		t.Errorf("expected ErrEmailAlreadyInUse, got %v", err)
	}

	// Changing to a free email must succeed and reset email_verified.
	newEmail := "heidi-new@example.com"
	updated, err = svc.UpdateUserProfile(ctx, user1.ID, nil, nil, &newEmail)
	if err != nil {
		t.Fatalf("UpdateUserProfile with new email failed: %v", err)
	}
	if updated.Email != newEmail {
		t.Errorf("expected email %q, got %q", newEmail, updated.Email)
	}
	if updated.EmailVerified {
		t.Error("expected email_verified to be reset to false after an email change")
	}
}

// GetUserSessions, RevokeSession, and RevokeAllUserSessions cover the
// active-sessions management surface used by the security settings page.
func TestSessionManagement(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	user, err := svc.RegisterUser(ctx, "judy", "judy@example.com", "password123")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	session1, err := svc.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession (1) failed: %v", err)
	}
	session2, err := svc.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession (2) failed: %v", err)
	}

	sessions, err := svc.GetUserSessions(ctx, user.ID, "user")
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 active sessions, got %d", len(sessions))
	}

	if err := svc.RevokeSession(ctx, session1); err != nil {
		t.Fatalf("RevokeSession failed: %v", err)
	}
	if _, err := svc.ValidateUserSession(ctx, session1); err == nil {
		t.Error("expected revoked session to fail validation")
	}
	if _, err := svc.ValidateUserSession(ctx, session2); err != nil {
		t.Errorf("expected untouched session to still validate: %v", err)
	}

	session3, err := svc.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession (3) failed: %v", err)
	}
	if err := svc.RevokeAllUserSessions(ctx, user.ID, "user", session3); err != nil {
		t.Fatalf("RevokeAllUserSessions failed: %v", err)
	}
	if _, err := svc.ValidateUserSession(ctx, session2); err == nil {
		t.Error("expected session2 to be revoked by RevokeAllUserSessions")
	}
	if _, err := svc.ValidateUserSession(ctx, session3); err != nil {
		t.Errorf("expected the excepted session3 to remain valid: %v", err)
	}
}

// RevokeAllUserSessions must route to admin_sessions (server.db) rather
// than user_sessions when userType is "admin".
func TestRevokeAllUserSessions_AdminType(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuthService(st)
	ctx := context.Background()

	if err := svc.CreatePrimaryAdmin(ctx, "root", "correct-password", "root@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}
	admin, err := svc.AuthenticateAdmin(ctx, "root", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}

	kept, err := svc.CreateSession(ctx, admin.ID, false)
	if err != nil {
		t.Fatalf("CreateSession (kept) failed: %v", err)
	}
	revoked, err := svc.CreateSession(ctx, admin.ID, false)
	if err != nil {
		t.Fatalf("CreateSession (revoked) failed: %v", err)
	}

	if err := svc.RevokeAllUserSessions(ctx, admin.ID, "admin", kept); err != nil {
		t.Fatalf("RevokeAllUserSessions failed: %v", err)
	}

	if _, err := svc.ValidateSession(ctx, revoked); err == nil {
		t.Error("expected the non-excepted admin session to be revoked")
	}
	if _, err := svc.ValidateSession(ctx, kept); err != nil {
		t.Errorf("expected the excepted admin session to remain valid: %v", err)
	}
}
