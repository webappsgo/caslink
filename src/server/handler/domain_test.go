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
)

func newDomainTestHandler(t *testing.T) (*DomainHandler, *service.OrgService, *service.AuthService, *service.User) {
	t.Helper()

	st := newSchemaTestStore(t)
	domainService := service.NewDomainService(st, config.CustomDomainsConfig{})
	orgService := service.NewOrgService(st)
	authService := service.NewAuthService(st)

	user, err := authService.RegisterUser(context.Background(), "alice", "alice@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	auditService := service.NewAuditService(st)
	return NewDomainHandler(domainService, authService, orgService, auditService), orgService, authService, user
}

// TestListUserDomainsUnauthenticated verifies 401 when no user is attached.
func TestListUserDomainsUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/domains", nil)
	w := httptest.NewRecorder()
	h.ListUserDomains(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body.OK {
		t.Errorf("expected ok:false")
	}
	if body.Error != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED, got %q", body.Error)
	}
}

// TestListUserDomainsEmpty verifies a 200 with an empty domains list for a
// user with none configured.
func TestListUserDomainsEmpty(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/domains", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.ListUserDomains(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAddUserDomainUnauthenticated verifies 401 when no user is attached.
func TestAddUserDomainUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader(`{"domain":"example.com"}`))
	w := httptest.NewRecorder()
	h.AddUserDomain(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestAddUserDomainInvalidBody verifies 400 on malformed JSON, using the
// canonical error envelope.
func TestAddUserDomainInvalidBody(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader("{not json"))
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.AddUserDomain(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body.Error != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %q", body.Error)
	}
}

