package store

import (
	"testing"
)

// TestGetConfigValueMissingKeyReturnsFalse covers the documented boundary:
// a missing key returns ("", false, nil) rather than an error.
func TestGetConfigValueMissingKeyReturnsFalse(t *testing.T) {
	st := newSchemaTestStore(t)

	value, ok, err := st.GetConfigValue("does-not-exist")
	if err != nil {
		t.Fatalf("expected no error for missing key, got %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for missing key")
	}
	if value != "" {
		t.Fatalf("expected empty value for missing key, got %q", value)
	}
}

// TestSetConfigValueThenGetConfigValueRoundTrips covers the happy path of
// inserting a fresh key and reading it back.
func TestSetConfigValueThenGetConfigValueRoundTrips(t *testing.T) {
	st := newSchemaTestStore(t)

	if err := st.SetConfigValue("site.name", "Caslink", "admin"); err != nil {
		t.Fatalf("SetConfigValue failed: %v", err)
	}

	value, ok, err := st.GetConfigValue("site.name")
	if err != nil {
		t.Fatalf("GetConfigValue failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true after SetConfigValue")
	}
	if value != "Caslink" {
		t.Fatalf("expected value %q, got %q", "Caslink", value)
	}
}

// TestSetConfigValueUpsertOverwritesExistingKey exercises the ON CONFLICT
// path — writing the same key twice must update the value in place rather
// than erroring on the UNIQUE constraint or leaving two rows.
func TestSetConfigValueUpsertOverwritesExistingKey(t *testing.T) {
	st := newSchemaTestStore(t)

	if err := st.SetConfigValue("port", "8080", "admin"); err != nil {
		t.Fatalf("initial SetConfigValue failed: %v", err)
	}
	if err := st.SetConfigValue("port", "9090", "root"); err != nil {
		t.Fatalf("upsert SetConfigValue failed: %v", err)
	}

	value, ok, err := st.GetConfigValue("port")
	if err != nil {
		t.Fatalf("GetConfigValue failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true after upsert")
	}
	if value != "9090" {
		t.Fatalf("expected upserted value %q, got %q", "9090", value)
	}

	var rowCount int
	if err := st.ServerDB.QueryRow(`SELECT COUNT(*) FROM config WHERE key = 'port'`).Scan(&rowCount); err != nil {
		t.Fatalf("row count query failed: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("expected exactly 1 row for key %q after upsert, got %d", "port", rowCount)
	}

	var updatedBy string
	if err := st.ServerDB.QueryRow(`SELECT updated_by FROM config WHERE key = 'port'`).Scan(&updatedBy); err != nil {
		t.Fatalf("updated_by query failed: %v", err)
	}
	if updatedBy != "root" {
		t.Fatalf("expected updated_by to reflect the latest writer %q, got %q", "root", updatedBy)
	}
}

// TestGetConfigValuesEmptyKeysReturnsEmptyMap covers the zero-argument
// boundary — GetConfigValues() with no keys must short-circuit rather than
// issuing a malformed "IN ()" query.
func TestGetConfigValuesEmptyKeysReturnsEmptyMap(t *testing.T) {
	st := newSchemaTestStore(t)

	result, err := st.GetConfigValues()
	if err != nil {
		t.Fatalf("GetConfigValues() with no keys failed: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}
}

// TestGetConfigValuesReturnsOnlyExistingKeys covers fetching a mix of
// present and absent keys, and a single-key call (verifying the
// placeholder-building loop that starts at index 1 works for length 1).
func TestGetConfigValuesReturnsOnlyExistingKeys(t *testing.T) {
	st := newSchemaTestStore(t)

	if err := st.SetConfigValue("a", "1", "admin"); err != nil {
		t.Fatalf("SetConfigValue(a) failed: %v", err)
	}
	if err := st.SetConfigValue("b", "2", "admin"); err != nil {
		t.Fatalf("SetConfigValue(b) failed: %v", err)
	}

	// Single-key call.
	result, err := st.GetConfigValues("a")
	if err != nil {
		t.Fatalf("GetConfigValues(a) failed: %v", err)
	}
	if len(result) != 1 || result["a"] != "1" {
		t.Fatalf("expected {a: 1}, got %v", result)
	}

	// Mixed present/absent keys.
	result, err = st.GetConfigValues("a", "b", "missing")
	if err != nil {
		t.Fatalf("GetConfigValues(a,b,missing) failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(result), result)
	}
	if result["a"] != "1" || result["b"] != "2" {
		t.Fatalf("expected a=1 b=2, got %v", result)
	}
	if _, ok := result["missing"]; ok {
		t.Fatalf("did not expect a value for missing key")
	}
}
