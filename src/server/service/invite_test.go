package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newInviteService(t *testing.T) (*InviteService, context.Context) {
	t.Helper()
	st := newFullSchemaStore(t)
	return NewInviteService(st), context.Background()
}

func TestCreateAndValidateInvite(t *testing.T) {
	svc, ctx := newInviteService(t)

	plaintext, inv, err := svc.CreateInvite(ctx, CreateInviteParams{
		Kind:      InviteKindUserRegistration,
		Email:     "  New@Example.COM ",
		CreatedBy: 42,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected non-empty plaintext token")
	}
	if inv.ID == 0 {
		t.Fatal("expected assigned invite id")
	}
	if inv.Email != "new@example.com" {
		t.Errorf("email not normalized: %q", inv.Email)
	}
	if inv.MaxUses != 1 {
		t.Errorf("default max_uses = %d, want 1", inv.MaxUses)
	}
	if time.Until(inv.ExpiresAt) < 6*24*time.Hour {
		t.Errorf("default expiry too short: %v", inv.ExpiresAt)
	}

	got, err := svc.ValidateInvite(ctx, plaintext, InviteKindUserRegistration)
	if err != nil {
		t.Fatalf("ValidateInvite: %v", err)
	}
	if got.ID != inv.ID {
		t.Errorf("validated id = %d, want %d", got.ID, inv.ID)
	}
}

func TestValidateInviteUnknownToken(t *testing.T) {
	svc, ctx := newInviteService(t)
	if _, err := svc.ValidateInvite(ctx, "does-not-exist", ""); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("err = %v, want ErrInviteInvalid", err)
	}
}

func TestCreateInviteBadKind(t *testing.T) {
	svc, ctx := newInviteService(t)
	if _, _, err := svc.CreateInvite(ctx, CreateInviteParams{Kind: "bogus"}); !errors.Is(err, ErrInviteBadKind) {
		t.Fatalf("err = %v, want ErrInviteBadKind", err)
	}
}

func TestValidateInviteKindMismatch(t *testing.T) {
	svc, ctx := newInviteService(t)
	plaintext, _, err := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindAdminRegistration})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if _, err := svc.ValidateInvite(ctx, plaintext, InviteKindUserRegistration); !errors.Is(err, ErrInviteKindMismatch) {
		t.Fatalf("err = %v, want ErrInviteKindMismatch", err)
	}
}

func TestConsumeInviteSingleUse(t *testing.T) {
	svc, ctx := newInviteService(t)
	plaintext, _, err := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	inv, err := svc.ConsumeInvite(ctx, plaintext, InviteKindUserRegistration, 7)
	if err != nil {
		t.Fatalf("first ConsumeInvite: %v", err)
	}
	if inv.UseCount != 1 {
		t.Errorf("use_count = %d, want 1", inv.UseCount)
	}
	if inv.UsedBy != 7 {
		t.Errorf("used_by = %d, want 7", inv.UsedBy)
	}
	if inv.UsedAt == nil {
		t.Error("expected used_at to be set")
	}

	// Second consume must fail — single-use.
	if _, err := svc.ConsumeInvite(ctx, plaintext, InviteKindUserRegistration, 8); !errors.Is(err, ErrInviteUsed) {
		t.Fatalf("second consume err = %v, want ErrInviteUsed", err)
	}
	// And validation now reports used.
	if _, err := svc.ValidateInvite(ctx, plaintext, ""); !errors.Is(err, ErrInviteUsed) {
		t.Fatalf("post-use validate err = %v, want ErrInviteUsed", err)
	}
}

