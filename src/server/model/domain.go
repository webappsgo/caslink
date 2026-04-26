package model

import (
	"errors"
	"time"
)

// CustomDomain represents a custom domain for user or organization
type CustomDomain struct {
	ID                 int64      `json:"id"`
	OwnerType          string     `json:"owner_type"` // user, org
	OwnerID            int64      `json:"owner_id"`
	Domain             string     `json:"domain"`
	IsApex             bool       `json:"is_apex"`
	IsWildcard         bool       `json:"is_wildcard"`
	VerificationStatus string     `json:"verification_status"` // pending, verified, failed
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	VerifiedIP         *string    `json:"verified_ip,omitempty"`
	LastCheckAt        *time.Time `json:"last_check_at,omitempty"`
	CheckCount         int        `json:"check_count"`
	SSLEnabled         bool       `json:"ssl_enabled"`
	SSLStatus          string     `json:"ssl_status"` // none, pending, active, expired, error
	SSLChallenge       *string    `json:"ssl_challenge,omitempty"`
	SSLProvider        *string    `json:"ssl_provider,omitempty"`
	SSLIssuedAt        *time.Time `json:"ssl_issued_at,omitempty"`
	SSLExpiresAt       *time.Time `json:"ssl_expires_at,omitempty"`
	SSLLastError       *string    `json:"ssl_last_error,omitempty"`
	Status             string     `json:"status"` // pending, active, suspended, error
	SuspendedReason    *string    `json:"suspended_reason,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AddDomainRequest represents a request to add a custom domain
type AddDomainRequest struct {
	Domain string `json:"domain" validate:"required,fqdn"`
}

// DomainAudit represents a custom domain audit log entry
type DomainAudit struct {
	ID        int64     `json:"id"`
	DomainID  int64     `json:"domain_id"`
	Action    string    `json:"action"`
	ActorType string    `json:"actor_type"`
	ActorID   *int64    `json:"actor_id,omitempty"`
	Details   *string   `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Error definitions
var (
	ErrDomainNotFound         = errors.New("domain not found")
	ErrDomainAlreadyExists    = errors.New("domain already exists")
	ErrDomainNotVerified      = errors.New("domain not verified")
	ErrDomainLimitReached     = errors.New("domain limit reached")
	ErrDomainReserved         = errors.New("domain is reserved")
	ErrDomainBlockedPattern   = errors.New("domain matches blocked pattern")
	ErrSSLNotConfigured       = errors.New("SSL not configured")
	ErrSSLCertificateExpired  = errors.New("SSL certificate expired")
)
