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
	return NewDomainHandler(domainService, nil, orgService), orgService, authService, user
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
// and the response uses the canonical ok:true envelope with a "domain" key.
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