// TestAddUserDomainSuccess verifies the happy path creates a domain and
// returns 201.
func TestAddUserDomainSuccess(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader(`{"domain":"links.example.com"}`))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.AddUserDomain(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAddUserDomainDuplicateRejected verifies adding the same domain twice
// fails with 400 (service-level "domain already exists" error).
func TestAddUserDomainDuplicateRejected(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	body := `{"domain":"dup.example.com"}`
	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader(body))
	r1 = withUser(r1, user)
	w1 := httptest.NewRecorder()
	h.AddUserDomain(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected first add to succeed with 201, got %d: %s", w1.Code, w1.Body.String())
	}

	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader(body))
	r2 = withUser(r2, user)
	w2 := httptest.NewRecorder()
	h.AddUserDomain(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected duplicate add to fail with 400, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestVerifyUserDomainUnauthenticated verifies 401 when no user is attached.
func TestVerifyUserDomainUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains/example.com/verify", nil)
	r = withChiURLParam(r, "domain", "example.com")
	w := httptest.NewRecorder()
	h.VerifyUserDomain(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestVerifyUserDomainNotFound verifies 404 when the named domain does not
// belong to (or does not exist for) the authenticated user.
func TestVerifyUserDomainNotFound(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains/nope.example.com/verify", nil)
	r = withChiURLParam(r, "domain", "nope.example.com")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.VerifyUserDomain(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body.Error != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", body.Error)
	}
}

// TestVerifyUserDomainOwnedButUnverifiable verifies that a domain the user
// does own reaches the real DNS-TXT verification step (VerifyDomain) rather
// than being rejected earlier as not-found, and since no such DNS record
// exists in this hermetic test environment, verification fails with 400 —
// never silently bypassed/mocked.
func TestVerifyUserDomainOwnedButUnverifiable(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	addBody := `{"domain":"unverified.example.invalid"}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader(addBody))
	addReq = withUser(addReq, user)
	addW := httptest.NewRecorder()
	h.AddUserDomain(addW, addReq)
	if addW.Code != http.StatusCreated {
		t.Fatalf("expected add to succeed with 201, got %d: %s", addW.Code, addW.Body.String())
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains/unverified.example.invalid/verify", nil)
	r = withChiURLParam(r, "domain", "unverified.example.invalid")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.VerifyUserDomain(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (no DNS TXT record present), got %d: %s", w.Code, w.Body.String())
	}
}

// TestListOrgDomainsUnauthenticated verifies 401 when no user is attached.
func TestListOrgDomainsUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/acme/domains", nil)
	r = withChiURLParam(r, "slug", "acme")
	w := httptest.NewRecorder()
	h.ListOrgDomains(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestListOrgDomainsOrgNotFound verifies 404 for an unknown org slug.
func TestListOrgDomainsOrgNotFound(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/nonexistent/domains", nil)
	r = withChiURLParam(r, "slug", "nonexistent")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.ListOrgDomains(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestListOrgDomainsForbiddenForNonMember verifies 403 when the requesting
// user is not a member of the organization.
func TestListOrgDomainsForbiddenForNonMember(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	owner := &service.User{ID: 99, Username: "owner"}
	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/domains", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.ListOrgDomains(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestListOrgDomainsSuccessForMember verifies 200 for an actual org member
// (the owner, in this case).
func TestListOrgDomainsSuccessForMember(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/domains", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.ListOrgDomains(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAddOrgDomainUnauthenticated verifies 401 when no user is attached.
func TestAddOrgDomainUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/acme/domains", strings.NewReader(`{"domain":"acme.example.com"}`))
	r = withChiURLParam(r, "slug", "acme")
	w := httptest.NewRecorder()
	h.AddOrgDomain(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestAddOrgDomainOrgNotFound verifies 404 for an unknown org slug.
func TestAddOrgDomainOrgNotFound(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/nonexistent/domains", strings.NewReader(`{"domain":"x.example.com"}`))
	r = withChiURLParam(r, "slug", "nonexistent")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.AddOrgDomain(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAddOrgDomainForbiddenForMemberRole verifies a plain "member" (not
// owner/admin) cannot add a domain.
func TestAddOrgDomainForbiddenForMemberRole(t *testing.T) {
	h, orgService, authService, user := newDomainTestHandler(t)

	owner, err := authService.RegisterUser(context.Background(), "orgowner", "owner@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "alice@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains", strings.NewReader(`{"domain":"acme.example.com"}`))
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.AddOrgDomain(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAddOrgDomainSuccessForOwner verifies the org owner can add a domain,
// and the response uses the canonical {ok:true,data:{...}} envelope with the
// "domain" key living directly inside data (no double-wrapped inner "ok").
func TestAddOrgDomainSuccessForOwner(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains", strings.NewReader(`{"domain":"acme.example.com"}`))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.AddOrgDomain(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env.OK {
		t.Errorf("expected ok:true, got %v", env.OK)
	}
	if _, hasDomain := env.Data["domain"]; !hasDomain {
		t.Errorf("expected a domain field in the response, got %v", env.Data)
	}
	if _, doubleWrapped := env.Data["ok"]; doubleWrapped {
		t.Errorf("data must not carry a nested ok key (double-wrapped envelope): %v", env.Data)
	}
}

// TestVerifyOrgDomainUnauthenticated verifies 401 when no user is attached.
func TestVerifyOrgDomainUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/acme/domains/acme.example.com/verify", nil)
	r = withChiURLParam(r, "slug", "acme")
	r = withChiURLParam(r, "domain", "acme.example.com")
	w := httptest.NewRecorder()
	h.VerifyOrgDomain(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestVerifyOrgDomainForbiddenForMemberRole verifies a plain "member" cannot
// trigger verification.
func TestVerifyOrgDomainForbiddenForMemberRole(t *testing.T) {
	h, orgService, authService, user := newDomainTestHandler(t)

	owner, err := authService.RegisterUser(context.Background(), "orgowner", "owner@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "alice@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains/acme.example.com/verify", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "domain", "acme.example.com")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.VerifyOrgDomain(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestVerifyOrgDomainNotFound verifies 404 when the named domain is not one of
// the organization's domains, even for an authorized owner.
func TestVerifyOrgDomainNotFound(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains/nope.example.com/verify", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "domain", "nope.example.com")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.VerifyOrgDomain(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestVerifyOrgDomainOwnedButUnverifiable verifies that a domain the org does
// own reaches the real DNS-TXT verification step (VerifyDomain) rather than
// being rejected earlier as not-found; with no such DNS record present it
// fails with 400 — never silently bypassed.
func TestVerifyOrgDomainOwnedButUnverifiable(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains", strings.NewReader(`{"domain":"unverified.example.invalid"}`))
	addReq = withChiURLParam(addReq, "slug", org.Slug)
	addReq = withUser(addReq, user)
	addW := httptest.NewRecorder()
	h.AddOrgDomain(addW, addReq)
	if addW.Code != http.StatusCreated {
		t.Fatalf("expected add to succeed with 201, got %d: %s", addW.Code, addW.Body.String())
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains/unverified.example.invalid/verify", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "domain", "unverified.example.invalid")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.VerifyOrgDomain(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (no DNS TXT record present), got %d: %s", w.Code, w.Body.String())
	}
}

// TestAddOrgDomainInvalidBody verifies 400 on malformed JSON for an
// authorized (owner) caller.
func TestAddOrgDomainInvalidBody(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains", strings.NewReader("{not json"))
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.AddOrgDomain(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- API mirror handlers (/api/v1) -------------------------------------

// TestAPIListUserDomainsBearerUserToken verifies the bearer path resolves the
// token owner and returns 200 with the canonical envelope.
func TestAPIListUserDomainsBearerUserToken(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users/domains", nil), rec)
	w := httptest.NewRecorder()
	h.APIListUserDomains(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env.OK {
		t.Errorf("expected ok:true")
	}
	if _, ok := env.Data["domains"]; !ok {
		t.Errorf("expected a domains field, got %v", env.Data)
	}
}

// TestAPIListUserDomainsBearerNonUserTokenRejected verifies an admin- or
// org-scoped token cannot reach a user-owned domain list.
func TestAPIListUserDomainsBearerNonUserTokenRejected(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	rec := &service.TokenRecord{OwnerType: "admin", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users/domains", nil), rec)
	w := httptest.NewRecorder()
	h.APIListUserDomains(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIListUserDomainsUnauthenticated verifies 401 with neither session nor
// bearer token.
func TestAPIListUserDomainsUnauthenticated(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/domains", nil)
	w := httptest.NewRecorder()
	h.APIListUserDomains(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestAPIAddUserDomainBearerSuccess verifies the bearer path can create a
// domain and returns 201 with the DNS instructions.
func TestAPIAddUserDomainBearerSuccess(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", strings.NewReader(`{"domain":"api.example.com"}`)), rec)
	w := httptest.NewRecorder()
	h.APIAddUserDomain(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := env.Data["dns_instructions"]; !ok {
		t.Errorf("expected dns_instructions in response, got %v", env.Data)
	}
}

// TestAPIVerifyUserDomainNotFound verifies 404 when the named domain is not
// owned by the bearer's user.
func TestAPIVerifyUserDomainNotFound(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodPost, "/api/v1/users/domains/nope.example.com/verify", nil), rec)
	r = withChiURLParam(r, "domain", "nope.example.com")
	w := httptest.NewRecorder()
	h.APIVerifyUserDomain(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIListOrgDomainsBearerNonMemberForbidden verifies a user-scoped bearer
// token for a non-member gets 403.
func TestAPIListOrgDomainsBearerNonMemberForbidden(t *testing.T) {
	h, orgService, authService, user := newDomainTestHandler(t)

	owner, err := authService.RegisterUser(context.Background(), "orgowner2", "owner2@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/domains", nil), rec)
	r = withChiURLParam(r, "slug", org.Slug)
	w := httptest.NewRecorder()
	h.APIListOrgDomains(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIAddOrgDomainBearerMemberRoleForbidden verifies a plain member cannot
// add an org domain via the API.
func TestAPIAddOrgDomainBearerMemberRoleForbidden(t *testing.T) {
	h, orgService, authService, user := newDomainTestHandler(t)

	owner, err := authService.RegisterUser(context.Background(), "orgowner3", "owner3@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "alice@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains", strings.NewReader(`{"domain":"acme.example.com"}`)), rec)
	r = withChiURLParam(r, "slug", org.Slug)
	w := httptest.NewRecorder()
	h.APIAddOrgDomain(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIAddOrgDomainBearerOwnerSuccess verifies the org owner can add a domain
// via the API bearer path.
func TestAPIAddOrgDomainBearerOwnerSuccess(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/domains", strings.NewReader(`{"domain":"acme.example.com"}`)), rec)
	r = withChiURLParam(r, "slug", org.Slug)
	w := httptest.NewRecorder()
	h.APIAddOrgDomain(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// seedUserDomain adds a domain for the given user directly through the service
// and returns its stored name (normalized). Used by the admin-handler tests.
func seedUserDomain(t *testing.T, h *DomainHandler, userID int64, name string) string {
	t.Helper()
	d, err := h.domainService.AddDomain(context.Background(), "user", userID, name)
	if err != nil {
		t.Fatalf("seed AddDomain(%q) failed: %v", name, err)
	}
	return d.Domain
}

// adminBearerReq builds a request carrying an admin-scoped bearer record and a
// chi {domain} URL param, mirroring what RequireBearerAdmin admits at runtime.
func adminBearerReq(method, target, domain string) *http.Request {
	rec := &service.TokenRecord{OwnerType: "admin", OwnerID: 1}
	r := withBearer(httptest.NewRequest(method, target, nil), rec)
	if domain != "" {
		r = withChiURLParam(r, "domain", domain)
	}
	return r
}

// TestAPIAdminListDomains verifies the admin list returns every owner's domain
// with the canonical paginated envelope.
func TestAPIAdminListDomains(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	seedUserDomain(t, h, user.ID, "one.example.com")
	seedUserDomain(t, h, user.ID, "two.example.com")

	r := adminBearerReq(http.MethodGet, "/api/v1/server/admin/config/domains", "")
	w := httptest.NewRecorder()
	h.APIAdminListDomains(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !body.OK {
		t.Errorf("expected ok:true")
	}
	if body.Pagination == nil || body.Pagination.Total != 2 {
		t.Errorf("expected pagination total 2, got %+v", body.Pagination)
	}
}

// TestAPIAdminGetDomain verifies detail lookup by name, and 404 for unknown.
func TestAPIAdminGetDomain(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	name := seedUserDomain(t, h, user.ID, "get.example.com")

	r := adminBearerReq(http.MethodGet, "/api/v1/server/admin/config/domains/"+name, name)
	w := httptest.NewRecorder()
	h.APIAdminGetDomain(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	r2 := adminBearerReq(http.MethodGet, "/api/v1/server/admin/config/domains/nope.example.com", "nope.example.com")
	w2 := httptest.NewRecorder()
	h.APIAdminGetDomain(w2, r2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown domain, got %d", w2.Code)
	}
}

// TestAPIAdminSuspendUnsuspendDomain verifies suspend then unsuspend flips the
// stored status, and that both succeed for an admin.
func TestAPIAdminSuspendUnsuspendDomain(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	name := seedUserDomain(t, h, user.ID, "susp.example.com")

	rs := adminBearerReq(http.MethodPost, "/api/v1/server/admin/config/domains/"+name+"/suspend", name)
	ws := httptest.NewRecorder()
	h.APIAdminSuspendDomain(ws, rs)
	if ws.Code != http.StatusOK {
		t.Fatalf("suspend: expected 200, got %d: %s", ws.Code, ws.Body.String())
	}
	if d, err := h.domainService.GetDomainByName(context.Background(), name); err != nil || d.Status != "suspended" {
		t.Fatalf("expected status suspended, got %v (err %v)", statusOf(d), err)
	}

	ru := adminBearerReq(http.MethodPost, "/api/v1/server/admin/config/domains/"+name+"/unsuspend", name)
	wu := httptest.NewRecorder()
	h.APIAdminUnsuspendDomain(wu, ru)
	if wu.Code != http.StatusOK {
		t.Fatalf("unsuspend: expected 200, got %d: %s", wu.Code, wu.Body.String())
	}
	if d, err := h.domainService.GetDomainByName(context.Background(), name); err != nil || d.Status == "suspended" {
		t.Fatalf("expected non-suspended status after unsuspend, got %v (err %v)", statusOf(d), err)
	}
}

// TestAPIAdminDeleteDomain verifies force-delete removes the domain (404 after).
func TestAPIAdminDeleteDomain(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	name := seedUserDomain(t, h, user.ID, "del.example.com")

	r := adminBearerReq(http.MethodDelete, "/api/v1/server/admin/config/domains/"+name, name)
	w := httptest.NewRecorder()
	h.APIAdminDeleteDomain(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := h.domainService.GetDomainByName(context.Background(), name); err == nil {
		t.Fatalf("expected domain gone after delete")
	}
}

// TestAPIAdminSuspendDomainNotFound verifies 404 when suspending an unknown
// domain rather than a 400 or a panic.
func TestAPIAdminSuspendDomainNotFound(t *testing.T) {
	h, _, _, _ := newDomainTestHandler(t)
	r := adminBearerReq(http.MethodPost, "/api/v1/server/admin/config/domains/ghost.example.com/suspend", "ghost.example.com")
	w := httptest.NewRecorder()
	h.APIAdminSuspendDomain(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// statusOf is a nil-safe accessor for a domain's status in test failure text.
func statusOf(d *service.CustomDomain) string {
	if d == nil {
		return "<nil>"
	}
	return d.Status
}

// seedOrgDomain adds an org-owned domain directly through the service layer.
func seedOrgDomain(t *testing.T, h *DomainHandler, orgID int64, name string) string {
	t.Helper()
	d, err := h.domainService.AddDomain(context.Background(), "org", orgID, name)
	if err != nil {
		t.Fatalf("seed org AddDomain(%q) failed: %v", name, err)
	}
	return d.Domain
}

// ---- user domain detail / dns / delete (CRUD parity) -------------------

// TestAPIGetUserDomainSuccess verifies the owner can read a single domain and
// gets both the domain object and its dns_instructions.
func TestAPIGetUserDomainSuccess(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	name := seedUserDomain(t, h, user.ID, "detail.example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users/domains/"+name, nil), rec)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIGetUserDomain(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := env.Data["domain"]; !ok {
		t.Errorf("expected a domain field, got %v", env.Data)
	}
	if _, ok := env.Data["dns_instructions"]; !ok {
		t.Errorf("expected dns_instructions, got %v", env.Data)
	}
}

// TestAPIGetUserDomainNotFoundForOtherUser verifies one user cannot read
// another user's domain — it 404s rather than leaking the record.
func TestAPIGetUserDomainNotFoundForOtherUser(t *testing.T) {
	h, _, authService, owner := newDomainTestHandler(t)
	name := seedUserDomain(t, h, owner.ID, "private.example.com")

	other, err := authService.RegisterUser(context.Background(), "bob", "bob@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: other.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users/domains/"+name, nil), rec)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIGetUserDomain(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user read, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIUserDomainDNSSuccess verifies the DNS-instructions-only endpoint.
func TestAPIUserDomainDNSSuccess(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	name := seedUserDomain(t, h, user.ID, "dns.example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/users/domains/"+name+"/dns", nil), rec)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIUserDomainDNS(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := env.Data["dns_instructions"]; !ok {
		t.Errorf("expected dns_instructions, got %v", env.Data)
	}
}

// TestAPIDeleteUserDomainSuccess verifies the owner deletes their domain and
// the row is actually removed.
func TestAPIDeleteUserDomainSuccess(t *testing.T) {
	h, _, _, user := newDomainTestHandler(t)
	name := seedUserDomain(t, h, user.ID, "gone.example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodDelete, "/api/v1/users/domains/"+name, nil), rec)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIDeleteUserDomain(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := h.domainService.GetDomainByName(context.Background(), name); err == nil {
		t.Fatalf("expected domain gone after delete")
	}
}

// TestAPIDeleteUserDomainCrossOwnerRejected verifies one user cannot delete
// another user's domain — 404, and the row survives.
func TestAPIDeleteUserDomainCrossOwnerRejected(t *testing.T) {
	h, _, authService, owner := newDomainTestHandler(t)
	name := seedUserDomain(t, h, owner.ID, "keep.example.com")

	other, err := authService.RegisterUser(context.Background(), "mallory", "mallory@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: other.ID}
	r := withBearer(httptest.NewRequest(http.MethodDelete, "/api/v1/users/domains/"+name, nil), rec)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIDeleteUserDomain(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user delete, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := h.domainService.GetDomainByName(context.Background(), name); err != nil {
		t.Fatalf("cross-user delete must not remove the row: %v", err)
	}
}

// ---- org domain detail / delete (CRUD parity) --------------------------

// TestAPIGetOrgDomainSuccessForMember verifies a member can read an org domain.
func TestAPIGetOrgDomainSuccessForMember(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)
	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	name := seedOrgDomain(t, h, org.ID, "org.example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/domains/"+name, nil), rec)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIGetOrgDomain(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIDeleteOrgDomainSuccessForOwner verifies the org owner deletes a domain
// and the row is removed.
func TestAPIDeleteOrgDomainSuccessForOwner(t *testing.T) {
	h, orgService, _, user := newDomainTestHandler(t)
	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	name := seedOrgDomain(t, h, org.ID, "orgdel.example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/domains/"+name, nil), rec)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIDeleteOrgDomain(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := h.domainService.GetDomainByName(context.Background(), name); err == nil {
		t.Fatalf("expected org domain gone after delete")
	}
}

// TestAPIDeleteOrgDomainForbiddenForMember verifies a plain member (not
// owner/admin) cannot delete an org domain, and the row survives.
func TestAPIDeleteOrgDomainForbiddenForMember(t *testing.T) {
	h, orgService, authService, user := newDomainTestHandler(t)
	owner, err := authService.RegisterUser(context.Background(), "orgowner3", "owner3@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Acme Corp", "acme")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "alice@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}
	name := seedOrgDomain(t, h, org.ID, "orgkeep.example.com")

	rec := &service.TokenRecord{OwnerType: "user", OwnerID: user.ID}
	r := withBearer(httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/domains/"+name, nil), rec)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "domain", name)
	w := httptest.NewRecorder()
	h.APIDeleteOrgDomain(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member delete, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := h.domainService.GetDomainByName(context.Background(), name); err != nil {
		t.Fatalf("forbidden delete must not remove the row: %v", err)
	}
}
