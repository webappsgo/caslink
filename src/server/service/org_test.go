package service

import (
	"context"
	"testing"

	"github.com/webappsgo/caslink/src/server/store"
)

// insertOrgTestUser inserts a minimal user row and returns its ID. Shared by
// every org_test.go case that needs an owner/member/lookup target.
func insertOrgTestUser(t *testing.T, st *store.Store, username, email string) int64 {
	t.Helper()

	res, err := st.UsersDB.Exec(
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		username, email, "hash-placeholder",
	)
	if err != nil {
		t.Fatalf("failed to insert test user %q: %v", username, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to get inserted user ID: %v", err)
	}
	return id
}

// Covers the happy path plus slug auto-generation and uniqueness — org
// creation is the entry point every other org operation depends on.
func TestCreateOrganization(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	ownerID := insertOrgTestUser(t, st, "owner1", "owner1@example.com")

	tests := []struct {
		name      string
		orgName   string
		slug      string
		wantSlug  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "explicit valid slug",
			orgName:  "Acme Corp",
			slug:     "acme-corp",
			wantSlug: "acme-corp",
		},
		{
			name:     "slug auto-generated from name",
			orgName:  "Widget Makers Inc",
			slug:     "",
			wantSlug: "widget-makers-inc",
		},
		{
			name:      "slug too short is invalid",
			orgName:   "Ab",
			slug:      "ab",
			wantErr:   true,
			errSubstr: "invalid organization slug",
		},
		{
			name:      "slug with invalid characters",
			orgName:   "Bad Slug",
			slug:      "Bad_Slug!",
			wantErr:   true,
			errSubstr: "invalid organization slug",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			org, err := svc.CreateOrganization(ctx, ownerID, tc.orgName, tc.slug)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CreateOrganization(%q, %q) expected error, got nil", tc.orgName, tc.slug)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateOrganization(%q, %q) unexpected error: %v", tc.orgName, tc.slug, err)
			}
			if org.Slug != tc.wantSlug {
				t.Errorf("Slug = %q, want %q", org.Slug, tc.wantSlug)
			}
			if org.OwnerID != ownerID {
				t.Errorf("OwnerID = %d, want %d", org.OwnerID, ownerID)
			}
			if org.ID == 0 {
				t.Error("expected non-zero org ID")
			}

			// Owner must be recorded as an 'owner'-role member.
			isMember, role, err := svc.IsMember(ctx, org.ID, ownerID)
			if err != nil {
				t.Fatalf("IsMember failed: %v", err)
			}
			if !isMember {
				t.Fatal("expected owner to be a member of the created org")
			}
			if role != "owner" {
				t.Errorf("owner role = %q, want %q", role, "owner")
			}
		})
	}
}

// Duplicate slugs must be rejected, whether provided explicitly or derived
// from a colliding name, and must not leave a partial org/membership behind.
func TestCreateOrganizationDuplicateSlug(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	ownerID := insertOrgTestUser(t, st, "owner2", "owner2@example.com")
	otherID := insertOrgTestUser(t, st, "owner3", "owner3@example.com")

	if _, err := svc.CreateOrganization(ctx, ownerID, "First Org", "first-org"); err != nil {
		t.Fatalf("initial CreateOrganization failed: %v", err)
	}

	_, err := svc.CreateOrganization(ctx, otherID, "First Org Again", "first-org")
	if err == nil {
		t.Fatal("expected error creating org with duplicate slug")
	}

	// The rejected attempt must not have added the second user as a member
	// of the existing org.
	orgs, err := svc.GetUserOrganizations(ctx, otherID)
	if err != nil {
		t.Fatalf("GetUserOrganizations failed: %v", err)
	}
	if len(orgs) != 0 {
		t.Errorf("expected no orgs for rejected creator, got %d", len(orgs))
	}
}

