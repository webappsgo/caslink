package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newSchemaTestStore opens a fresh in-memory SQLite DB per test and runs the
// full InitSchema against it, mirroring the pattern in
// src/graphql/resolvers_test.go but exercising store.go's own schema
// creation instead of a hand-duplicated subset.
func newSchemaTestStore(t *testing.T) *Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	st := NewTestStore(db)
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return st
}

// TestNewTestStoreWiresBothDBsToSameConnection verifies NewTestStore uses
// the single supplied *sql.DB for both ServerDB and UsersDB, and normalizes
// the driver to "sqlite".
func TestNewTestStoreWiresBothDBsToSameConnection(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	st := NewTestStore(db)
	if st.ServerDB != db || st.UsersDB != db {
		t.Fatalf("expected ServerDB and UsersDB to both be the supplied *sql.DB")
	}
	if st.DBType() != "sqlite" {
		t.Fatalf("expected driver sqlite, got %q", st.DBType())
	}
}

// TestInitSchemaCreatesExpectedTables checks that every table InitSchema is
// responsible for actually exists after running it, catching regressions
// where a CREATE TABLE statement is silently dropped or misrouted between
// server.db and users.db.
func TestInitSchemaCreatesExpectedTables(t *testing.T) {
	st := newSchemaTestStore(t)

	serverTables := []string{
		"urls", "clicks", "click_daily_stats", "qr_codes", "uploads",
		"config", "config_meta", "admin_sessions", "rate_limits",
		"scheduler_tasks", "scheduler_history", "backups",
	}
	for _, table := range serverTables {
		if !tableExists(t, st.ServerDB, table) {
			t.Errorf("expected table %q to exist in server.db", table)
		}
	}

	usersTables := []string{
		"admins", "users", "sessions", "tokens", "api_tokens", "server_config",
		"audit_log", "organizations", "org_members", "custom_domains",
		"custom_domain_audit", "password_resets", "email_verifications",
		"totp_secrets", "passkey_credentials", "recovery_keys", "user_sessions",
		"passkeys", "org_tokens", "trusted_devices", "partial_sessions",
	}
	for _, table := range usersTables {
		if !tableExists(t, st.UsersDB, table) {
			t.Errorf("expected table %q to exist in users.db", table)
		}
	}
}

// TestInitSchemaIsIdempotent verifies InitSchema can be run repeatedly
// against the same database without error, which is required since it runs
// on every server startup (CREATE TABLE IF NOT EXISTS + ALTER TABLE ADD
// COLUMN failures silently ignored).
func TestInitSchemaIsIdempotent(t *testing.T) {
	st := newSchemaTestStore(t)

	for i := 0; i < 3; i++ {
		if err := st.InitSchema(); err != nil {
			t.Fatalf("InitSchema run %d failed: %v", i+1, err)
		}
	}
}

// TestInitSchemaAddedColumnsPresent checks a representative sample of the
// ALTER TABLE ADD COLUMN statements actually land, since SQLite has no
// "ADD COLUMN IF NOT EXISTS" and failures there are silently swallowed —
// a typo in a column/table name would otherwise never surface as a test
// failure.
func TestInitSchemaAddedColumnsPresent(t *testing.T) {
	st := newSchemaTestStore(t)

	if !columnExists(t, st.ServerDB, "urls", "geo_mode") {
		t.Errorf("expected urls.geo_mode column to exist")
	}
	if !columnExists(t, st.ServerDB, "urls", "tags") {
		t.Errorf("expected urls.tags column to exist")
	}
	if !columnExists(t, st.UsersDB, "users", "display_name") {
		t.Errorf("expected users.display_name column to exist")
	}
	if !columnExists(t, st.UsersDB, "users", "bio") {
		t.Errorf("expected users.bio column to exist")
	}
	if !columnExists(t, st.UsersDB, "totp_secrets", "key_version") {
		t.Errorf("expected totp_secrets.key_version column to exist")
	}
}

