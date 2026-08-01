package handler

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/caslink/src/server/store"
)

// newSchemaTestStore opens a fresh in-memory SQLite DB per test and runs the
// full InitSchema against it, mirroring src/server/store/store_test.go and
// src/graphql/resolvers_test.go so handler tests exercise the real schema
// instead of a hand-duplicated subset.
func newSchemaTestStore(t *testing.T) *store.Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	st := store.NewTestStore(db)
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return st
}
