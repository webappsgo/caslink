package config

import "testing"

// TestParseBoolTruthy verifies every documented truthy word (case-insensitive,
// with surrounding whitespace) is recognized by ParseBool.
func TestParseBoolTruthy(t *testing.T) {
	truthy := []string{
		"1", "y", "t", "yes", "true", "on", "enable", "enabled",
		"yep", "yup", "yeah", "aye", "si", "oui",
	}
	for _, v := range truthy {
		if !ParseBool(v) {
			t.Errorf("ParseBool(%q) = false, want true", v)
		}
		upper := v
		if got := ParseBool(strUpper(upper)); !got {
			t.Errorf("ParseBool(%q) = false, want true (uppercase)", strUpper(upper))
		}
		if got := ParseBool("  " + v + "  "); !got {
			t.Errorf("ParseBool(%q) = false, want true (padded)", "  "+v+"  ")
		}
	}
}

// TestParseBoolFalsy verifies every documented falsy word, plus any
// unrecognized word, is treated as false by ParseBool (it never errors).
func TestParseBoolFalsy(t *testing.T) {
	falsy := []string{
		"0", "n", "f", "no", "false", "off", "disable", "disabled",
		"nope", "nah", "nay", "nein", "non",
	}
	for _, v := range falsy {
		if ParseBool(v) {
			t.Errorf("ParseBool(%q) = true, want false", v)
		}
	}
}

// TestParseBoolUnrecognizedIsFalse documents ParseBool's behavior of
// silently returning false (not erroring) for garbage input.
func TestParseBoolUnrecognizedIsFalse(t *testing.T) {
	for _, v := range []string{"", "maybe", "banana", "2"} {
		if ParseBool(v) {
			t.Errorf("ParseBool(%q) = true, want false", v)
		}
	}
}

// TestParseBoolDefault covers the empty-string-uses-default path and the
// unrecognized-value-ignores-default path (ParseBool never falls back to
// default for garbage — only for "").
func TestParseBoolDefault(t *testing.T) {
	tests := []struct {
		name  string
		value string
		def   bool
		want  bool
	}{
		{"empty falls back to default true", "", true, true},
		{"empty falls back to default false", "", false, false},
		{"recognized truthy ignores default false", "yes", false, true},
		{"recognized falsy ignores default true", "no", true, false},
		{"unrecognized value ignores default true (returns false)", "banana", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseBoolDefault(tt.value, tt.def); got != tt.want {
				t.Errorf("ParseBoolDefault(%q, %v) = %v, want %v", tt.value, tt.def, got, tt.want)
			}
		})
	}
}

// TestIsTruthy checks the full truthy word list plus rejection of falsy and
// unrecognized values.
func TestIsTruthy(t *testing.T) {
	truthy := []string{
		"1", "y", "t", "yes", "true", "on", "enable", "enabled",
		"yep", "yup", "yeah", "aye", "si", "oui",
	}
	for _, v := range truthy {
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
	falsy := []string{
		"0", "n", "f", "no", "false", "off", "disable", "disabled",
		"nope", "nah", "nay", "nein", "non",
	}
	for _, v := range falsy {
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
	words := []string{
		"1", "y", "t", "yes", "true", "on", "enable", "enabled",
		"yep", "yup", "yeah", "aye", "si", "oui",
		"0", "n", "f", "no", "false", "off", "disable", "disabled",
		"nope", "nah", "nay", "nein", "non",
	}
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
