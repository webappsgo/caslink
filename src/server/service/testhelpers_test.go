package service

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/webappsgo/caslink/src/server/store"
	_ "modernc.org/sqlite"
)

// newFullSchemaStore creates an in-memory SQLite store carrying the entire
// server.db + users.db schema (via store.InitSchema), for tests that need
// more than one or two tables (audit log, analytics, org, auth, admin,
// totp, webauthn, email, bulk, qr, ...). Prefer this over hand-rolling a
// bespoke schema subset when a test touches multiple related tables.
func newFullSchemaStore(t *testing.T) *store.Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s_full?mode=memory&cache=shared&_fk=1", t.Name())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// modernc.org/sqlite does not honor the "_fk=1" DSN parameter, and a
	// pooled connection may not carry the pragma to every physical
	// connection — pin to a single connection and enable it explicitly,
	// matching the production openSQLite() pattern in store/sqlite.go.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign_keys pragma: %v", err)
	}

	st := store.NewTestStore(db)
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	return st
}
