// Package templates exposes embedded text/HTML templates so callers in
// sibling packages can read them without violating the go:embed parent-
// directory restriction.
package templates

import _ "embed"

//go:embed email/password_reset.txt
var PasswordResetEmail string

//go:embed email/password_changed.txt
var PasswordChangedEmail string

//go:embed email/welcome_user.txt
var WelcomeUserEmail string

//go:embed email/welcome_admin.txt
var WelcomeAdminEmail string

//go:embed email/email_verify.txt
var EmailVerifyEmail string