func TestConsumeInviteMultiUse(t *testing.T) {
	svc, ctx := newInviteService(t)
	plaintext, _, err := svc.CreateInvite(ctx, CreateInviteParams{
		Kind:    InviteKindUserRegistration,
		MaxUses: 3,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.ConsumeInvite(ctx, plaintext, "", int64(i+1)); err != nil {
			t.Fatalf("consume %d: %v", i+1, err)
		}
	}
	if _, err := svc.ConsumeInvite(ctx, plaintext, "", 99); !errors.Is(err, ErrInviteUsed) {
		t.Fatalf("4th consume err = %v, want ErrInviteUsed", err)
	}
}

func TestConsumeInviteUnlimited(t *testing.T) {
	svc, ctx := newInviteService(t)
	plaintext, inv, err := svc.CreateInvite(ctx, CreateInviteParams{
		Kind:    InviteKindUserRegistration,
		MaxUses: -1,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if inv.MaxUses != 0 {
		t.Fatalf("unlimited invite stored max_uses = %d, want 0", inv.MaxUses)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.ConsumeInvite(ctx, plaintext, "", int64(i+1)); err != nil {
			t.Fatalf("unlimited consume %d: %v", i, err)
		}
	}
}

func TestConsumeInviteExpired(t *testing.T) {
	svc, ctx := newInviteService(t)
	plaintext, inv, err := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	// Force expiry into the past.
	if _, err := svc.store.UsersDB.ExecContext(ctx,
		`UPDATE invites SET expires_at = ? WHERE id = ?`,
		time.Now().Add(-time.Hour).Unix(), inv.ID); err != nil {
		t.Fatalf("force expiry: %v", err)
	}
	if _, err := svc.ValidateInvite(ctx, plaintext, ""); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("validate err = %v, want ErrInviteExpired", err)
	}
	if _, err := svc.ConsumeInvite(ctx, plaintext, "", 1); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("consume err = %v, want ErrInviteExpired", err)
	}
}

func TestRevokeInvite(t *testing.T) {
	svc, ctx := newInviteService(t)
	plaintext, inv, err := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if err := svc.RevokeInvite(ctx, inv.ID); err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	if _, err := svc.ValidateInvite(ctx, plaintext, ""); !errors.Is(err, ErrInviteRevoked) {
		t.Fatalf("validate err = %v, want ErrInviteRevoked", err)
	}
	if _, err := svc.ConsumeInvite(ctx, plaintext, "", 1); !errors.Is(err, ErrInviteRevoked) {
		t.Fatalf("consume err = %v, want ErrInviteRevoked", err)
	}
}

func TestRevokeUnusedByKind(t *testing.T) {
	svc, ctx := newInviteService(t)
	// Two unused user invites, one consumed user invite, one admin invite.
	p1, _, _ := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	_, _, _ = svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	pUsed, _, _ := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	pAdmin, _, _ := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindAdminRegistration})
	if _, err := svc.ConsumeInvite(ctx, pUsed, "", 1); err != nil {
		t.Fatalf("consume: %v", err)
	}

	n, err := svc.RevokeUnusedByKind(ctx, InviteKindUserRegistration)
	if err != nil {
		t.Fatalf("RevokeUnusedByKind: %v", err)
	}
	if n != 2 {
		t.Errorf("revoked %d, want 2", n)
	}
	// Unused user invite is now revoked.
	if _, err := svc.ValidateInvite(ctx, p1, ""); !errors.Is(err, ErrInviteRevoked) {
		t.Errorf("p1 err = %v, want ErrInviteRevoked", err)
	}
	// Admin invite untouched.
	if _, err := svc.ValidateInvite(ctx, pAdmin, ""); err != nil {
		t.Errorf("admin invite unexpectedly affected: %v", err)
	}
}

func TestCleanupExpiredInvites(t *testing.T) {
	svc, ctx := newInviteService(t)
	_, live, _ := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	_, dead, _ := svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})
	if _, err := svc.store.UsersDB.ExecContext(ctx,
		`UPDATE invites SET expires_at = ? WHERE id = ?`,
		time.Now().Add(-time.Hour).Unix(), dead.ID); err != nil {
		t.Fatalf("force expiry: %v", err)
	}

	n, err := svc.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("cleaned %d, want 1", n)
	}
	var count int
	if err := svc.store.UsersDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM invites WHERE id = ?`, live.ID).Scan(&count); err != nil {
		t.Fatalf("count live: %v", err)
	}
	if count != 1 {
		t.Errorf("live invite missing after cleanup")
	}
}

func TestListInvitesByKind(t *testing.T) {
	svc, ctx := newInviteService(t)
	_, _, _ = svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindAdminRegistration})
	_, _, _ = svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindAdminRegistration})
	_, _, _ = svc.CreateInvite(ctx, CreateInviteParams{Kind: InviteKindUserRegistration})

	list, err := svc.ListInvitesByKind(ctx, InviteKindAdminRegistration, 0)
	if err != nil {
		t.Fatalf("ListInvitesByKind: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("listed %d admin invites, want 2", len(list))
	}
}
