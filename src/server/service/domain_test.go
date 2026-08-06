package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/store"
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
		verification_token TEXT NOT NULL DEFAULT '',
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
	s := NewDomainService(st, config.CustomDomainsConfig{})
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

func TestAddDomainEnforcement(t *testing.T) {
	ctx := context.Background()
	cfg := config.CustomDomainsConfig{
		MaxDomainsPerUser: 2,
		MaxDomainsPerOrg:  20,
		Reserved:          []string{"localhost", "*.local", "*.test", "*.example", "*.invalid"},
		BlockedPatterns:   []string{".*\\.(gov|mil|edu)$"},
	}

	t.Run("reserved rejected", func(t *testing.T) {
		s := NewDomainService(newTestDomainStore(t), cfg)
		for _, d := range []string{"localhost", "foo.local", "bar.test", "acme.example", "x.invalid"} {
			if _, err := s.AddDomain(ctx, "user", 1, d); err != model.ErrDomainReserved {
				t.Errorf("AddDomain(%q) err = %v, want ErrDomainReserved", d, err)
			}
		}
	})

	t.Run("blocked pattern rejected", func(t *testing.T) {
		s := NewDomainService(newTestDomainStore(t), cfg)
		for _, d := range []string{"agency.gov", "base.mil", "school.edu"} {
			if _, err := s.AddDomain(ctx, "user", 1, d); err != model.ErrDomainBlockedPattern {
				t.Errorf("AddDomain(%q) err = %v, want ErrDomainBlockedPattern", d, err)
			}
		}
	})

	t.Run("per-user limit enforced", func(t *testing.T) {
		s := NewDomainService(newTestDomainStore(t), cfg)
		if _, err := s.AddDomain(ctx, "user", 1, "one.com"); err != nil {
			t.Fatalf("AddDomain(one.com) unexpected err: %v", err)
		}
		if _, err := s.AddDomain(ctx, "user", 1, "two.com"); err != nil {
			t.Fatalf("AddDomain(two.com) unexpected err: %v", err)
		}
		if _, err := s.AddDomain(ctx, "user", 1, "three.com"); err != model.ErrDomainLimitReached {
			t.Errorf("AddDomain(three.com) err = %v, want ErrDomainLimitReached", err)
		}
		// A different owner is unaffected by the first owner's count.
		if _, err := s.AddDomain(ctx, "user", 2, "other.com"); err != nil {
			t.Errorf("AddDomain for second user unexpected err: %v", err)
		}
	})

	t.Run("zero limit is unlimited", func(t *testing.T) {
		s := NewDomainService(newTestDomainStore(t), config.CustomDomainsConfig{MaxDomainsPerUser: 0})
		for i := 0; i < 5; i++ {
			if _, err := s.AddDomain(ctx, "user", 1, fmt.Sprintf("d%d.com", i)); err != nil {
				t.Fatalf("AddDomain unexpected err with unlimited quota: %v", err)
			}
		}
	})
}

func TestIsDomainVerifiedActive(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{})
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

