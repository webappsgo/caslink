package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestURLPasswordHashNeverExposedInJSON guards the `json:"-"` tag on
// PasswordHash — a regression here would leak the Argon2id hash to any API
// consumer, which is exactly the kind of struct-tag mistake table-driven
// field checks catch before it ships.
func TestURLPasswordHashNeverExposedInJSON(t *testing.T) {
	hash := "argon2id$secret-hash-value"
	u := URL{
		ID:           1,
		ShortCode:    "abc123",
		LongURL:      "https://example.com",
		PasswordHash: &hash,
	}

	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	if strings.Contains(string(out), hash) {
		t.Fatalf("PasswordHash leaked into JSON output: %s", out)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if _, ok := decoded["PasswordHash"]; ok {
		t.Error("PasswordHash key present in JSON output")
	}
	if _, ok := decoded["password_hash"]; ok {
		t.Error("password_hash key present in JSON output")
	}
}

// TestClickIPHashNeverExposedInJSON guards the same never-expose contract
// for Click.IPHash.
func TestClickIPHashNeverExposedInJSON(t *testing.T) {
	c := Click{
		ID:     1,
		URLID:  1,
		IPHash: "sha256-of-visitor-ip",
	}

	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	if strings.Contains(string(out), c.IPHash) {
		t.Fatalf("IPHash leaked into JSON output: %s", out)
	}
}

// TestURLOptionalFieldsOmittedWhenNil verifies omitempty pointer/slice
// fields are absent from the JSON payload when unset, so clients can
// distinguish "not set" from "set to zero value".
func TestURLOptionalFieldsOmittedWhenNil(t *testing.T) {
	u := URL{
		ID:        1,
		ShortCode: "abc123",
		LongURL:   "https://example.com",
	}

	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	omittedKeys := []string{
		"title", "description", "user_id", "org_id", "expires_at", "tags",
		"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
		"geo_countries", "mobile_url", "desktop_url", "tablet_url",
	}
	for _, key := range omittedKeys {
		if _, ok := decoded[key]; ok {
			t.Errorf("expected %q to be omitted from JSON when nil/empty, got: %v", key, decoded[key])
		}
	}

	// Non-omitempty fields with zero values MUST still be present.
	requiredKeys := []string{"id", "short_code", "long_url", "custom_code", "visibility", "geo_mode", "created_at", "updated_at"}
	for _, key := range requiredKeys {
		if _, ok := decoded[key]; !ok {
			t.Errorf("expected %q to always be present in JSON, but it was omitted", key)
		}
	}
}

// TestURLOptionalFieldsPresentWhenSet is the inverse of the omit test: once
// a pointer field is populated, it must round-trip through JSON correctly.
func TestURLOptionalFieldsPresentWhenSet(t *testing.T) {
	title := "My Link"
	orgID := int64(42)
	expires := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	u := URL{
		ID:        1,
		ShortCode: "abc123",
		LongURL:   "https://example.com",
		Title:     &title,
		OrgID:     &orgID,
		ExpiresAt: &expires,
		Tags:      []string{"news", "campaign"},
	}

	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var roundTripped URL
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if roundTripped.Title == nil || *roundTripped.Title != title {
		t.Errorf("Title round-trip mismatch: got %v, want %q", roundTripped.Title, title)
	}
	if roundTripped.OrgID == nil || *roundTripped.OrgID != orgID {
		t.Errorf("OrgID round-trip mismatch: got %v, want %d", roundTripped.OrgID, orgID)
	}
	if roundTripped.ExpiresAt == nil || !roundTripped.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt round-trip mismatch: got %v, want %v", roundTripped.ExpiresAt, expires)
	}
	if len(roundTripped.Tags) != 2 || roundTripped.Tags[0] != "news" || roundTripped.Tags[1] != "campaign" {
		t.Errorf("Tags round-trip mismatch: got %v", roundTripped.Tags)
	}
}

// TestUpdateURLRequestDistinguishesClearFromUnchanged exercises the
// documented pointer-to-slice contract on UpdateURLRequest: a nil
// Tags/GeoCountries pointer means "leave unchanged", while a pointer to an
// empty slice means "clear the field". This is the core piece of business
// logic encoded in the struct and worth locking down with a regression
// test, since a naive `[]string` field could not express the distinction.
func TestUpdateURLRequestDistinguishesClearFromUnchanged(t *testing.T) {
	// "leave unchanged" — Tags omitted entirely from the JSON payload.
	var leaveUnchanged UpdateURLRequest
	if err := json.Unmarshal([]byte(`{}`), &leaveUnchanged); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if leaveUnchanged.Tags != nil {
		t.Errorf("expected Tags to remain nil (unchanged) when omitted from payload, got: %v", *leaveUnchanged.Tags)
	}

	// "clear" — Tags explicitly set to an empty array.
	var clear UpdateURLRequest
	if err := json.Unmarshal([]byte(`{"tags":[]}`), &clear); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if clear.Tags == nil {
		t.Fatal("expected Tags to be a non-nil pointer to an empty slice (clear), got nil")
	}
	if len(*clear.Tags) != 0 {
		t.Errorf("expected Tags to point to an empty slice, got: %v", *clear.Tags)
	}

	// "set" — Tags explicitly set to a populated array.
	var set UpdateURLRequest
	if err := json.Unmarshal([]byte(`{"tags":["a","b"]}`), &set); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if set.Tags == nil || len(*set.Tags) != 2 {
		t.Errorf("expected Tags to point to a 2-element slice, got: %v", set.Tags)
	}
}

// TestURLErrorsAreDistinctSentinels confirms every exported error is
// non-nil, has a non-empty message, and is distinguishable from the others
// via errors.Is — callers throughout the codebase branch on these.
func TestURLErrorsAreDistinctSentinels(t *testing.T) {
	sentinels := []error{
		ErrURLNotFound,
		ErrCodeAlreadyExists,
		ErrInvalidPassword,
		ErrURLExpired,
		ErrInvalidCustomCode,
		ErrReservedWord,
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
