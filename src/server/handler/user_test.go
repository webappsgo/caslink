package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// newUserTestHandler builds a UserHandler backed by real, in-memory-SQLite
// services (AuthService, TokenService, URLService) and a real template
// renderer, mirroring the pattern in auth_user_test.go.
func newUserTestHandler(t *testing.T) (*UserHandler, *service.AuthService, *service.TokenService, *service.URLService, *store.Store) {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	tokenService := service.NewTokenService(st)
	urlService := service.NewURLService(st)
	cfg := config.DefaultConfig()
	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	return NewUserHandler(authService, tokenService, urlService, renderer, cfg), authService, tokenService, urlService, st
}

// seedUser registers a real user via AuthService so handler tests exercise
// actual DB-backed lookups instead of hand-built structs.
func seedUser(t *testing.T, authService *service.AuthService, username, email string) *service.User {
	t.Helper()
	user, err := authService.RegisterUser(context.Background(), username, email, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("seed RegisterUser(%s) failed: %v", username, err)
	}
	return user
}

// withUser and withBearer (attaching auth context the same way
// UserAuthMiddleware/BearerAuthMiddleware would) are already defined in
// bulk_test.go / url_test.go and reused here.

// ---- Profile / Settings / Security page tests ----

func TestProfileUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/users", nil)
	w := httptest.NewRecorder()
	h.Profile(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestProfileAuthorized(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "alice", "alice@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users", nil), user)
	w := httptest.NewRecorder()
	h.Profile(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSettingsUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	w := httptest.NewRecorder()
	h.Settings(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSettingsAuthorized(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "bob", "bob@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/settings", nil), user)
	w := httptest.NewRecorder()
	h.Settings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecurityUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/users/security", nil)
	w := httptest.NewRecorder()
	h.Security(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSecurityAuthorized(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "carl", "carl@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security", nil), user)
	w := httptest.NewRecorder()
	h.Security(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- Tokens page (GET/POST) tests ----

func TestTokensGetUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/users/tokens", nil)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestTokensGetAuthorizedEmptyList(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "dana", "dana@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/tokens", nil), user)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTokensPostInvalidFormBadQueryEscape(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "erin", "erin@example.com")

	// An invalid %-escape in the query string makes r.ParseForm() itself
	// return an error, exercising the "Invalid form" 400 branch.
	r := httptest.NewRequest(http.MethodPost, "/users/tokens?a=%zz", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTokensPostCreateEmptyNameRerendersForm(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "frank", "frank@example.com")

	form := "action=create&token_name=+"
	r := httptest.NewRequest(http.MethodPost, "/users/tokens", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	// Validation failures re-render the page (200), they do not redirect.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTokensPostCreateSuccess(t *testing.T) {
	h, authService, tokenService, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "gina", "gina@example.com")

	form := "action=create&token_name=my-token&expires_in=30"
	r := httptest.NewRequest(http.MethodPost, "/users/tokens", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tokens, err := tokenService.ListTokens(context.Background(), "user", user.ID)
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token to be created, got %d", len(tokens))
	}
	if tokens[0].Name != "my-token" {
		t.Errorf("expected token name %q, got %q", "my-token", tokens[0].Name)
	}
}

func TestTokensPostRevokeInvalidID(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "hank", "hank@example.com")

	form := "action=revoke&token_id=not-a-number"
	r := httptest.NewRequest(http.MethodPost, "/users/tokens", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered form with flash), got %d: %s", w.Code, w.Body.String())
	}
}