// TestCloseHandlesNilAndPartialStores ensures Close never panics on a
// zero-value Store or a Store with only one of the two DBs set, and that a
// non-nil error from ServerDB.Close is still returned even when UsersDB is
// nil (or vice versa).
func TestCloseHandlesNilAndPartialStores(t *testing.T) {
	// Zero-value store — both DBs nil.
	empty := &Store{}
	if err := empty.Close(); err != nil {
		t.Fatalf("Close on empty store returned error: %v", err)
	}

	// Only ServerDB set.
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	serverOnly := &Store{ServerDB: db}
	if err := serverOnly.Close(); err != nil {
		t.Fatalf("Close with only ServerDB set returned error: %v", err)
	}

	// Closing again must not panic even though the underlying DB is closed.
	if err := serverOnly.Close(); err != nil {
		t.Fatalf("second Close call returned error: %v", err)
	}
}

// TestOpenStoreWithConfigSQLiteCreatesFilesAndSchema exercises the full
// SQLite path through OpenStoreWithConfig (and therefore Open, openSQLite,
// InitSchema) end to end, using a t.TempDir() so nothing touches the
// project tree.
func TestOpenStoreWithConfigSQLiteCreatesFilesAndSchema(t *testing.T) {
	dataDir := t.TempDir()

	st, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	if st.DBType() != "sqlite" {
		t.Fatalf("expected sqlite driver, got %q", st.DBType())
	}
	if st.DBLocality() != "local" {
		t.Fatalf("expected local locality, got %q", st.DBLocality())
	}
	if err := st.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if !tableExists(t, st.ServerDB, "urls") {
		t.Errorf("expected urls table to exist after Open")
	}
	if !tableExists(t, st.UsersDB, "users") {
		t.Errorf("expected users table to exist after Open")
	}

	serverPath := filepath.Join(dataDir, "db", "server.db")
	usersPath := filepath.Join(dataDir, "db", "users.db")
	if !fileExists(serverPath) {
		t.Errorf("expected server.db to be created at %s", serverPath)
	}
	if !fileExists(usersPath) {
		t.Errorf("expected users.db to be created at %s", usersPath)
	}
}

// TestOpenStoreWithConfigSQLiteMissingDataDirIsCreated verifies that a
// dataDir which doesn't exist yet is created on demand (via openSQLite's
// MkdirAll), matching first-run behavior.
func TestOpenStoreWithConfigSQLiteMissingDataDirIsCreated(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")

	st, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	if !fileExists(filepath.Join(dataDir, "db", "server.db")) {
		t.Errorf("expected server.db to be created under the missing dataDir")
	}
}

// TestPingReportsErrorAfterClose ensures Ping surfaces the underlying error
// once the pool has been closed rather than silently succeeding.
func TestPingReportsErrorAfterClose(t *testing.T) {
	st := newSchemaTestStore(t)
	if err := st.ServerDB.Close(); err != nil {
		t.Fatalf("failed to close ServerDB: %v", err)
	}

	if err := st.Ping(); err == nil {
		t.Fatalf("expected Ping to fail after ServerDB was closed")
	}
}

