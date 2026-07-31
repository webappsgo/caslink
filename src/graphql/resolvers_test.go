package graphql

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	_ "modernc.org/sqlite"
)

// newTestStore creates an in-memory SQLite store carrying both the urls and
// tokens schema, mirroring the pattern established in
// src/server/service/url_test.go and src/server/service/token_test.go.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())

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
		`CREATE TABLE IF NOT EXISTS tokens (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_type   TEXT NOT NULL,
			owner_id     INTEGER NOT NULL,
			name         TEXT NOT NULL DEFAULT 'default',
			token_hash   TEXT NOT NULL UNIQUE,
			token_prefix TEXT NOT NULL DEFAULT '',
			scope        TEXT NOT NULL DEFAULT 'global',
			expires_at   TIMESTAMP,
			last_used_at TIMESTAMP,
			created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(owner_type, owner_id, name)
		)`,
	}

	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("schema exec failed: %v", err)
		}
	}

	return store.NewTestStore(db)
}

// newTestResolver wires a Resolver against a fresh in-memory store.
func newTestResolver(t *testing.T) (*Resolver, *service.URLService, *service.TokenService) {
	t.Helper()
	st := newTestStore(t)
	urlSvc := service.NewURLService(st)
	tokenSvc := service.NewTokenService(st)
	res := NewResolver(urlSvc, tokenSvc, Info{
		Version:   "test",
		CommitID:  "abc123",
		BuildDate: "2026-07-30",
		Mode:      "development",
	})
	return res, urlSvc, tokenSvc
}

func newRequest(t *testing.T, bearer string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return r
}

