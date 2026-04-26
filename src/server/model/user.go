package model

import (
	"errors"
	"time"
)

// User represents a regular user account
type User struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	EmailVerified bool       `json:"email_verified"`
	DisplayName   *string    `json:"display_name,omitempty"`
	Avatar        *string    `json:"avatar,omitempty"`
	Bio           *string    `json:"bio,omitempty"`
	TOTPEnabled   bool       `json:"totp_enabled"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLogin     *time.Time `json:"last_login,omitempty"`
}

// RegisterUserRequest represents a registration request
type RegisterUserRequest struct {
	Username string `json:"username" validate:"required,min=3,max=32"`
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Identifier string `json:"identifier" validate:"required"` // Username or email
	Password   string `json:"password" validate:"required"`
	RememberMe bool   `json:"remember_me"`
}

// Error definitions
var (
	ErrUserNotFound          = errors.New("user not found")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrUsernameBlocklisted   = errors.New("username contains blocked word")
	ErrInvalidUsername       = errors.New("invalid username format")
	ErrWeakPassword          = errors.New("password too weak")
)
