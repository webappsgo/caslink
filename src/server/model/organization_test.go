package model

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestCreateOrgRequestSlugOptional confirms Slug's omitempty/optional
// contract: a request with no slug still unmarshals cleanly, and JSON
// output omits an empty slug (server generates one from Name instead).
func TestCreateOrgRequestSlugOptional(t *testing.T) {
	var req CreateOrgRequest
	if err := json.Unmarshal([]byte(`{"name":"Acme Corp"}`), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if req.Name != "Acme Corp" {
		t.Errorf("got Name=%q, want %q", req.Name, "Acme Corp")
	}
	if req.Slug != "" {
		t.Errorf("expected Slug to default to empty string, got %q", req.Slug)
	}

	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if _, ok := decoded["slug"]; ok {
		t.Errorf("expected empty slug to be omitted from JSON, got: %v", decoded["slug"])
	}
}

// TestUpdateOrgRequestNilVsEmptyName exercises the pointer-field
// leave-unchanged contract on UpdateOrgRequest, matching the same pattern
// used by UpdateURLRequest.
func TestUpdateOrgRequestNilVsEmptyName(t *testing.T) {
	var leaveUnchanged UpdateOrgRequest
	if err := json.Unmarshal([]byte(`{}`), &leaveUnchanged); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if leaveUnchanged.Name != nil {
		t.Errorf("expected Name to remain nil when omitted, got: %q", *leaveUnchanged.Name)
	}

	var rename UpdateOrgRequest
	if err := json.Unmarshal([]byte(`{"name":"New Name"}`), &rename); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if rename.Name == nil || *rename.Name != "New Name" {
		t.Errorf("expected Name to be set to %q, got: %v", "New Name", rename.Name)
	}
}

// TestInviteMemberRequestFields is a basic field-mapping check for the
// invite DTO, since a typo'd json tag here would silently break invites.
func TestInviteMemberRequestFields(t *testing.T) {
	var req InviteMemberRequest
	payload := `{"email":"bob@example.com","role":"admin"}`
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if req.Email != "bob@example.com" {
		t.Errorf("got Email=%q, want %q", req.Email, "bob@example.com")
	}
	if req.Role != "admin" {
		t.Errorf("got Role=%q, want %q", req.Role, "admin")
	}
}

// TestOrgErrorsAreDistinctSentinels mirrors the URL/domain sentinel tests.
func TestOrgErrorsAreDistinctSentinels(t *testing.T) {
	sentinels := []error{
		ErrOrgNotFound,
		ErrOrgSlugAlreadyExists,
		ErrNotOrgMember,
		ErrInsufficientOrgPerms,
		ErrCannotLeaveAsOwner,
		ErrOrgLimitReached,
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