func TestExecuteHealthAndVersion(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	result := res.Execute(r, `query { health { status version } version { version commit } }`, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("unexpected errors: %v", result["errors"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", result["data"])
	}

	health, ok := data["health"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected health map, got %T", data["health"])
	}
	if health["status"] != "healthy" {
		t.Errorf("health.status = %v, want healthy", health["status"])
	}
	if health["version"] != "test" {
		t.Errorf("health.version = %v, want test", health["version"])
	}
	if _, present := health["uptime"]; present {
		t.Errorf("uptime should not be present since it was not selected, got %v", health["uptime"])
	}

	version, ok := data["version"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected version map, got %T", data["version"])
	}
	if version["commit"] != "abc123" {
		t.Errorf("version.commit = %v, want abc123", version["commit"])
	}
}

func TestExecuteInvalidQuery(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	result := res.Execute(r, `not a valid query {{{`, nil)
	if _, hasErr := result["errors"]; !hasErr {
		t.Fatal("expected errors for invalid query")
	}
	if result["data"] != nil {
		t.Errorf("expected nil data for invalid query, got %v", result["data"])
	}
}

func TestResolveURLNotFound(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	result := res.Execute(r, `query { url(code: "missing") { short_code long_url } }`, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("not-found should resolve to nil, not an error: %v", result["errors"])
	}
	data := result["data"].(map[string]interface{})
	if data["url"] != nil {
		t.Errorf("expected nil url, got %v", data["url"])
	}
}

func TestResolveURLFound(t *testing.T) {
	res, urlSvc, _ := newTestResolver(t)
	ctx := context.Background()

	created, err := urlSvc.CreateURL(ctx, &model.CreateURLRequest{LongURL: "https://example.com/target"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := newRequest(t, "")
	q := fmt.Sprintf(`query { url(code: "%s") { short_code long_url } }`, created.ShortCode)
	result := res.Execute(r, q, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("unexpected errors: %v", result["errors"])
	}

	data := result["data"].(map[string]interface{})
	got, ok := data["url"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected url map, got %T", data["url"])
	}
	if got["long_url"] != "https://example.com/target" {
		t.Errorf("long_url = %v, want https://example.com/target", got["long_url"])
	}
	if got["short_code"] != created.ShortCode {
		t.Errorf("short_code = %v, want %v", got["short_code"], created.ShortCode)
	}
}

func TestResolveURLsRequiresAuth(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	result := res.Execute(r, `query { urls { short_code } }`, nil)
	if _, hasErr := result["errors"]; !hasErr {
		t.Fatal("expected an error for unauthenticated urls query")
	}
}

func TestResolveURLsForUser(t *testing.T) {
	res, urlSvc, tokenSvc := newTestResolver(t)
	ctx := context.Background()

	if _, err := urlSvc.CreateURLForUser(ctx, 42, &model.CreateURLRequest{LongURL: "https://example.com/a"}); err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}
	if _, err := urlSvc.CreateURLForUser(ctx, 42, &model.CreateURLRequest{LongURL: "https://example.com/b"}); err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}
	if _, err := urlSvc.CreateURLForUser(ctx, 99, &model.CreateURLRequest{LongURL: "https://example.com/other-user"}); err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	plaintext, err := tokenSvc.CreateToken(ctx, 42, "user", "test-token", []string{"read"}, nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	r := newRequest(t, plaintext)
	result := res.Execute(r, `query { urls { long_url } }`, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("unexpected errors: %v", result["errors"])
	}

	data := result["data"].(map[string]interface{})
	list, ok := data["urls"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected urls list, got %T", data["urls"])
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 urls owned by user 42, got %d", len(list))
	}
}

func TestResolveCreateURLAnonymous(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	result := res.Execute(r, `mutation { createURL(input: { url: "https://example.com/new" }) { long_url } }`, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("unexpected errors: %v", result["errors"])
	}

	data := result["data"].(map[string]interface{})
	created, ok := data["createURL"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected createURL map, got %T", data["createURL"])
	}
	if created["long_url"] != "https://example.com/new" {
		t.Errorf("long_url = %v, want https://example.com/new", created["long_url"])
	}
}

func TestResolveCreateURLMissingURL(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	result := res.Execute(r, `mutation { createURL(input: { title: "no url" }) { long_url } }`, nil)
	if _, hasErr := result["errors"]; !hasErr {
		t.Fatal("expected an error when input.url is missing")
	}
}

func TestResolveUpdateAndDeleteURLOwnership(t *testing.T) {
	res, urlSvc, tokenSvc := newTestResolver(t)
	ctx := context.Background()

	owned, err := urlSvc.CreateURLForUser(ctx, 7, &model.CreateURLRequest{LongURL: "https://example.com/owned"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	ownerToken, err := tokenSvc.CreateToken(ctx, 7, "user", "owner-token", []string{"write"}, nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	otherToken, err := tokenSvc.CreateToken(ctx, 8, "user", "other-token", []string{"write"}, nil)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// A different user's token must not be able to update someone else's URL.
	rOther := newRequest(t, otherToken)
	updateQ := fmt.Sprintf(`mutation { updateURL(code: "%s", input: { title: "hijacked" }) { title } }`, owned.ShortCode)
	result := res.Execute(rOther, updateQ, nil)
	if _, hasErr := result["errors"]; !hasErr {
		t.Fatal("expected an error when a non-owner updates a URL")
	}

	// The owner's token must succeed.
	rOwner := newRequest(t, ownerToken)
	result = res.Execute(rOwner, updateQ, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("unexpected errors updating own URL: %v", result["errors"])
	}
	data := result["data"].(map[string]interface{})
	updated := data["updateURL"].(map[string]interface{})
	if updated["title"] != "hijacked" {
		t.Errorf("title = %v, want hijacked", updated["title"])
	}

	// A non-owner must not be able to delete it either.
	deleteQ := fmt.Sprintf(`mutation { deleteURL(code: "%s") }`, owned.ShortCode)
	result = res.Execute(rOther, deleteQ, nil)
	if _, hasErr := result["errors"]; !hasErr {
		t.Fatal("expected an error when a non-owner deletes a URL")
	}

	// The owner can delete it.
	result = res.Execute(rOwner, deleteQ, nil)
	if _, hasErr := result["errors"]; hasErr {
		t.Fatalf("unexpected errors deleting own URL: %v", result["errors"])
	}
	data = result["data"].(map[string]interface{})
	if data["deleteURL"] != true {
		t.Errorf("deleteURL = %v, want true", data["deleteURL"])
	}
}

func TestBearerFromRequestInvalidToken(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "not-a-real-token")

	if _, ok := res.bearerFromRequest(r); ok {
		t.Error("expected bearerFromRequest to reject an invalid token")
	}
}

func TestBearerFromRequestNoHeader(t *testing.T) {
	res, _, _ := newTestResolver(t)
	r := newRequest(t, "")

	if _, ok := res.bearerFromRequest(r); ok {
		t.Error("expected bearerFromRequest to report no token when Authorization header is absent")
	}
}
