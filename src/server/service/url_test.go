package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/store"
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
			visibility TEXT NOT NULL DEFAULT 'public',
			tags TEXT,
			utm_source TEXT,
			utm_medium TEXT,
			utm_campaign TEXT,
			utm_term TEXT,
			utm_content TEXT,
			geo_mode TEXT NOT NULL DEFAULT 'none',
			geo_countries TEXT,
			mobile_url TEXT,
			desktop_url TEXT,
			tablet_url TEXT,
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

func TestUpdateURL(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	created, err := svc.CreateURL(ctx, &model.CreateURLRequest{LongURL: "https://example.com/old"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	newURL := "https://example.com/new"
	newTitle := "New Title"
	updated, err := svc.UpdateURL(ctx, created.ShortCode, &model.UpdateURLRequest{
		LongURL: &newURL,
		Title:   &newTitle,
	})
	if err != nil {
		t.Fatalf("UpdateURL failed: %v", err)
	}
	if updated.LongURL != newURL {
		t.Errorf("LongURL = %q, want %q", updated.LongURL, newURL)
	}
	if updated.Title == nil || *updated.Title != newTitle {
		t.Errorf("Title = %v, want %q", updated.Title, newTitle)
	}

	if _, err := svc.UpdateURL(ctx, "does-not-exist", &model.UpdateURLRequest{LongURL: &newURL}); err != model.ErrURLNotFound {
		t.Errorf("UpdateURL on missing code: got %v, want ErrURLNotFound", err)
	}
}

func TestUpdateURLRejectsInvalidURL(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	created, err := svc.CreateURL(ctx, &model.CreateURLRequest{LongURL: "https://example.com/old"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	bad := "not a url"
	if _, err := svc.UpdateURL(ctx, created.ShortCode, &model.UpdateURLRequest{LongURL: &bad}); err == nil {
		t.Error("UpdateURL with invalid URL: got nil error, want error")
	}
}

// TestValidateDestinationURL locks in the http/https-only scheme allowlist —
// a regression here would let javascript:/data:/file: targets be stored and
// served as redirects.
func TestValidateDestinationURL(t *testing.T) {
	valid := []string{
		"https://example.com/x",
		"http://example.com",
		"HTTPS://Example.com/Path?q=1",
	}
	for _, u := range valid {
		if err := validateDestinationURL(u); err != nil {
			t.Errorf("validateDestinationURL(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"ftp://example.com/x",
		"not a url",
		"https://",
		"/relative/path",
	}
	for _, u := range invalid {
		if err := validateDestinationURL(u); err == nil {
			t.Errorf("validateDestinationURL(%q) = nil, want error", u)
		}
	}
}

func TestUpdateURLCanReviveExpiredLink(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	created, err := svc.CreateURL(ctx, &model.CreateURLRequest{LongURL: "https://example.com/old", ExpiresAt: &past})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	if _, err := svc.GetURLByCode(ctx, created.ShortCode); err != model.ErrURLExpired {
		t.Fatalf("GetURLByCode on expired link: got %v, want ErrURLExpired", err)
	}

	future := time.Now().Add(time.Hour)
	if _, err := svc.UpdateURL(ctx, created.ShortCode, &model.UpdateURLRequest{ExpiresAt: &future}); err != nil {
		t.Fatalf("UpdateURL on expired link failed: %v", err)
	}

	if _, err := svc.GetURLByCode(ctx, created.ShortCode); err != nil {
		t.Fatalf("GetURLByCode after reviving link: %v", err)
	}
}

func TestDeleteURL(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	created, err := svc.CreateURL(ctx, &model.CreateURLRequest{LongURL: "https://example.com/old"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	if err := svc.DeleteURL(ctx, created.ShortCode); err != nil {
		t.Fatalf("DeleteURL failed: %v", err)
	}

	if _, err := svc.GetURLByCode(ctx, created.ShortCode); err != model.ErrURLNotFound {
		t.Errorf("GetURLByCode after delete: got %v, want ErrURLNotFound", err)
	}

	if err := svc.DeleteURL(ctx, created.ShortCode); err != model.ErrURLNotFound {
		t.Errorf("DeleteURL on missing code: got %v, want ErrURLNotFound", err)
	}
}

func TestGetURLByCodeForOwner(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	userLink, err := svc.CreateURLForUser(ctx, 7, &model.CreateURLRequest{LongURL: "https://example.com/u"})
	if err != nil {
		t.Fatalf("CreateURLForUser: %v", err)
	}
	orgLink, err := svc.CreateURLForOrg(ctx, 42, &model.CreateURLRequest{LongURL: "https://example.com/o"})
	if err != nil {
		t.Fatalf("CreateURLForOrg: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	expiredLink, err := svc.CreateURLForUser(ctx, 7, &model.CreateURLRequest{LongURL: "https://example.com/x", ExpiresAt: &past})
	if err != nil {
		t.Fatalf("CreateURLForUser(expired): %v", err)
	}

	t.Run("user owner match", func(t *testing.T) {
		got, err := svc.GetURLByCodeForOwner(ctx, userLink.ShortCode, "user", 7)
		if err != nil || got == nil || got.ShortCode != userLink.ShortCode {
			t.Fatalf("owner match = (%+v, %v), want %s", got, err, userLink.ShortCode)
		}
	})

	t.Run("org owner match", func(t *testing.T) {
		got, err := svc.GetURLByCodeForOwner(ctx, orgLink.ShortCode, "org", 42)
		if err != nil || got == nil || got.ShortCode != orgLink.ShortCode {
			t.Fatalf("org match = (%+v, %v), want %s", got, err, orgLink.ShortCode)
		}
	})

	t.Run("wrong owner is not found", func(t *testing.T) {
		if _, err := svc.GetURLByCodeForOwner(ctx, userLink.ShortCode, "user", 8); err != model.ErrURLNotFound {
			t.Errorf("wrong user err = %v, want ErrURLNotFound", err)
		}
		if _, err := svc.GetURLByCodeForOwner(ctx, userLink.ShortCode, "org", 7); err != model.ErrURLNotFound {
			t.Errorf("user link via org scope err = %v, want ErrURLNotFound", err)
		}
	})

	t.Run("invalid owner type is not found", func(t *testing.T) {
		if _, err := svc.GetURLByCodeForOwner(ctx, userLink.ShortCode, "banana", 7); err != model.ErrURLNotFound {
			t.Errorf("invalid ownerType err = %v, want ErrURLNotFound", err)
		}
	})

	t.Run("expired owned link reports expired", func(t *testing.T) {
		if _, err := svc.GetURLByCodeForOwner(ctx, expiredLink.ShortCode, "user", 7); err != model.ErrURLExpired {
			t.Errorf("expired err = %v, want ErrURLExpired", err)
		}
	})
}

func TestParseExpiration(t *testing.T) {
	if got := parseExpiration("never"); got != nil {
		t.Errorf(`parseExpiration("never") = %v, want nil`, got)
	}
	if got := parseExpiration(""); got != nil {
		t.Errorf(`parseExpiration("") = %v, want nil (default never)`, got)
	}
	if got := parseExpiration("bogus"); got != nil {
		t.Errorf(`parseExpiration("bogus") = %v, want nil (default never)`, got)
	}
	got := parseExpiration("1h")
	if got == nil {
		t.Fatal(`parseExpiration("1h") = nil, want a future time`)
	}
	if !got.After(time.Now()) {
		t.Errorf(`parseExpiration("1h") = %v, want a time in the future`, got)
	}
}

// TestCreateURLNeverExpireIsNotInstantlyExpired guards the regression where
// "never"/default expiry produced a year-0001 timestamp that GetURLByCode
// read as already expired, killing the link the instant it was created.
func TestCreateURLNeverExpireIsNotInstantlyExpired(t *testing.T) {
	st := newTestURLStore(t)
	svc := NewURLService(st)
	ctx := context.Background()

	for _, expireAfter := range []string{"never", ""} {
		created, err := svc.CreateURL(ctx, &model.CreateURLRequest{
			LongURL:     "https://example.com/keepme",
			ExpireAfter: expireAfter,
		})
		if err != nil {
			t.Fatalf("CreateURL(ExpireAfter=%q) failed: %v", expireAfter, err)
		}
		if created.ExpiresAt != nil {
			t.Errorf("CreateURL(ExpireAfter=%q): ExpiresAt = %v, want nil", expireAfter, created.ExpiresAt)
		}
		got, err := svc.GetURLByCode(ctx, created.ShortCode)
		if err != nil {
			t.Fatalf("GetURLByCode(ExpireAfter=%q) failed: %v (link was treated as expired on creation)", expireAfter, err)
		}
		if got.LongURL != "https://example.com/keepme" {
			t.Errorf("GetURLByCode LongURL = %q, want the created URL", got.LongURL)
		}
	}
}
