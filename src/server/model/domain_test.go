package model

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestCustomDomainOptionalFieldsOmittedWhenNil verifies the many
// omitempty pointer fields on CustomDomain (verification/SSL metadata)
// stay absent until populated, so a freshly-added pending domain doesn't
// leak empty SSL/verification detail to API consumers.
func TestCustomDomainOptionalFieldsOmittedWhenNil(t *testing.T) {
	d := CustomDomain{
		ID:                 1,
		OwnerType:          "user",
		OwnerID:            10,
		Domain:             "links.example.com",
		VerificationStatus: "pending",
		Status:             "pending",
	}

	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	omittedKeys := []string{
		"verified_at", "verified_ip", "last_check_at", "ssl_challenge",
		"ssl_provider", "ssl_issued_at", "ssl_expires_at", "ssl_last_error",
		"suspended_reason",
	}
	for _, key := range omittedKeys {
		if _, ok := decoded[key]; ok {
			t.Errorf("expected %q to be omitted from JSON when nil, got: %v", key, decoded[key])
		}
	}

	requiredKeys := []string{"id", "owner_type", "owner_id", "domain", "is_apex", "is_wildcard", "verification_status", "check_count", "ssl_enabled", "ssl_status", "status", "created_at", "updated_at"}
	for _, key := range requiredKeys {
		if _, ok := decoded[key]; !ok {
			t.Errorf("expected %q to always be present in JSON, but it was omitted", key)
		}
	}
}

// TestAddDomainRequestRoundTrip is a boundary check on the request DTO
// used by the custom-domain add endpoint.
func TestAddDomainRequestRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "simple domain", payload: `{"domain":"links.example.com"}`, want: "links.example.com"},
		{name: "empty domain", payload: `{"domain":""}`, want: ""},
		{name: "missing key", payload: `{}`, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req AddDomainRequest
			if err := json.Unmarshal([]byte(tc.payload), &req); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if req.Domain != tc.want {
				t.Errorf("got Domain=%q, want %q", req.Domain, tc.want)
			}
		})
	}
}

// TestDomainErrorsAreDistinctSentinels mirrors the URL package's error
// sentinel test: these errors drive HTTP status mapping in the domain
// service, so they must stay distinct and non-empty.
func TestDomainErrorsAreDistinctSentinels(t *testing.T) {
	sentinels := []error{
		ErrDomainNotFound,
		ErrDomainAlreadyExists,
		ErrDomainNotVerified,
		ErrDomainLimitReached,
		ErrDomainReserved,
		ErrDomainBlockedPattern,
		ErrSSLNotConfigured,
		ErrSSLCertificateExpired,
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
