package config

import "testing"

// truthyWords and falsyWords mirror the full documented word lists in
// AI.md's "Boolean Handling" section (config-rules.md / testing-rules.md).
var truthyWords = []string{
	"1", "y", "t", "yes", "true", "on", "ok", "enable", "enabled",
	"yep", "yup", "yeah", "aye", "si", "oui", "da", "hai", "affirmative",
	"accept", "allow", "grant", "sure", "totally",
}

var falsyWords = []string{
	"0", "n", "f", "no", "false", "off", "disable", "disabled",
	"nope", "nah", "nay", "nein", "non", "niet", "iie", "lie",
	"negative", "reject", "block", "revoke", "deny", "never", "noway",
}

// TestParseBoolTruthy verifies every documented truthy word (case-insensitive,
// with surrounding whitespace) is recognized by ParseBool.
func TestParseBoolTruthy(t *testing.T) {
	for _, v := range truthyWords {
		if got, err := ParseBool(v, false); err != nil || !got {
			t.Errorf("ParseBool(%q, false) = %v, %v, want true, nil", v, got, err)
		}
		if got, err := ParseBool(strUpper(v), false); err != nil || !got {
			t.Errorf("ParseBool(%q, false) = %v, %v, want true, nil (uppercase)", strUpper(v), got, err)
		}
		if got, err := ParseBool("  "+v+"  ", false); err != nil || !got {
			t.Errorf("ParseBool(%q, false) = %v, %v, want true, nil (padded)", "  "+v+"  ", got, err)
		}
	}
}

// TestParseBoolFalsy verifies every documented falsy word is recognized by
// ParseBool without error.
func TestParseBoolFalsy(t *testing.T) {
	for _, v := range falsyWords {
		if got, err := ParseBool(v, true); err != nil || got {
			t.Errorf("ParseBool(%q, true) = %v, %v, want false, nil", v, got, err)
		}
	}
}

// TestParseBoolInvalidReturnsError verifies ParseBool errors (rather than
// silently defaulting) for unrecognized, non-empty input.
func TestParseBoolInvalidReturnsError(t *testing.T) {
	for _, v := range []string{"maybe", "banana", "2"} {
		if _, err := ParseBool(v, true); err == nil {
			t.Errorf("ParseBool(%q, true) returned nil error, want error", v)
		}
	}
}

// TestParseBoolEmptyUsesDefault covers the empty-string-uses-default path.
func TestParseBoolEmptyUsesDefault(t *testing.T) {
	tests := []struct {
		name string
		def  bool
		want bool
	}{
		{"empty falls back to default true", true, true},
		{"empty falls back to default false", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBool("", tt.def)
			if err != nil {
				t.Fatalf("ParseBool(\"\", %v) returned error: %v", tt.def, err)
			}
			if got != tt.want {
				t.Errorf("ParseBool(\"\", %v) = %v, want %v", tt.def, got, tt.want)
			}
		})
	}
}

// TestMustParseBool verifies MustParseBool returns the parsed value for
// valid input and panics for invalid input.
func TestMustParseBool(t *testing.T) {
	if got := MustParseBool("yes", false); !got {
		t.Errorf("MustParseBool(%q, false) = false, want true", "yes")
	}
	if got := MustParseBool("", true); !got {
		t.Errorf("MustParseBool(\"\", true) = false, want true")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseBool(\"banana\", false) did not panic")
		}
	}()
	MustParseBool("banana", false)
}

// TestIsTruthy checks the full truthy word list plus rejection of falsy and
// unrecognized values.
func TestIsTruthy(t *testing.T) {
	for _, v := range truthyWords {
		if !IsTruthy(v) {
			t.Errorf("IsTruthy(%q) = false, want true", v)
		}
	}
	notTruthy := []string{"0", "no", "false", "", "banana", "2"}
	for _, v := range notTruthy {
		if IsTruthy(v) {
			t.Errorf("IsTruthy(%q) = true, want false", v)
		}
	}
}

// TestIsFalsy checks the full falsy word list plus rejection of truthy and
// unrecognized values.
func TestIsFalsy(t *testing.T) {
	for _, v := range falsyWords {
		if !IsFalsy(v) {
			t.Errorf("IsFalsy(%q) = false, want true", v)
		}
	}
	notFalsy := []string{"1", "yes", "true", "", "banana", "2"}
	for _, v := range notFalsy {
		if IsFalsy(v) {
			t.Errorf("IsFalsy(%q) = true, want false", v)
		}
	}
}

// TestIsTruthyIsFalsyMutuallyExclusive ensures no word is ever classified
// as both truthy and falsy at once, across the full combined word list.
func TestIsTruthyIsFalsyMutuallyExclusive(t *testing.T) {
	words := append(append([]string{}, truthyWords...), falsyWords...)
	for _, v := range words {
		if IsTruthy(v) && IsFalsy(v) {
			t.Errorf("%q classified as both truthy and falsy", v)
		}
	}
}

// strUpper is a tiny local helper avoiding an extra import of strings in
// the truthy test's uppercase check.
func strUpper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 32
		}
	}
	return string(out)
}
