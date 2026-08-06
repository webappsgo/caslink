package validate

import (
	"strings"
	"testing"
)

func TestValidateOrgSlug(t *testing.T) {
	cases := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{"minimum two chars", "ab", false},
		{"single char rejected", "a", true},
		{"typical slug", "acme-corp", false},
		{"digits and hyphens", "team-42", false},
		{"uppercase normalized", "AcmeCorp", false},
		{"leading whitespace trimmed", "  acme  ", false},
		{"max 39 chars", "a" + strings.Repeat("b", 38), false},
		{"40 chars rejected", "a" + strings.Repeat("b", 39), true},
		{"empty rejected", "", true},
		{"leading hyphen rejected", "-acme", true},
		{"trailing hyphen rejected", "acme-", true},
		{"consecutive hyphens rejected", "ac--me", true},
		{"underscore rejected", "acme_corp", true},
		{"space rejected", "acme corp", true},
		{"reserved name rejected", "admin", true},
		{"reserved project name rejected", "caslink", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOrgSlug(tc.slug)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateOrgSlug(%q) = nil, want error", tc.slug)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateOrgSlug(%q) = %v, want nil", tc.slug, err)
			}
		})
	}
}
