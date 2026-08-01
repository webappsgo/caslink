package model

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestUserOptionalFieldsOmittedWhenNil verifies profile fields that may
// legitimately be unset (DisplayName, Avatar, Bio, LastLogin) stay absent
// from the JSON payload rather than serializing as null/empty-string.
func TestUserOptionalFieldsOmittedWhenNil(t *testing.T) {
	u := User{
		ID:       1,
		Username: "alice",
		Email:    "alice@example.com",
	}

	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	omittedKeys := []string{"display_name", "avatar", "bio", "last_login"}
	for _, key := range omittedKeys {
		if _, ok := decoded[key]; ok {
			t.Errorf("expected %q to be omitted from JSON when nil, got: %v", key, decoded[key])
		}
	}

	requiredKeys := []string{"id", "username", "email", "email_verified", "totp_enabled", "created_at"}
	for _, key := range requiredKeys {
		if _, ok := decoded[key]; !ok {
			t.Errorf("expected %q to always be present in JSON, but it was omitted", key)
		}
	}
}

// TestUserStructHasNoPasswordField is a regression guard: User is the
// type returned to API clients, and it must never gain a password/hash
// field that could leak credential material. If someone adds one without
// a json:"-" tag, this test's JSON round-trip of a populated User would
// need to be updated deliberately rather than silently start leaking data.
func TestUserStructHasNoPasswordField(t *testing.T) {
	u := User{ID: 1, Username: "alice", Email: "alice@example.com"}
	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	for _, forbidden := range []string{"password", "password_hash", "Password", "PasswordHash"} {
		if _, ok := decoded[forbidden]; ok {
			t.Errorf("User JSON output must never contain a %q field", forbidden)
		}
	}
}

// TestRegisterUserRequestFieldMapping checks the registration DTO's JSON
// tags map correctly — a typo here would silently drop a required field
// from every registration request.
func TestRegisterUserRequestFieldMapping(t *testing.T) {
	payload := `{"username":"alice","email":"alice@example.com","password":"hunter22"}`
	var req RegisterUserRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if req.Username != "alice" || req.Email != "alice@example.com" || req.Password != "hunter22" {
		t.Errorf("field mapping mismatch: %+v", req)
	}
}

// TestLoginRequestRememberMeDefaultsFalse confirms the zero value for the
// non-pointer RememberMe bool is false when omitted from the payload, and
// that it's honored when explicitly set.
func TestLoginRequestRememberMeDefaultsFalse(t *testing.T) {
	var req LoginRequest
	if err := json.Unmarshal([]byte(`{"identifier":"alice","password":"secret"}`), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if req.RememberMe {
		t.Error("expected RememberMe to default to false when omitted")
	}

	var reqTrue LoginRequest
	if err := json.Unmarshal([]byte(`{"identifier":"alice","password":"secret","remember_me":true}`), &reqTrue); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !reqTrue.RememberMe {
		t.Error("expected RememberMe to be true when explicitly set")
	}
}

// TestUserErrorsAreDistinctSentinels mirrors the URL/domain/org sentinel
// tests — these errors drive auth error-code mapping (PART 9/11).
func TestUserErrorsAreDistinctSentinels(t *testing.T) {
	sentinels := []error{
		ErrUserNotFound,
		ErrInvalidCredentials,
		ErrUsernameAlreadyExists,
		ErrEmailAlreadyExists,
		ErrUsernameBlocklisted,
		ErrInvalidUsername,
		ErrWeakPassword,
	}

	seen := make(map[string]bool, len(sentinels))
	for _, err := range sentinels {
		if err == nil {
			t.Fatal("found nil sentinel error")
		}
		if err.Error() == "" {
			t.Fatal("found sentinel error with empty message")
		}
		if seen[err.Error()] {
			t.Errorf("duplicate error message found: %q", err.Error())
		}
		seen[err.Error()] = true
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinel %d (%v) unexpectedly matches sentinel %d (%v) via errors.Is", i, a, j, b)
			}
		}
	}
}
