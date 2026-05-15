package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"

	"github.com/casjaysdevdocker/caslink/src/server/store"
	_ "modernc.org/sqlite"
)

// newTestURLStore creates an in-memory SQLite store for URL tests.
func newTestURLStore(t *testing.T) *store.Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s_url?mode=memory&cache=shared&_fk=1", t.Name())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := []string{
		`CREATE TABLE IF NOT EXISTS urls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			short_code TEXT NOT NULL UNIQUE,
			long_url TEXT NOT NULL,
			title TEXT,
			description TEXT,
			user_id INTEGER,
			org_id INTEGER,
			custom_code BOOLEAN DEFAULT 0,
			password_hash TEXT,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("schema exec failed: %v", err)
		}
	}

	return store.NewTestStore(db)
}

var alphanumericRE = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func TestShortCodeGeneration(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		code, err := svc.generateRandomCode(ctx)
		if err != nil {
			t.Fatalf("generateRandomCode failed on iteration %d: %v", i, err)
		}

		if len(code) < 6 {
			t.Errorf("code %q has length %d, want >= 6", code, len(code))
		}

		if !alphanumericRE.MatchString(code) {
			t.Errorf("code %q contains non-alphanumeric characters", code)
		}
	}
}

func TestShortCodeUniqueness(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	seen := make(map[string]struct{}, 1000)

	for i := 0; i < 1000; i++ {
		code, err := svc.generateRandomCode(ctx)
		if err != nil {
			t.Fatalf("generateRandomCode failed on iteration %d: %v", i, err)
		}

		if _, dup := seen[code]; dup {
			t.Errorf("duplicate code generated: %q at iteration %d", code, i)
		}
		seen[code] = struct{}{}

		// Insert the code so subsequent calls see it as "taken".
		if _, err := st.ServerDB.ExecContext(ctx,
			`INSERT INTO urls (short_code, long_url) VALUES (?, ?)`,
			code, "https://example.com",
		); err != nil {
			t.Fatalf("failed to insert code %q: %v", code, err)
		}
	}
}