func TestTokensPostRevokeSuccessRedirects(t *testing.T) {
	h, authService, tokenService, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "ivy", "ivy@example.com")

	_, err := tokenService.CreateToken(context.Background(), user.ID, "user", "to-revoke", nil, nil)
	if err != nil {
		t.Fatalf("seed CreateToken failed: %v", err)
	}
	tokens, _ := tokenService.ListTokens(context.Background(), "user", user.ID)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 seeded token, got %d", len(tokens))
	}

	form := "action=revoke&token_id=" + itoa64(tokens[0].ID)
	r := httptest.NewRequest(http.MethodPost, "/users/tokens", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/users/tokens" {
		t.Errorf("expected redirect to /users/tokens, got %q", loc)
	}

	remaining, _ := tokenService.ListTokens(context.Background(), "user", user.ID)
	if len(remaining) != 0 {
		t.Errorf("expected token to be revoked, still have %d", len(remaining))
	}
}

func TestTokensPostRevokeWrongOwnerFails(t *testing.T) {
	h, authService, tokenService, _, _ := newUserTestHandler(t)
	owner := seedUser(t, authService, "jack", "jack@example.com")
	attacker := seedUser(t, authService, "jill", "jill@example.com")

	_, err := tokenService.CreateToken(context.Background(), owner.ID, "user", "owners-token", nil, nil)
	if err != nil {
		t.Fatalf("seed CreateToken failed: %v", err)
	}
	tokens, _ := tokenService.ListTokens(context.Background(), "user", owner.ID)

	form := "action=revoke&token_id=" + itoa64(tokens[0].ID)
	r := httptest.NewRequest(http.MethodPost, "/users/tokens", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, attacker)
	w := httptest.NewRecorder()
	h.Tokens(w, r)

	// RevokeToken scopes by ownerID so the attacker's revoke is rejected —
	// the handler re-renders with a failure flash (200), not a redirect.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (revoke failed, form re-rendered), got %d", w.Code)
	}

	remaining, _ := tokenService.ListTokens(context.Background(), "user", owner.ID)
	if len(remaining) != 1 {
		t.Errorf("expected owner's token to survive a cross-owner revoke attempt, got %d remaining", len(remaining))
	}
}

// itoa64 is a tiny local helper for building form bodies from int64 IDs.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// ---- currentAPIUser / APIProfile ----

func TestAPIProfileUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	h.APIProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPIProfileSessionAuthorized(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "kim", "kim@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), user)
	w := httptest.NewRecorder()
	h.APIProfile(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data.Username != "kim" {
		t.Errorf("expected username kim, got %q", env.Data.Username)
	}
}

func TestAPIProfileBearerUserToken(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "leo", "leo@example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), rec)
	w := httptest.NewRecorder()
	h.APIProfile(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIProfileBearerNonUserTokenRejected(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "mia", "mia@example.com")

	rec := &service.TokenRecord{OwnerType: "admin", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), rec)
	w := httptest.NewRecorder()
	h.APIProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPIProfileBearerUnknownUserRejected(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: 999999}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), rec)
	w := httptest.NewRecorder()
	h.APIProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

// ---- APIUpdateProfile ----

func TestAPIUpdateProfileUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/users", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.APIUpdateProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPIUpdateProfileInvalidJSON(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "nora", "nora@example.com")

	r := withUser(httptest.NewRequest(http.MethodPatch, "/api/v1/users", strings.NewReader(`{not json`)), user)
	w := httptest.NewRecorder()
	h.APIUpdateProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusBadRequest)
}

func TestAPIUpdateProfileNoFields(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "oleg", "oleg@example.com")

	r := withUser(httptest.NewRequest(http.MethodPatch, "/api/v1/users", strings.NewReader(`{}`)), user)
	w := httptest.NewRecorder()
	h.APIUpdateProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusBadRequest)
}

func TestAPIUpdateProfileSuccess(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "pete", "pete@example.com")

	body := `{"display_name":"Pete P."}`
	r := withUser(httptest.NewRequest(http.MethodPatch, "/api/v1/users", strings.NewReader(body)), user)
	w := httptest.NewRecorder()
	h.APIUpdateProfile(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			DisplayName *string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data.DisplayName == nil || *env.Data.DisplayName != "Pete P." {
		t.Errorf("expected display_name to be updated, got %v", env.Data.DisplayName)
	}
}

