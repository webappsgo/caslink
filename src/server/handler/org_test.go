package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// newOrgTestHandler builds an OrgHandler backed by a real schema store, a
// real OrgService/AuthService, and a real template renderer, mirroring
// newAuthUserTestHandler in auth_user_test.go.
func newOrgTestHandler(t *testing.T) (*OrgHandler, *service.OrgService, *service.AuthService, *store.Store) {
	t.Helper()

	st := newSchemaTestStore(t)
	orgService := service.NewOrgService(st)
	authService := service.NewAuthService(st)
	inviteService := service.NewInviteService(st)
	cfg := config.DefaultConfig()
	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	return NewOrgHandler(orgService, authService, inviteService, renderer, cfg), orgService, authService, st
}

// registerTestUser registers a real user via AuthService and returns it,
// failing the test on error.
func registerTestUser(t *testing.T, authService *service.AuthService, username, email string) *service.User {
	t.Helper()
	u, err := authService.RegisterUser(context.Background(), username, email, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("seed RegisterUser(%q) failed: %v", username, err)
	}
	return u
}

// decodeErrorEnvelope decodes the canonical {"ok":false,"error":"CODE",
// "message":"..."} error shape used throughout org.go's JSON handlers.
func decodeErrorEnvelope(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var env struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode failed: %v (%s)", err, body)
	}
	if env.OK {
		t.Fatalf("expected ok:false, got ok:true (%s)", body)
	}
	return map[string]string{"error": env.Error, "message": env.Message}
}

// ---------------------------------------------------------------------
// ListOrgs (HTML)
// ---------------------------------------------------------------------

func TestListOrgsUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/orgs", nil)
	w := httptest.NewRecorder()
	h.ListOrgs(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestListOrgsSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "alice", "alice@example.com")

	if _, err := orgService.CreateOrganization(context.Background(), user.ID, "Acme Inc", "acme-inc"); err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/orgs", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.ListOrgs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Acme")) {
		t.Errorf("expected rendered page to mention the org name, got: %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------
// CreateOrgPage / CreateOrg (HTML form)
// ---------------------------------------------------------------------

func TestCreateOrgPageUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/orgs/new", nil)
	w := httptest.NewRecorder()
	h.CreateOrgPage(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCreateOrgPageAuthorized(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "bob", "bob@example.com")

	r := httptest.NewRequest(http.MethodGet, "/orgs/new", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.CreateOrgPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateOrgFormNameTooShort(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "carl", "carl@example.com")

	form := url.Values{"name": {"ab"}, "slug": {"ab-org"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/new", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.CreateOrg(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered form), got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("between 3 and 40 characters")) {
		t.Errorf("expected validation error message in body, got: %s", w.Body.String())
	}
}

func TestCreateOrgFormNameTooLong(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "dana", "dana@example.com")

	longName := strings.Repeat("x", 41)
	form := url.Values{"name": {longName}, "slug": {"dana-org"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/new", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.CreateOrg(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered form), got %d", w.Code)
	}
}

func TestCreateOrgFormInvalidSlug(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "erin2", "erin2@example.com")

	form := url.Values{"name": {"Valid Name"}, "slug": {"Not_Valid!"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/new", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.CreateOrg(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered form), got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("slug must be")) {
		t.Errorf("expected slug validation error in body, got: %s", w.Body.String())
	}
}

func TestCreateOrgFormSuccess(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "frank2", "frank2@example.com")

	form := url.Values{"name": {"Frank Org"}, "slug": {"frank-org"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/new", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.CreateOrg(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/orgs/frank-org/" {
		t.Errorf("expected redirect to /orgs/frank-org/, got %q", loc)
	}
}

func TestCreateOrgFormDuplicateSlug(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "greta", "greta@example.com")

	if _, err := orgService.CreateOrganization(context.Background(), user.ID, "Greta Org", "greta-org"); err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	form := url.Values{"name": {"Greta Org Two"}, "slug": {"greta-org"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/new", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.CreateOrg(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered form with error), got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("already exists")) {
		t.Errorf("expected duplicate-slug error in body, got: %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------
// OrgDashboard / OrgSettings / OrgMembers (HTML)
// ---------------------------------------------------------------------

func TestOrgDashboardNotFound(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "helen", "helen@example.com")

	r := httptest.NewRequest(http.MethodGet, "/orgs/nope/", nil)
	r = withChiURLParam(r, "slug", "nope")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.OrgDashboard(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOrgDashboardUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/orgs/x/", nil)
	r = withChiURLParam(r, "slug", "x")
	w := httptest.NewRecorder()
	h.OrgDashboard(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestOrgDashboardSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "ian", "ian@example.com")

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Ian Org", "ian-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/orgs/"+org.Slug+"/", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.OrgDashboard(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOrgSettingsNotFound(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "jane", "jane@example.com")

	r := httptest.NewRequest(http.MethodGet, "/orgs/nope/settings", nil)
	r = withChiURLParam(r, "slug", "nope")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.OrgSettings(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOrgSettingsSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "kyle", "kyle@example.com")

	org, err := orgService.CreateOrganization(context.Background(), user.ID, "Kyle Org", "kyle-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/orgs/"+org.Slug+"/settings", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.OrgSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOrgMembersSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "leo", "leo@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Leo Org", "leo-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/orgs/"+org.Slug+"/members", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembers(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("leo")) {
		t.Errorf("expected owner username in rendered members page, got: %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------
// OrgMembersAction (POST form)
// ---------------------------------------------------------------------

func TestOrgMembersActionUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	form := url.Values{"action": {"remove"}, "user_id": {"1"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/x/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", "x")
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestOrgMembersActionOrgNotFound(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "mia", "mia@example.com")

	form := url.Values{"action": {"remove"}, "user_id": {"1"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/nope/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", "nope")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOrgMembersActionForbiddenNonMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "nora", "nora@example.com")
	stranger := registerTestUser(t, authService, "oscar", "oscar@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Nora Org", "nora-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	form := url.Values{"action": {"remove"}, "user_id": {strconv.FormatInt(owner.ID, 10)}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, stranger)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d", w.Code)
	}
}

func TestOrgMembersActionRemoveSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "pete", "pete@example.com")
	member := registerTestUser(t, authService, "quinn", "quinn@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Pete Org", "pete-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "quinn@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	form := url.Values{"action": {"remove"}, "user_id": {strconv.FormatInt(member.ID, 10)}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered members page), got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Member removed")) {
		t.Errorf("expected success flash in body, got: %s", w.Body.String())
	}

	isMember, _, err := orgService.IsMember(context.Background(), org.ID, member.ID)
	if err != nil {
		t.Fatalf("IsMember failed: %v", err)
	}
	if isMember {
		t.Error("expected member to be removed from the organization")
	}
}

func TestOrgMembersActionRemoveOwnerFails(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "rachel", "rachel@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Rachel Org", "rachel-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	form := url.Values{"action": {"remove"}, "user_id": {strconv.FormatInt(owner.ID, 10)}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered members page with error flash), got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("transfer ownership first")) {
		t.Errorf("expected owner-removal error flash in body, got: %s", w.Body.String())
	}

	isMember, role, err := orgService.IsMember(context.Background(), org.ID, owner.ID)
	if err != nil || !isMember || role != "owner" {
		t.Errorf("expected owner to remain a member with role owner, got isMember=%v role=%q err=%v", isMember, role, err)
	}
}

func TestOrgMembersActionChangeRoleSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "sam", "sam@example.com")
	member := registerTestUser(t, authService, "tina", "tina@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Sam Org", "sam-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "tina@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	form := url.Values{"action": {"change_role"}, "user_id": {strconv.FormatInt(member.ID, 10)}, "role": {"admin"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Role updated")) {
		t.Errorf("expected success flash in body, got: %s", w.Body.String())
	}

	_, role, err := orgService.IsMember(context.Background(), org.ID, member.ID)
	if err != nil || role != "admin" {
		t.Errorf("expected member role to be admin, got %q err=%v", role, err)
	}
}

func TestOrgMembersActionChangeRoleInvalid(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "uma", "uma@example.com")
	member := registerTestUser(t, authService, "vic", "vic@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Uma Org", "uma-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "vic@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	form := url.Values{"action": {"change_role"}, "user_id": {strconv.FormatInt(member.ID, 10)}, "role": {"owner"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("must be")) {
		t.Errorf("expected invalid-role error flash in body, got: %s", w.Body.String())
	}
}

func TestOrgMembersActionInviteSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "walt", "walt@example.com")
	registerTestUser(t, authService, "xena", "xena@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Walt Org", "walt-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	form := url.Values{"action": {"invite"}, "email": {"xena@example.com"}, "role": {"member"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Member added")) {
		t.Errorf("expected success flash in body, got: %s", w.Body.String())
	}
}

func TestOrgMembersActionInviteNoAccount(t *testing.T) {
	h, orgService, authService, st := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "yuki", "yuki@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Yuki Org", "yuki-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	form := url.Values{"action": {"invite"}, "email": {"nobody@example.com"}, "role": {"member"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// No account exists, so a shareable single-use invite link is issued.
	if !bytes.Contains(w.Body.Bytes(), []byte("/orgs/invite/accept?token=")) {
		t.Errorf("expected shareable invite link in body, got: %s", w.Body.String())
	}

	inviteSvc := service.NewInviteService(st)
	invites, err := inviteSvc.ListInvitesByKind(context.Background(), service.InviteKindOrgMembership, org.ID)
	if err != nil {
		t.Fatalf("ListInvitesByKind failed: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 pending invite, got %d", len(invites))
	}
	if invites[0].Email != "nobody@example.com" || invites[0].Role != "member" {
		t.Errorf("unexpected invite: email=%q role=%q", invites[0].Email, invites[0].Role)
	}
}

// TestOrgAcceptInviteJoinsOrg exercises the end-to-end org-membership invite
// flow: a non-existent invitee later registers, then consumes the invite link
// and joins the org with the invite's role.
func TestOrgAcceptInviteJoinsOrg(t *testing.T) {
	h, orgService, authService, st := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "amy", "amy@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Amy Org", "amy-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	inviteSvc := service.NewInviteService(st)
	plaintext, _, err := inviteSvc.CreateInvite(context.Background(), service.CreateInviteParams{
		Kind:      service.InviteKindOrgMembership,
		Email:     "bob@example.com",
		OrgID:     org.ID,
		Role:      "admin",
		CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateInvite failed: %v", err)
	}

	// The invitee registers an account, then accepts.
	invitee := registerTestUser(t, authService, "bob", "bob@example.com")

	r := httptest.NewRequest(http.MethodGet, "/orgs/invite/accept?token="+plaintext, nil)
	r = withUser(r, invitee)
	w := httptest.NewRecorder()
	h.OrgAcceptInvite(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/orgs/"+org.Slug+"/" {
		t.Errorf("expected redirect to org dashboard, got %q", loc)
	}

	isMember, role, err := orgService.IsMember(context.Background(), org.ID, invitee.ID)
	if err != nil || !isMember || role != "admin" {
		t.Errorf("expected invitee to be an admin member, got isMember=%v role=%q err=%v", isMember, role, err)
	}

	// The single-use invite is now consumed and cannot be reused.
	if _, err := inviteSvc.ValidateInvite(context.Background(), plaintext, service.InviteKindOrgMembership); err == nil {
		t.Error("expected consumed invite to be unusable")
	}
}

// TestOrgAcceptInviteBadToken rejects a missing/invalid invite token.
func TestOrgAcceptInviteBadToken(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "cara", "cara@example.com")

	r := httptest.NewRequest(http.MethodGet, "/orgs/invite/accept?token=nope", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.OrgAcceptInvite(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid token, got %d", w.Code)
	}
}

// TestAPICreateOrgInviteSuccess issues an org-membership invite via the API.
func TestAPICreateOrgInviteSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "dan", "dan@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Dan Org", "dan-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	body := `{"email":"newbie@example.com","role":"admin"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/invites", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APICreateOrgInvite(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Email     string `json:"email"`
			Role      string `json:"role"`
			AcceptURL string `json:"accept_url"`
			Token     string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Data.Email != "newbie@example.com" || resp.Data.Role != "admin" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.Data.Token == "" || !strings.Contains(resp.Data.AcceptURL, resp.Data.Token) {
		t.Errorf("expected accept_url to embed token, got %q", resp.Data.AcceptURL)
	}
}

// TestAPICreateOrgInviteForbiddenMember blocks a plain member from inviting.
func TestAPICreateOrgInviteForbiddenMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "erin", "erin@example.com")
	member := registerTestUser(t, authService, "finn", "finn@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Erin Org", "erin-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "finn@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	body := `{"email":"x@example.com","role":"member"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/invites", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, member)
	w := httptest.NewRecorder()
	h.APICreateOrgInvite(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for plain member, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPIListAndRevokeOrgInvite lists then revokes a pending invite via the API.
func TestAPIListAndRevokeOrgInvite(t *testing.T) {
	h, orgService, authService, st := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "gwen", "gwen@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Gwen Org", "gwen-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	inviteSvc := service.NewInviteService(st)
	_, inv, err := inviteSvc.CreateInvite(context.Background(), service.CreateInviteParams{
		Kind:  service.InviteKindOrgMembership,
		Email: "pending@example.com",
		OrgID: org.ID,
		Role:  "member",
	})
	if err != nil {
		t.Fatalf("CreateInvite failed: %v", err)
	}

	// List.
	lr := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/invites", nil)
	lr = withChiURLParam(lr, "slug", org.Slug)
	lr = withUser(lr, owner)
	lw := httptest.NewRecorder()
	h.APIListOrgInvites(lw, lr)
	if lw.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", lw.Code, lw.Body.String())
	}
	if !bytes.Contains(lw.Body.Bytes(), []byte("pending@example.com")) {
		t.Errorf("expected pending invite in list, got: %s", lw.Body.String())
	}

	// Revoke.
	dr := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/invites/"+strconv.FormatInt(inv.ID, 10), nil)
	dr = withChiURLParam(dr, "slug", org.Slug)
	dr = withChiURLParam(dr, "inviteID", strconv.FormatInt(inv.ID, 10))
	dr = withUser(dr, owner)
	dw := httptest.NewRecorder()
	h.APIRevokeOrgInvite(dw, dr)
	if dw.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", dw.Code, dw.Body.String())
	}

	invites, err := inviteSvc.ListInvitesByKind(context.Background(), service.InviteKindOrgMembership, org.ID)
	if err != nil {
		t.Fatalf("ListInvitesByKind failed: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("expected no active invites after revoke, got %d", len(invites))
	}
}

func TestOrgMembersActionUnknownAction(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "zack", "zack@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Zack Org", "zack-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	form := url.Values{"action": {"self-destruct"}}
	r := httptest.NewRequest(http.MethodPost, "/orgs/"+org.Slug+"/members", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.OrgMembersAction(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Unknown action")) {
		t.Errorf("expected unknown-action error flash in body, got: %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------
// APIListOrgs / APICreateOrg (JSON)
// ---------------------------------------------------------------------

func TestAPIListOrgsUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs", nil)
	w := httptest.NewRecorder()
	h.APIListOrgs(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIListOrgsSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "amy", "amy@example.com")
	if _, err := orgService.CreateOrganization(context.Background(), user.ID, "Amy Org", "amy-org"); err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APIListOrgs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Data struct {
			Organizations []map[string]interface{} `json:"organizations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(env.Data.Organizations) != 1 {
		t.Fatalf("expected 1 organization, got %d", len(env.Data.Organizations))
	}
}

func TestAPICreateOrgUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	body := `{"name":"Some Org","slug":"some-org"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPICreateOrgInvalidBody(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "ben", "ben@example.com")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader("not-json"))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateOrgNameTooShort(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "cara", "cara@example.com")

	body := `{"name":"ab","slug":"cara-org"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateOrgInvalidSlug(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "dean", "dean@example.com")

	body := `{"name":"Dean Org","slug":"Not Valid!"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	env := decodeErrorEnvelope(t, w.Body.Bytes())
	if env["error"] != "BAD_REQUEST" {
		t.Errorf("unexpected error code: %q", env["error"])
	}
	if env["message"] == "" {
		t.Errorf("expected a slug validation message, got empty")
	}
}

func TestAPICreateOrgSuccess(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "ellie", "ellie@example.com")

	body := `{"name":"Ellie Org","slug":"ellie-org"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Data struct {
			Success bool `json:"success"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env.Data.Success {
		t.Error("expected success=true")
	}
}

func TestAPICreateOrgAutoSlug(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "farah", "farah@example.com")

	body := `{"name":"Farah Widgets"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPICreateOrgDuplicateSlug(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "gabe", "gabe@example.com")
	if _, err := orgService.CreateOrganization(context.Background(), user.ID, "Gabe Org", "gabe-org"); err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	body := `{"name":"Gabe Org Two","slug":"gabe-org"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APICreateOrg(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------
// APIGetOrg
// ---------------------------------------------------------------------

func TestAPIGetOrgUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/x", nil)
	r = withChiURLParam(r, "slug", "x")
	w := httptest.NewRecorder()
	h.APIGetOrg(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPIGetOrgNotFound(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "hugo", "hugo@example.com")

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/nope", nil)
	r = withChiURLParam(r, "slug", "nope")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APIGetOrg(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPIGetOrgForbiddenNonMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "iris", "iris@example.com")
	stranger := registerTestUser(t, authService, "jack2", "jack2@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Iris Org", "iris-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug, nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, stranger)
	w := httptest.NewRecorder()
	h.APIGetOrg(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAPIGetOrgSuccessMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "kim", "kim@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Kim Org", "kim-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug, nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APIGetOrg(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------
// APIGetMembers
// ---------------------------------------------------------------------

func TestAPIGetMembersForbiddenNonMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "liam", "liam@example.com")
	stranger := registerTestUser(t, authService, "mona", "mona@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Liam Org", "liam-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/members", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, stranger)
	w := httptest.NewRecorder()
	h.APIGetMembers(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAPIGetMembersSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "noah", "noah@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Noah Org", "noah-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/members", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APIGetMembers(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIGetMembersNotFound(t *testing.T) {
	h, _, authService, _ := newOrgTestHandler(t)
	user := registerTestUser(t, authService, "opal", "opal@example.com")

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/nope/members", nil)
	r = withChiURLParam(r, "slug", "nope")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.APIGetMembers(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------
// APICreateOrgToken / APIListOrgTokens / APIRevokeOrgToken
// ---------------------------------------------------------------------

func TestAPICreateOrgTokenForbiddenMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "paul", "paul@example.com")
	member := registerTestUser(t, authService, "quincy", "quincy@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Paul Org", "paul-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "quincy@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	body := `{"name":"ci-token"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/tokens", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, member)
	w := httptest.NewRecorder()
	h.APICreateOrgToken(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for plain member, got %d", w.Code)
	}
}

func TestAPICreateOrgTokenMissingName(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "rick", "rick@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Rick Org", "rick-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	body := `{"name":""}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/tokens", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APICreateOrgToken(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPICreateOrgTokenSuccessOwner(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "sara2", "sara2@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Sara Org", "sara-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	body := `{"name":"ci-token","permissions":["links:read"]}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/tokens", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APICreateOrgToken(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Canonical single-wrap shape: {"ok":true,"data":{"token":...,"org_token":...}}.
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env.OK || env.Data.Token == "" {
		t.Errorf("expected ok=true and a non-empty plaintext token, got %+v", env)
	}
}

func TestAPIListOrgTokensForbiddenNonMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "toby", "toby@example.com")
	stranger := registerTestUser(t, authService, "uzi", "uzi@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Toby Org", "toby-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/tokens", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, stranger)
	w := httptest.NewRecorder()
	h.APIListOrgTokens(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAPIListOrgTokensSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "vera", "vera@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Vera Org", "vera-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if _, _, err := orgService.CreateOrgToken(context.Background(), org.ID, owner.ID, "seed-token", nil); err != nil {
		t.Fatalf("seed CreateOrgToken failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug+"/tokens", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APIListOrgTokens(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Canonical single-wrap shape: {"ok":true,"data":[...tokens]}.
	var env struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(env.Data) != 1 {
		t.Fatalf("expected 1 token, got %d", len(env.Data))
	}
}

func TestAPIRevokeOrgTokenInvalidID(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "walt2", "walt2@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Walt2 Org", "walt2-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/tokens/nope", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "tokenID", "nope")
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APIRevokeOrgToken(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPIRevokeOrgTokenForbiddenMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "xander", "xander@example.com")
	member := registerTestUser(t, authService, "yara", "yara@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Xander Org", "xander-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "yara@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}
	tok, _, err := orgService.CreateOrgToken(context.Background(), org.ID, owner.ID, "seed-token", nil)
	if err != nil {
		t.Fatalf("seed CreateOrgToken failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/tokens/"+strconv.FormatInt(tok.ID, 10), nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "tokenID", strconv.FormatInt(tok.ID, 10))
	r = withUser(r, member)
	w := httptest.NewRecorder()
	h.APIRevokeOrgToken(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAPIRevokeOrgTokenNotFound(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "zeke", "zeke@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Zeke Org", "zeke-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/tokens/999999", nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "tokenID", "999999")
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APIRevokeOrgToken(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPIRevokeOrgTokenSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "abby", "abby@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Abby Org", "abby-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	tok, _, err := orgService.CreateOrgToken(context.Background(), org.ID, owner.ID, "seed-token", nil)
	if err != nil {
		t.Fatalf("seed CreateOrgToken failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/tokens/"+strconv.FormatInt(tok.ID, 10), nil)
	r = withChiURLParam(r, "slug", org.Slug)
	r = withChiURLParam(r, "tokenID", strconv.FormatInt(tok.ID, 10))
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APIRevokeOrgToken(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tokens, err := orgService.ListOrgTokens(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("ListOrgTokens failed: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected the token to be revoked, still have %d", len(tokens))
	}
}

// ---------------------------------------------------------------------
// APITransferOrgOwnership
// ---------------------------------------------------------------------

func TestAPITransferOrgOwnershipUnauthorized(t *testing.T) {
	h, _, _, _ := newOrgTestHandler(t)

	body := `{"new_owner_id":1}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/x/transfer", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", "x")
	w := httptest.NewRecorder()
	h.APITransferOrgOwnership(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAPITransferOrgOwnershipMissingNewOwner(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "cody", "cody@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Cody Org", "cody-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	body := `{"new_owner_id":0}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/transfer", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APITransferOrgOwnership(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPITransferOrgOwnershipNotOwner(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "drew", "drew@example.com")
	member := registerTestUser(t, authService, "ezra", "ezra@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Drew Org", "drew-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "ezra@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	body := `{"new_owner_id":` + strconv.FormatInt(member.ID, 10) + `}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/transfer", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, member)
	w := httptest.NewRecorder()
	h.APITransferOrgOwnership(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (caller is not the owner), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPITransferOrgOwnershipNewOwnerNotMember(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "finn", "finn@example.com")
	outsider := registerTestUser(t, authService, "gwen", "gwen@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Finn Org", "finn-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}

	body := `{"new_owner_id":` + strconv.FormatInt(outsider.ID, 10) + `}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/transfer", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APITransferOrgOwnership(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPITransferOrgOwnershipSuccess(t *testing.T) {
	h, orgService, authService, _ := newOrgTestHandler(t)
	owner := registerTestUser(t, authService, "holly", "holly@example.com")
	newOwner := registerTestUser(t, authService, "ivo", "ivo@example.com")

	org, err := orgService.CreateOrganization(context.Background(), owner.ID, "Holly Org", "holly-org")
	if err != nil {
		t.Fatalf("seed CreateOrganization failed: %v", err)
	}
	if err := orgService.AddMemberByEmail(context.Background(), org.ID, "ivo@example.com", "member"); err != nil {
		t.Fatalf("seed AddMemberByEmail failed: %v", err)
	}

	body := `{"new_owner_id":` + strconv.FormatInt(newOwner.ID, 10) + `}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/"+org.Slug+"/transfer", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "slug", org.Slug)
	r = withUser(r, owner)
	w := httptest.NewRecorder()
	h.APITransferOrgOwnership(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	_, oldRole, err := orgService.IsMember(context.Background(), org.ID, owner.ID)
	if err != nil || oldRole != "admin" {
		t.Errorf("expected old owner demoted to admin, got role=%q err=%v", oldRole, err)
	}
	_, newRole, err := orgService.IsMember(context.Background(), org.ID, newOwner.ID)
	if err != nil || newRole != "owner" {
		t.Errorf("expected new owner promoted to owner, got role=%q err=%v", newRole, err)
	}
}