// insertDomainRow inserts a custom_domains row directly (bypassing AddDomain's
// validation) so a test can control created_at, status, and verification_status
// precisely.
func insertDomainRow(t *testing.T, st *store.Store, domain, verStatus, status string, createdAt time.Time) int64 {
	t.Helper()
	res, err := st.UsersDB.ExecContext(context.Background(),
		`INSERT INTO custom_domains (owner_type, owner_id, domain, verification_status, verification_token, status, created_at, updated_at)
		 VALUES ('user', 1, ?, ?, 'tok-'||?, ?, ?, ?)`,
		domain, verStatus, domain, status, createdAt, createdAt,
	)
	if err != nil {
		t.Fatalf("insert domain %q: %v", domain, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func countDomains(t *testing.T, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.UsersDB.QueryRow("SELECT COUNT(*) FROM custom_domains").Scan(&n); err != nil {
		t.Fatalf("count domains: %v", err)
	}
	return n
}

func TestCleanupExpiredPendingVerifications(t *testing.T) {
	st := newTestDomainStore(t)
	// 1h verification window.
	s := NewDomainService(st, config.CustomDomainsConfig{VerificationTTL: 3600})
	ctx := context.Background()

	now := time.Now()
	old := now.Add(-2 * time.Hour) // outside the 1h window
	fresh := now.Add(-10 * time.Minute)

	expiredPending := insertDomainRow(t, st, "expired-pending.example", "pending", "pending", old)
	insertDomainRow(t, st, "expired-failed.example", "failed", "pending", old)
	freshPending := insertDomainRow(t, st, "fresh-pending.example", "pending", "pending", fresh)
	activeOld := insertDomainRow(t, st, "active-old.example", "verified", "active", old)

	removed, err := s.CleanupExpiredPendingVerifications(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredPendingVerifications: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2 (the two expired non-active domains)", removed)
	}
	if countDomains(t, st) != 2 {
		t.Fatalf("remaining domains = %d, want 2", countDomains(t, st))
	}

	// The fresh pending and the old-but-active domains must survive.
	for _, id := range []int64{freshPending, activeOld} {
		var one int
		if err := st.UsersDB.QueryRow("SELECT COUNT(*) FROM custom_domains WHERE id = ?", id).Scan(&one); err != nil {
			t.Fatalf("survivor lookup: %v", err)
		}
		if one != 1 {
			t.Errorf("domain id %d should have survived cleanup", id)
		}
	}
	_ = expiredPending
}

func TestRetryPendingVerifications(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{VerificationTTL: 3600})
	ctx := context.Background()

	now := time.Now()
	old := now.Add(-2 * time.Hour)
	fresh := now.Add(-10 * time.Minute)

	// Only the fresh, not-yet-active domain is in the retry window.
	freshPending := insertDomainRow(t, st, "retry-fresh.example.test", "pending", "pending", fresh)
	insertDomainRow(t, st, "retry-expired.example.test", "pending", "pending", old)
	insertDomainRow(t, st, "retry-active.example.test", "verified", "active", fresh)

	retried, err := s.RetryPendingVerifications(ctx)
	if err != nil {
		t.Fatalf("RetryPendingVerifications: %v", err)
	}
	if retried != 1 {
		t.Fatalf("retried = %d, want 1 (only the fresh pending domain)", retried)
	}

	// VerifyDomain ran against a domain with no real TXT record, so it must
	// have recorded a failed check on the fresh pending row.
	var verStatus string
	var checkCount int
	if err := st.UsersDB.QueryRow(
		"SELECT verification_status, check_count FROM custom_domains WHERE id = ?", freshPending,
	).Scan(&verStatus, &checkCount); err != nil {
		t.Fatalf("post-retry lookup: %v", err)
	}
	if verStatus != "failed" {
		t.Errorf("fresh pending verification_status = %q, want failed after unresolvable retry", verStatus)
	}
	if checkCount != 1 {
		t.Errorf("fresh pending check_count = %d, want 1 after one retry", checkCount)
	}
}

// insertWildcardDomain inserts a verified+active wildcard domain directly,
// since AddDomain/insertDomainRow do not set is_wildcard.
func insertWildcardDomain(t *testing.T, st *store.Store, domain string) {
	t.Helper()
	now := time.Now()
	_, err := st.UsersDB.ExecContext(context.Background(),
		`INSERT INTO custom_domains (owner_type, owner_id, domain, is_wildcard, verification_status, verification_token, status, created_at, updated_at)
		 VALUES ('user', 1, ?, 1, 'verified', 'tok', 'active', ?, ?)`,
		domain, now, now,
	)
	if err != nil {
		t.Fatalf("insert wildcard domain %q: %v", domain, err)
	}
}

func TestResolve(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{})
	ctx := context.Background()
	now := time.Now()

	insertDomainRow(t, st, "links.acme.com", "verified", "active", now)
	insertDomainRow(t, st, "pending.acme.com", "pending", "pending", now)
	insertDomainRow(t, st, "suspended.acme.com", "verified", "suspended", now)
	insertWildcardDomain(t, st, "wild.example.com")

	t.Run("verified active resolves with owner", func(t *testing.T) {
		cd, err := s.Resolve(ctx, "links.acme.com")
		if err != nil {
			t.Fatalf("Resolve: unexpected err: %v", err)
		}
		if cd == nil || cd.Domain != "links.acme.com" {
			t.Fatalf("Resolve returned %+v, want domain links.acme.com", cd)
		}
		if cd.OwnerType != "user" || cd.OwnerID != 1 {
			t.Errorf("owner = %s/%d, want user/1", cd.OwnerType, cd.OwnerID)
		}
	})

	t.Run("host normalization (port, case, trailing dot)", func(t *testing.T) {
		for _, host := range []string{"links.acme.com:8080", "LINKS.ACME.COM", "links.acme.com."} {
			cd, err := s.Resolve(ctx, host)
			if err != nil || cd == nil || cd.Domain != "links.acme.com" {
				t.Errorf("Resolve(%q) = (%+v, %v), want links.acme.com", host, cd, err)
			}
		}
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		if _, err := s.Resolve(ctx, "nope.example"); err != ErrDomainNotFound {
			t.Errorf("Resolve(unknown) err = %v, want ErrDomainNotFound", err)
		}
	})

	t.Run("pending is not found", func(t *testing.T) {
		if _, err := s.Resolve(ctx, "pending.acme.com"); err != ErrDomainNotFound {
			t.Errorf("Resolve(pending) err = %v, want ErrDomainNotFound", err)
		}
	})

	t.Run("suspended is not found", func(t *testing.T) {
		if _, err := s.Resolve(ctx, "suspended.acme.com"); err != ErrDomainNotFound {
			t.Errorf("Resolve(suspended) err = %v, want ErrDomainNotFound", err)
		}
	})

	t.Run("wildcard is excluded", func(t *testing.T) {
		if _, err := s.Resolve(ctx, "wild.example.com"); err != ErrDomainNotFound {
			t.Errorf("Resolve(wildcard) err = %v, want ErrDomainNotFound", err)
		}
	})
}

func TestResolveCacheInvalidation(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{})
	ctx := context.Background()
	now := time.Now()

	id := insertDomainRow(t, st, "fresh.acme.com", "pending", "pending", now)

	// Negative result is cached.
	if _, err := s.Resolve(ctx, "fresh.acme.com"); err != ErrDomainNotFound {
		t.Fatalf("Resolve(pending) err = %v, want ErrDomainNotFound", err)
	}

	// Promote to verified+active directly; the cached negative must persist.
	if _, err := st.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains SET verification_status = 'verified', status = 'active' WHERE id = ?`, id); err != nil {
		t.Fatalf("promote domain: %v", err)
	}
	if _, err := s.Resolve(ctx, "fresh.acme.com"); err != ErrDomainNotFound {
		t.Errorf("Resolve after promote (still cached) err = %v, want ErrDomainNotFound", err)
	}

	// After invalidation the fresh state is visible.
	s.invalidateResolveCache()
	cd, err := s.Resolve(ctx, "fresh.acme.com")
	if err != nil || cd == nil || cd.Domain != "fresh.acme.com" {
		t.Errorf("Resolve after invalidate = (%+v, %v), want fresh.acme.com", cd, err)
	}
}