func TestAPIUpdateProfileEmailConflict(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	seedUser(t, authService, "quinn", "quinn@example.com")
	user2 := seedUser(t, authService, "rita", "rita@example.com")

	body := `{"email":"quinn@example.com"}`
	r := withUser(httptest.NewRequest(http.MethodPatch, "/api/v1/users", strings.NewReader(body)), user2)
	w := httptest.NewRecorder()
	h.APIUpdateProfile(w, r)

	assertAPIErrorStatus(t, w, http.StatusConflict)
}

// ---- APICreateToken ----

func TestAPICreateTokenUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/tokens", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.APICreateToken(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPICreateTokenInvalidJSON(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "sam", "sam@example.com")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/tokens", strings.NewReader(`{bad`)), user)
	w := httptest.NewRecorder()
	h.APICreateToken(w, r)

	assertAPIErrorStatus(t, w, http.StatusBadRequest)
}

func TestAPICreateTokenEmptyName(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "tara", "tara@example.com")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/tokens", strings.NewReader(`{"name":"   "}`)), user)
	w := httptest.NewRecorder()
	h.APICreateToken(w, r)

	assertAPIErrorStatus(t, w, http.StatusBadRequest)
}

func TestAPICreateTokenSuccess(t *testing.T) {
	h, authService, tokenService, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "uma", "uma@example.com")

	body := `{"name":"cli-token","expires_in_days":7}`
	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/tokens", strings.NewReader(body)), user)
	w := httptest.NewRecorder()
	h.APICreateToken(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data.Token == "" {
		t.Error("expected a non-empty plaintext token in the response")
	}

	tokens, _ := tokenService.ListTokens(context.Background(), "user", user.ID)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 stored token, got %d", len(tokens))
	}
}

// ---- APIRevokeToken ----

func TestAPIRevokeTokenUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/users/tokens/1", nil)
	w := httptest.NewRecorder()
	h.APIRevokeToken(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPIRevokeTokenInvalidID(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "vic", "vic@example.com")

	r := withChiURLParam(withUser(httptest.NewRequest(http.MethodDelete, "/api/v1/users/tokens/abc", nil), user), "id", "abc")
	w := httptest.NewRecorder()
	h.APIRevokeToken(w, r)

	assertAPIErrorStatus(t, w, http.StatusBadRequest)
}

func TestAPIRevokeTokenNotFound(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "wade", "wade@example.com")

	r := withChiURLParam(withUser(httptest.NewRequest(http.MethodDelete, "/api/v1/users/tokens/999999", nil), user), "id", "999999")
	w := httptest.NewRecorder()
	h.APIRevokeToken(w, r)

	assertAPIErrorStatus(t, w, http.StatusNotFound)
}

func TestAPIRevokeTokenSuccess(t *testing.T) {
	h, authService, tokenService, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "xena", "xena@example.com")

	_, err := tokenService.CreateToken(context.Background(), user.ID, "user", "api-token", nil, nil)
	if err != nil {
		t.Fatalf("seed CreateToken failed: %v", err)
	}
	tokens, _ := tokenService.ListTokens(context.Background(), "user", user.ID)

	r := withChiURLParam(withUser(httptest.NewRequest(http.MethodDelete, "/api/v1/users/tokens/"+itoa64(tokens[0].ID), nil), user), "id", itoa64(tokens[0].ID))
	w := httptest.NewRecorder()
	h.APIRevokeToken(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	remaining, _ := tokenService.ListTokens(context.Background(), "user", user.ID)
	if len(remaining) != 0 {
		t.Errorf("expected token to be revoked, got %d remaining", len(remaining))
	}
}

// ---- APITokens / APISettings / APISecurity ----

func TestAPITokensUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/tokens", nil)
	w := httptest.NewRecorder()
	h.APITokens(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPITokensEmptySuccess(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "yuki", "yuki@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/api/v1/users/tokens", nil), user)
	w := httptest.NewRecorder()
	h.APITokens(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(env.Data) != 0 {
		t.Errorf("expected empty token list, got %d", len(env.Data))
	}
}

func TestAPISettingsUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/settings", nil)
	w := httptest.NewRecorder()
	h.APISettings(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPISettingsSuccess(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "zack", "zack@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/api/v1/users/settings", nil), user)
	w := httptest.NewRecorder()
	h.APISettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPISecurityUnauthorized(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/security", nil)
	w := httptest.NewRecorder()
	h.APISecurity(w, r)

	assertAPIErrorStatus(t, w, http.StatusUnauthorized)
}

func TestAPISecurityReflectsTOTPState(t *testing.T) {
	h, authService, _, _, st := newUserTestHandler(t)
	user := seedUser(t, authService, "abel", "abel@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/api/v1/users/security", nil), user)
	w := httptest.NewRecorder()
	h.APISecurity(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			TOTPEnabled bool `json:"totp_enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data.TOTPEnabled {
		t.Error("expected totp_enabled=false before TOTP is enabled")
	}

	totpService := service.NewTOTPService(st, nil, 0)
	secret, err := totpService.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if _, err := totpService.EnableTOTP(user.ID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	// getUserFromRequest returns the *same* pre-fetched user struct that was
	// attached to the context, so it will NOT reflect the TOTP change made
	// after that context was built. Re-fetch a fresh user via GetUserByID to
	// exercise the up-to-date value through the Bearer path instead.
	fresh, err := authService.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if !fresh.TOTPEnabled {
		t.Fatal("expected TOTPEnabled=true after EnableTOTP on a freshly-fetched user")
	}

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r2 := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users/security", nil), rec)
	w2 := httptest.NewRecorder()
	h.APISecurity(w2, r2)

	var env2 struct {
		Data struct {
			TOTPEnabled bool `json:"totp_enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &env2); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env2.Data.TOTPEnabled {
		t.Error("expected totp_enabled=true after EnableTOTP, resolved via Bearer -> GetUserByID")
	}
}

// ---- Dashboard ----

func TestUserDashboardUnauthenticatedRedirects(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/login" {
		t.Errorf("expected redirect to /server/auth/login, got %q", loc)
	}
}

func TestDashboardAuthorizedEmptyURLs(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "bea", "bea@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/dashboard", nil), user)
	w := httptest.NewRecorder()
	h.Dashboard(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDashboardCreatedFlash(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "cid", "cid@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/dashboard?created=abc123", nil), user)
	w := httptest.NewRecorder()
	h.Dashboard(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDashboardDeletedFlash(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "dot", "dot@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/dashboard?deleted=abc123", nil), user)
	w := httptest.NewRecorder()
	h.Dashboard(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- Bulk ----

func TestBulkUnauthenticatedRedirects(t *testing.T) {
	h, _, _, _, _ := newUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/users/bulk", nil)
	w := httptest.NewRecorder()
	h.Bulk(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/login" {
		t.Errorf("expected redirect to /server/auth/login, got %q", loc)
	}
}

func TestBulkAuthorized(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "eli", "eli@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/bulk", nil), user)
	w := httptest.NewRecorder()
	h.Bulk(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBulkImportErrorFlash(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "fay", "fay@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/bulk?import_error=bad+file", nil), user)
	w := httptest.NewRecorder()
	h.Bulk(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBulkImportedSuccessWithErrorsFlash(t *testing.T) {
	h, authService, _, _, _ := newUserTestHandler(t)
	user := seedUser(t, authService, "gus", "gus@example.com")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/bulk?imported=1&success=5&errors=2", nil), user)
	w := httptest.NewRecorder()
	h.Bulk(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- shared assertion helper ----

// assertAPIErrorStatus verifies the canonical error envelope shape and the
// expected HTTP status.
func assertAPIErrorStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, w.Code, w.Body.String())
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.OK {
		t.Error("expected ok=false in the canonical error envelope")
	}
}