// Listing orgs for a user must reflect membership across multiple orgs and
// exclude orgs the user does not belong to.
func TestGetUserOrganizations(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	userA := insertOrgTestUser(t, st, "usera", "usera@example.com")
	userB := insertOrgTestUser(t, st, "userb", "userb@example.com")

	orgOne, err := svc.CreateOrganization(ctx, userA, "Org One", "org-one")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if _, err := svc.CreateOrganization(ctx, userA, "Org Two", "org-two"); err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	// userB is unrelated and should see zero orgs, then one after joining.
	orgsB, err := svc.GetUserOrganizations(ctx, userB)
	if err != nil {
		t.Fatalf("GetUserOrganizations failed: %v", err)
	}
	if len(orgsB) != 0 {
		t.Fatalf("expected 0 orgs for userB, got %d", len(orgsB))
	}

	if err := svc.AddMemberByEmail(ctx, orgOne.ID, "userb@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	orgsA, err := svc.GetUserOrganizations(ctx, userA)
	if err != nil {
		t.Fatalf("GetUserOrganizations failed: %v", err)
	}
	if len(orgsA) != 2 {
		t.Fatalf("expected 2 orgs for userA, got %d", len(orgsA))
	}

	orgsB, err = svc.GetUserOrganizations(ctx, userB)
	if err != nil {
		t.Fatalf("GetUserOrganizations failed: %v", err)
	}
	if len(orgsB) != 1 {
		t.Fatalf("expected 1 org for userB after joining, got %d", len(orgsB))
	}
	if orgsB[0].ID != orgOne.ID {
		t.Errorf("userB org ID = %d, want %d", orgsB[0].ID, orgOne.ID)
	}
}

// GetUserOrganizationsWithSummary must report the caller's own role and an
// accurate member count without N+1 per-org queries.
func TestGetUserOrganizationsWithSummary(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "sumowner", "sumowner@example.com")
	member := insertOrgTestUser(t, st, "summember", "summember@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "Summary Org", "summary-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := svc.AddMemberByEmail(ctx, org.ID, "summember@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	ownerSummaries, err := svc.GetUserOrganizationsWithSummary(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserOrganizationsWithSummary failed: %v", err)
	}
	if len(ownerSummaries) != 1 {
		t.Fatalf("expected 1 summary for owner, got %d", len(ownerSummaries))
	}
	if ownerSummaries[0].Role != "owner" {
		t.Errorf("owner summary Role = %q, want %q", ownerSummaries[0].Role, "owner")
	}
	if ownerSummaries[0].MemberCount != 2 {
		t.Errorf("MemberCount = %d, want 2", ownerSummaries[0].MemberCount)
	}

	memberSummaries, err := svc.GetUserOrganizationsWithSummary(ctx, member)
	if err != nil {
		t.Fatalf("GetUserOrganizationsWithSummary failed: %v", err)
	}
	if len(memberSummaries) != 1 {
		t.Fatalf("expected 1 summary for member, got %d", len(memberSummaries))
	}
	if memberSummaries[0].Role != "member" {
		t.Errorf("member summary Role = %q, want %q", memberSummaries[0].Role, "member")
	}
}

