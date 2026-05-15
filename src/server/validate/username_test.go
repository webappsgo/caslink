package validate

import (
	"testing"
)

func TestValidUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		isAdmin  bool
		wantErr  bool
	}{
		// Valid names
		{name: "simple lowercase", username: "alice", wantErr: false},
		{name: "with numbers", username: "alice42", wantErr: false},
		{name: "with underscore", username: "alice_bob", wantErr: false},
		{name: "with hyphen", username: "alice-bob", wantErr: false},
		{name: "exactly 3 chars", username: "abc", wantErr: false},
		{name: "exactly 32 chars", username: "abcdefghijklmnopqrstuvwxyz123456", wantErr: false},

		// Too short
		{name: "1 char", username: "a", wantErr: true},
		{name: "2 chars", username: "ab", wantErr: true},
		{name: "empty", username: "", wantErr: true},

		// Too long
		{name: "33 chars", username: "abcdefghijklmnopqrstuvwxyz1234567", wantErr: true},

		// Starts with non-letter
		{name: "starts with digit", username: "1alice", wantErr: true},
		{name: "starts with underscore", username: "_alice", wantErr: true},
		{name: "starts with hyphen", username: "-alice", wantErr: true},

		// Ends with special char
		{name: "ends with underscore", username: "alice_", wantErr: true},
		{name: "ends with hyphen", username: "alice-", wantErr: true},

		// Mixed case is normalized to lowercase — "Alice" becomes "alice" which is valid
		{name: "mixed case normalized", username: "Alice", wantErr: false},
		// Purely uppercase after normalization still valid if otherwise fine
		{name: "all caps normalized", username: "ALICE", wantErr: false},

		// Special characters
		{name: "space", username: "alice bob", wantErr: true},
		{name: "dot", username: "alice.bob", wantErr: true},
		{name: "at sign", username: "alice@bob", wantErr: true},

		// Consecutive special chars
		{name: "double underscore", username: "alice__bob", wantErr: true},
		{name: "double hyphen", username: "alice--bob", wantErr: true},
		{name: "underscore-hyphen", username: "alice_-bob", wantErr: true},
		{name: "hyphen-underscore", username: "alice-_bob", wantErr: true},

		// Blocklisted words
		{name: "admin exact", username: "admin", wantErr: true},
		{name: "root exact", username: "root", wantErr: true},
		{name: "api exact", username: "api", wantErr: true},

		// Critical terms as substrings
		{name: "contains admin", username: "myadmin1", wantErr: true},
		{name: "contains root", username: "myroot1x", wantErr: true},

		// Admin exempt
		{name: "admin is exempt", username: "admin", isAdmin: true, wantErr: false},
		{name: "blocklisted admin exempt", username: "root", isAdmin: true, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUsername(tc.username, tc.isAdmin)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for username %q, got nil", tc.username)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for username %q: %v", tc.username, err)
			}
		})
	}
}
