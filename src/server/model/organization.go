package model

import (
	"errors"
	"time"
)

// Organization represents an organization/team
type Organization struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	OwnerID   int64     `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgMember represents an organization member
type OrgMember struct {
	ID       int64     `json:"id"`
	OrgID    int64     `json:"org_id"`
	UserID   int64     `json:"user_id"`
	Role     string    `json:"role"` // owner, admin, member
	JoinedAt time.Time `json:"joined_at"`
}

// CreateOrgRequest represents a request to create an organization
type CreateOrgRequest struct {
	Name string `json:"name" validate:"required,min=3,max=100"`
	Slug string `json:"slug,omitempty" validate:"omitempty,min=3,max=50"`
}

// UpdateOrgRequest represents a request to update an organization
type UpdateOrgRequest struct {
	Name *string `json:"name,omitempty" validate:"omitempty,min=3,max=100"`
}

// InviteMemberRequest represents a request to invite a member
type InviteMemberRequest struct {
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role" validate:"required,oneof=admin member"`
}

// Error definitions
var (
	ErrOrgNotFound           = errors.New("organization not found")
	ErrOrgSlugAlreadyExists  = errors.New("organization slug already exists")
	ErrNotOrgMember          = errors.New("not an organization member")
	ErrInsufficientOrgPerms  = errors.New("insufficient organization permissions")
	ErrCannotLeaveAsOwner    = errors.New("owner cannot leave organization")
	ErrOrgLimitReached       = errors.New("organization limit reached")
)
