package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
	_ "modernc.org/sqlite"
)

// newTestDomainStore creates an in-memory SQLite store with just the
// custom_domains table needed by DomainService tests.
func newTestDomainStore(t *testing.T) *store.Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s_domain?mode=memory&cache=shared&_fk=1", t.Name())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `CREATE TABLE IF NOT EXISTS custom_domains (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_type TEXT NOT NULL,
		owner_id INTEGER NOT NULL,
		domain TEXT NOT NULL UNIQUE,
		is_apex BOOLEAN DEFAULT 0,
		is_wildcard BOOLEAN DEFAULT 0,
		verification_status TEXT NOT NULL DEFAULT 'pending',
		verified_at DATETIME,
		verified_ip TEXT,
		last_check_at DATETIME,
		check_count INTEGER DEFAULT 0,
		ssl_enabled BOOLEAN DEFAULT 0,
		ssl_status TEXT NOT NULL DEFAULT 'none',
		ssl_challenge TEXT,
		ssl_provider TEXT,
		ssl_credentials TEXT,
		ssl_cert_pem TEXT,
		ssl_key_pem TEXT,
		ssl_issued_at DATETIME,
		ssl_expires_at DATETIME,
		ssl_last_error TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		suspended_reason TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return &store.Store{ServerDB: db, UsersDB: db}
}

func TestAddDomainIsApexDetection(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st)
	ctx := context.Background()

	cases := []struct {
		domain   string
		wantApex bool
	}{
		{"example.com", true},
		{"www.example.com", false},
		{"api.mycompany.com", false},
		{"localhost", false}, // single label, not a usable apex either way
	}

	for _, tc := range cases {
		cd, err := s.AddDomain(ctx, "user", 1, tc.domain)
		if err != nil {
			t.Fatalf("AddDomain(%q) failed: %v", tc.domain, err)
		}
		if cd.IsApex != tc.wantApex {
			t.Errorf("AddDomain(%q).IsApex = %v, want %v", tc.domain, cd.IsApex, tc.wantApex)
		}
	}
}

func TestIsDomainVerifiedActive(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st)
	ctx := context.Background()

	cd, err := s.AddDomain(ctx, "user", 1, "example.com")
	if err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}

	// Not verified yet — must not be eligible for automatic ACME issuance.
	ok, err := s.IsDomainVerifiedActive(ctx, "example.com")
	if err != nil {
		t.Fatalf("IsDomainVerifiedActive failed: %v", err)
	}
	if ok {
		t.Fatal("expected unverified domain to be ineligible for SSL")
	}

	now := time.Now()
	if _, err := st.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains SET verification_status = 'verified', status = 'active', updated_at = ? WHERE id = ?`,
		now, cd.ID); err != nil {
		t.Fatalf("failed to mark domain verified: %v", err)
	}

	ok, err = s.IsDomainVerifiedActive(ctx, "example.com")
	if err != nil {
		t.Fatalf("IsDomainVerifiedActive failed: %v", err)
	}
	if !ok {
		t.Fatal("expected verified, active, non-wildcard domain to be eligible for SSL")
	}

	// A wildcard domain must never be eligible — only DNS-01 can issue
	// wildcard certs, and DNS-01 is not implemented.
	if _, err := st.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains SET is_wildcard = 1 WHERE id = ?`, cd.ID); err != nil {
		t.Fatalf("failed to mark domain wildcard: %v", err)
	}
	ok, err = s.IsDomainVerifiedActive(ctx, "example.com")
	if err != nil {
		t.Fatalf("IsDomainVerifiedActive failed: %v", err)
	}
	if ok {
		t.Fatal("expected wildcard domain to be ineligible for automatic SSL")
	}

	// Unknown domain.
	ok, err = s.IsDomainVerifiedActive(ctx, "not-registered.example")
	if err != nil {
		t.Fatalf("IsDomainVerifiedActive failed: %v", err)
	}
	if ok {
		t.Fatal("expected unregistered domain to be ineligible")
	}
}