// GetOrganizationBySlug must resolve an existing slug and return a
// not-found error for one that does not exist.
func TestGetOrganizationBySlug(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "slugowner", "slugowner@example.com")

	created, err := svc.CreateOrganization(ctx, owner, "Slug Org", "slug-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	got, err := svc.GetOrganizationBySlug(ctx, "slug-org")
	if err != nil {
		t.Fatalf("GetOrganizationBySlug failed: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("got ID %d, want %d", got.ID, created.ID)
	}

	if _, err := svc.GetOrganizationBySlug(ctx, "does-not-exist"); err == nil {
		t.Error("expected error for nonexistent slug")
	}
}

// Membership add/list/detail round trip, including the username join in
// GetMembersWithUsernames.
func TestAddMemberByEmailAndListMembers(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "memowner", "memowner@example.com")
	insertOrgTestUser(t, st, "newmember", "newmember@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "Membership Org", "membership-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	if err := svc.AddMemberByEmail(ctx, org.ID, "newmember@example.com", "admin"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	// Unknown email must fail.
	if err := svc.AddMemberByEmail(ctx, org.ID, "nobody@example.com", "member"); err == nil {
		t.Error("expected error adding member with unknown email")
	}

	// Already-a-member must fail (owner is already a member).
	if err := svc.AddMemberByEmail(ctx, org.ID, "memowner@example.com", "member"); err == nil {
		t.Error("expected error adding an existing member again")
	}

	// Invalid role falls back to "member" rather than erroring.
	insertOrgTestUser(t, st, "roleuser", "roleuser@example.com")
	if err := svc.AddMemberByEmail(ctx, org.ID, "roleuser@example.com", "superadmin"); err != nil {
		t.Fatalf("AddMemberByEmail with invalid role unexpectedly failed: %v", err)
	}

	members, err := svc.GetOrgMembers(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetOrgMembers failed: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members (owner + admin + fallback-member), got %d", len(members))
	}

	detailed, err := svc.GetMembersWithUsernames(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetMembersWithUsernames failed: %v", err)
	}
	if len(detailed) != 3 {
		t.Fatalf("expected 3 detailed members, got %d", len(detailed))
	}
	usernames := map[string]string{}
	for _, m := range detailed {
		usernames[m.Username] = m.Role
	}
	if usernames["memowner"] != "owner" {
		t.Errorf("memowner role = %q, want %q", usernames["memowner"], "owner")
	}
	if usernames["newmember"] != "admin" {
		t.Errorf("newmember role = %q, want %q", usernames["newmember"], "admin")
	}
	if usernames["roleuser"] != "member" {
		t.Errorf("roleuser role = %q, want %q (invalid role should fall back)", usernames["roleuser"], "member")
	}
}

// IsMember must distinguish members from non-members and report the correct
// role, without erroring on a lookup for a user who was never a member.
func TestIsMember(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "ismemowner", "ismemowner@example.com")
	stranger := insertOrgTestUser(t, st, "stranger", "stranger@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "IsMember Org", "ismember-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	ok, role, err := svc.IsMember(ctx, org.ID, owner)
	if err != nil {
		t.Fatalf("IsMember failed: %v", err)
	}
	if !ok || role != "owner" {
		t.Errorf("IsMember(owner) = %v, %q, want true, \"owner\"", ok, role)
	}

	ok, role, err = svc.IsMember(ctx, org.ID, stranger)
	if err != nil {
		t.Fatalf("IsMember failed: %v", err)
	}
	if ok {
		t.Error("expected stranger to not be a member")
	}
	if role != "" {
		t.Errorf("expected empty role for non-member, got %q", role)
	}
}

// RemoveMember must succeed for ordinary members but refuse to remove the
// owner — the owner can only be replaced via TransferOwnership.
func TestRemoveMember(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "rmowner", "rmowner@example.com")
	insertOrgTestUser(t, st, "rmmember", "rmmember@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "Remove Org", "remove-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := svc.AddMemberByEmail(ctx, org.ID, "rmmember@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	memberID := memberUserID(t, ctx, st, org.ID, "rmmember@example.com")

	// Removing the owner must be refused.
	if err := svc.RemoveMember(ctx, org.ID, owner); err == nil {
		t.Error("expected error removing the organization owner")
	}
	if ok, _, _ := svc.IsMember(ctx, org.ID, owner); !ok {
		t.Error("owner must still be a member after failed removal attempt")
	}

	// Removing an ordinary member must succeed.
	if err := svc.RemoveMember(ctx, org.ID, memberID); err != nil {
		t.Fatalf("RemoveMember failed for ordinary member: %v", err)
	}
	if ok, _, _ := svc.IsMember(ctx, org.ID, memberID); ok {
		t.Error("member must no longer be a member after removal")
	}

	// Removing a user who was never a member must error.
	if err := svc.RemoveMember(ctx, org.ID, memberID); err == nil {
		t.Error("expected error removing an already-removed / unknown member")
	}
}

// memberUserID looks up the user ID for a given email, for tests that need
// to act on a member after adding them by email.
func memberUserID(t *testing.T, ctx context.Context, st *store.Store, orgID int64, email string) int64 {
	t.Helper()

	var id int64
	err := st.UsersDB.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&id)
	if err != nil {
		t.Fatalf("failed to look up user by email %q: %v", email, err)
	}
	return id
}

// ChangeMemberRole must move between member/admin, reject an owner
// promotion attempt (must go through TransferOwnership instead), reject
// invalid role names, and refuse to touch the owner's own role.
func TestChangeMemberRole(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "crowner", "crowner@example.com")
	insertOrgTestUser(t, st, "crmember", "crmember@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "ChangeRole Org", "changerole-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := svc.AddMemberByEmail(ctx, org.ID, "crmember@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}
	memberID := memberUserID(t, ctx, st, org.ID, "crmember@example.com")

	// member -> admin
	if err := svc.ChangeMemberRole(ctx, org.ID, memberID, "admin"); err != nil {
		t.Fatalf("ChangeMemberRole to admin failed: %v", err)
	}
	if _, role, _ := svc.IsMember(ctx, org.ID, memberID); role != "admin" {
		t.Errorf("role after promotion = %q, want %q", role, "admin")
	}

	// admin -> member
	if err := svc.ChangeMemberRole(ctx, org.ID, memberID, "member"); err != nil {
		t.Fatalf("ChangeMemberRole to member failed: %v", err)
	}
	if _, role, _ := svc.IsMember(ctx, org.ID, memberID); role != "member" {
		t.Errorf("role after demotion = %q, want %q", role, "member")
	}

	// Promotion to "owner" is not a valid target for this call.
	if err := svc.ChangeMemberRole(ctx, org.ID, memberID, "owner"); err == nil {
		t.Error("expected error promoting directly to owner via ChangeMemberRole")
	}

	// Nonsense role.
	if err := svc.ChangeMemberRole(ctx, org.ID, memberID, "superuser"); err == nil {
		t.Error("expected error for invalid role name")
	}

	// Cannot change the owner's own role.
	if err := svc.ChangeMemberRole(ctx, org.ID, owner, "admin"); err == nil {
		t.Error("expected error changing the owner's role")
	}

	// Unknown member.
	stranger := insertOrgTestUser(t, st, "crstranger", "crstranger@example.com")
	if err := svc.ChangeMemberRole(ctx, org.ID, stranger, "admin"); err == nil {
		t.Error("expected error changing role of a non-member")
	}
}

// TransferOwnership must move ownership atomically: new owner becomes
// "owner", old owner is demoted to "admin" (never removed), and
// organizations.owner_id is kept in sync. Guard-rail cases (non-owner
// caller, missing org, target not already a member) must all fail cleanly
// without partial state changes.
func TestTransferOwnership(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "tostart", "tostart@example.com")
	insertOrgTestUser(t, st, "tonew", "tonew@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "Transfer Org", "transfer-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := svc.AddMemberByEmail(ctx, org.ID, "tonew@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}
	newOwnerID := memberUserID(t, ctx, st, org.ID, "tonew@example.com")

	// Non-member cannot be promoted to owner.
	nonMember := insertOrgTestUser(t, st, "tononmember", "tononmember@example.com")
	if err := svc.TransferOwnership(ctx, org.ID, owner, nonMember); err == nil {
		t.Error("expected error transferring ownership to a non-member")
	}

	// Caller who is not the current owner cannot transfer.
	if err := svc.TransferOwnership(ctx, org.ID, newOwnerID, newOwnerID); err == nil {
		t.Error("expected error when caller is not the current owner")
	}

	// Nonexistent org.
	if err := svc.TransferOwnership(ctx, 999999, owner, newOwnerID); err == nil {
		t.Error("expected error transferring ownership of a nonexistent org")
	}

	// Valid transfer.
	if err := svc.TransferOwnership(ctx, org.ID, owner, newOwnerID); err != nil {
		t.Fatalf("TransferOwnership failed: %v", err)
	}

	if _, role, _ := svc.IsMember(ctx, org.ID, newOwnerID); role != "owner" {
		t.Errorf("new owner role = %q, want %q", role, "owner")
	}
	if _, role, _ := svc.IsMember(ctx, org.ID, owner); role != "admin" {
		t.Errorf("old owner role = %q, want %q (demoted, not removed)", role, "admin")
	}

	got, err := svc.GetOrganizationBySlug(ctx, "transfer-org")
	if err != nil {
		t.Fatalf("GetOrganizationBySlug failed: %v", err)
	}
	if got.OwnerID != newOwnerID {
		t.Errorf("organizations.owner_id = %d, want %d", got.OwnerID, newOwnerID)
	}

	// Old owner (now admin) can no longer transfer ownership themselves.
	if err := svc.TransferOwnership(ctx, org.ID, owner, owner); err == nil {
		t.Error("expected error transferring ownership from the now-demoted former owner")
	}
}

// Deleting an org's row directly must cascade-delete its org_members rows
// (schema-level ON DELETE CASCADE) — the service has no dedicated delete
// method, so this exercises the FK contract the service relies on for
// consistent membership state.
func TestDeleteOrganizationCascadesMembers(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "delowner", "delowner@example.com")
	insertOrgTestUser(t, st, "delmember", "delmember@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "Delete Org", "delete-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	if err := svc.AddMemberByEmail(ctx, org.ID, "delmember@example.com", "member"); err != nil {
		t.Fatalf("AddMemberByEmail failed: %v", err)
	}

	members, err := svc.GetOrgMembers(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetOrgMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members before delete, got %d", len(members))
	}

	if _, err := st.UsersDB.ExecContext(ctx, `DELETE FROM organizations WHERE id = ?`, org.ID); err != nil {
		t.Fatalf("failed to delete organization: %v", err)
	}

	membersAfter, err := svc.GetOrgMembers(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetOrgMembers after delete failed: %v", err)
	}
	if len(membersAfter) != 0 {
		t.Errorf("expected 0 members after org delete (FK cascade), got %d", len(membersAfter))
	}

	if _, err := svc.GetOrganizationBySlug(ctx, "delete-org"); err == nil {
		t.Error("expected not-found error for a deleted organization's slug")
	}
}

// Org-scoped tokens: create, list, revoke, and the IDOR guard that a token
// belonging to one org cannot be revoked by supplying a different orgID.
func TestOrgTokenLifecycle(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewOrgService(st)
	ctx := context.Background()
	owner := insertOrgTestUser(t, st, "tokowner", "tokowner@example.com")

	org, err := svc.CreateOrganization(ctx, owner, "Token Org", "token-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}
	otherOrg, err := svc.CreateOrganization(ctx, owner, "Other Token Org", "other-token-org")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	tok, plain, err := svc.CreateOrgToken(ctx, org.ID, owner, "ci-token", []string{"read"})
	if err != nil {
		t.Fatalf("CreateOrgToken failed: %v", err)
	}
	if plain == "" {
		t.Fatal("expected non-empty plaintext token")
	}
	if tok.OrgID != org.ID {
		t.Errorf("token OrgID = %d, want %d", tok.OrgID, org.ID)
	}

	tokens, err := svc.ListOrgTokens(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListOrgTokens failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}

	// Revoking via the wrong org ID must fail and leave the token intact.
	if err := svc.RevokeOrgToken(ctx, tok.ID, otherOrg.ID); err == nil {
		t.Error("expected error revoking a token via a mismatched org ID")
	}
	tokens, err = svc.ListOrgTokens(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListOrgTokens failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected token to survive mismatched-org revoke attempt, got %d tokens", len(tokens))
	}

	// Correct org ID succeeds.
	if err := svc.RevokeOrgToken(ctx, tok.ID, org.ID); err != nil {
		t.Fatalf("RevokeOrgToken failed: %v", err)
	}
	tokens, err = svc.ListOrgTokens(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListOrgTokens failed: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after revoke, got %d", len(tokens))
	}

	// Revoking an already-revoked (nonexistent) token must error.
	if err := svc.RevokeOrgToken(ctx, tok.ID, org.ID); err == nil {
		t.Error("expected error revoking an already-revoked token")
	}
}