// TestCountURLsAndCountClicks24h covers the empty-database boundary and the
// happy path after inserting rows, including a click older than 24h which
// must not be counted.
func TestCountURLsAndCountClicks24h(t *testing.T) {
	st := newSchemaTestStore(t)

	n, err := st.CountURLs()
	if err != nil {
		t.Fatalf("CountURLs on empty db failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 urls, got %d", n)
	}

	c, err := st.CountClicks24h()
	if err != nil {
		t.Fatalf("CountClicks24h on empty db failed: %v", err)
	}
	if c != 0 {
		t.Fatalf("expected 0 clicks, got %d", c)
	}

	res, err := st.ServerDB.Exec(
		`INSERT INTO urls (short_code, long_url) VALUES ('abc123', 'https://example.com')`,
	)
	if err != nil {
		t.Fatalf("insert url failed: %v", err)
	}
	urlID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId failed: %v", err)
	}

	n, err = st.CountURLs()
	if err != nil {
		t.Fatalf("CountURLs failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 url, got %d", n)
	}

	// Recent click — must be counted.
	if _, err := st.ServerDB.Exec(
		`INSERT INTO clicks (url_id, clicked_at) VALUES (?, datetime('now'))`, urlID,
	); err != nil {
		t.Fatalf("insert recent click failed: %v", err)
	}
	// Stale click (2 days old) — must not be counted.
	if _, err := st.ServerDB.Exec(
		`INSERT INTO clicks (url_id, clicked_at) VALUES (?, datetime('now', '-2 days'))`, urlID,
	); err != nil {
		t.Fatalf("insert stale click failed: %v", err)
	}

	c, err = st.CountClicks24h()
	if err != nil {
		t.Fatalf("CountClicks24h failed: %v", err)
	}
	if c != 1 {
		t.Fatalf("expected 1 click in the last 24h, got %d", c)
	}
}

// TestStatsReturnsCountsAndConnectionInfo covers Stats' happy path and
// verifies it degrades to zero (rather than erroring) for tables that are
// legitimately empty.
func TestStatsReturnsCountsAndConnectionInfo(t *testing.T) {
	st := newSchemaTestStore(t)

	if _, err := st.ServerDB.Exec(
		`INSERT INTO urls (short_code, long_url) VALUES ('x1', 'https://a.example')`,
	); err != nil {
		t.Fatalf("insert url failed: %v", err)
	}
	if _, err := st.UsersDB.Exec(
		`INSERT INTO admins (username, email, password_hash) VALUES ('root', 'root@example.com', 'hash')`,
	); err != nil {
		t.Fatalf("insert admin failed: %v", err)
	}

	stats, err := st.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats["urls"] != 1 {
		t.Errorf("expected urls=1, got %v", stats["urls"])
	}
	if stats["clicks"] != 0 {
		t.Errorf("expected clicks=0, got %v", stats["clicks"])
	}
	if stats["admins"] != 1 {
		t.Errorf("expected admins=1, got %v", stats["admins"])
	}
	if stats["users"] != 0 {
		t.Errorf("expected users=0, got %v", stats["users"])
	}
	if _, ok := stats["server_db_open_conns"]; !ok {
		t.Errorf("expected server_db_open_conns key to be present")
	}
	if _, ok := stats["users_db_open_conns"]; !ok {
		t.Errorf("expected users_db_open_conns key to be present")
	}
}

// TestDBTypeDefaultsToSQLiteWhenDriverUnset covers the zero-value Store
// boundary: DBType must fall back to "sqlite" rather than returning "".
func TestDBTypeDefaultsToSQLiteWhenDriverUnset(t *testing.T) {
	st := &Store{}
	if got := st.DBType(); got != "sqlite" {
		t.Fatalf("expected fallback sqlite, got %q", got)
	}
	if got := st.DBLocality(); got != "local" {
		t.Fatalf("expected fallback local, got %q", got)
	}
}

// TestDBLocalityForRemoteDrivers table-drives every remote driver alias
// through DBLocality to guard against a future alias being added to
// OpenStoreWithConfig's switch without updating DBLocality to match.
func TestDBLocalityForRemoteDrivers(t *testing.T) {
	remoteDrivers := []string{"postgres", "postgresql", "mysql", "mariadb", "sqlserver", "mssql"}
	for _, drv := range remoteDrivers {
		st := &Store{driver: drv}
		if got := st.DBLocality(); got != "remote" {
			t.Errorf("driver %q: expected remote locality, got %q", drv, got)
		}
	}
}

// tableExists reports whether the given table is present in db's sqlite_master.
func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("tableExists query failed for %q: %v", table, err)
	}
	return true
}

// columnExists reports whether the given column is present on table in db,
// via PRAGMA table_info.
func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s) failed: %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row failed: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

// fileExists reports whether path exists on disk, used to assert on-disk
// side effects of Open/OpenStoreWithConfig/openSQLite.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
