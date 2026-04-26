# Caslink TODO - Multi-User Implementation

## Authentication & Middleware (PART 23)

- [x] Implement authentication middleware for /user/* routes
- [x] Implement authentication middleware for /org/* routes
- [x] Implement org membership verification middleware
- [x] Add session validation to handlers

## User Profile & Settings (PART 23)

- [x] Implement /user/profile page and handlers (placeholder HTML)
- [x] Implement /user/settings page and handlers (placeholder HTML)
- [x] Implement /user/tokens (API token management) (placeholder HTML)
- [x] Implement /user/security routes (password change, 2FA, sessions) (placeholder HTML)
- [ ] Implement /user/security/password
- [ ] Implement /user/security/sessions
- [ ] Implement /user/security/2fa
- [ ] Implement /user/security/passkeys
- [ ] Implement /user/security/recovery

## Password Reset (PART 23 + PART 26)

- [x] Implement /auth/password/forgot page and handler
- [x] Implement /auth/password/reset/{token} page and handler
- [x] Implement password reset email service (checks SMTP per PART 26)
- [x] Implement password reset token generation and validation (24h expiry)
- [x] Add password_resets table to database schema
- [ ] Implement actual SMTP email sending
- [ ] Implement SHA256 token hashing
- [ ] Create password_reset email template per PART 26 format

## Two-Factor Authentication (PART 23)

- [ ] Implement TOTP/2FA setup flow
- [ ] Implement /auth/2fa verification step
- [ ] Implement QR code generation for TOTP setup
- [ ] Implement recovery key generation (10 keys, format: a1b2c3d4-e5f6)
- [ ] Implement recovery key validation
- [ ] Implement "Remember this device" functionality

## Passkeys/WebAuthn (PART 23)

- [ ] Implement WebAuthn registration
- [ ] Implement /auth/passkey route
- [ ] Implement passkey management UI
- [ ] Add passkeys table to database schema

## Organization Features (PART 23)

- [ ] Implement /org/{slug}/tokens (org API tokens)
- [ ] Implement /org/{slug}/members/invite
- [ ] Implement org member invite flow
- [ ] Implement /org/{slug}/roles page
- [ ] Implement /org/{slug}/security page
- [ ] Implement /org/{slug}/security/audit
- [ ] Implement /org/{slug}/security/audit/export
- [ ] Implement org ownership transfer
- [ ] Update URL handlers to support org context

## Custom Domains - DNS Verification (PART 35)

- [ ] Implement DNS lookup and verification logic
- [ ] Implement server public IP discovery
- [ ] Implement verification retry logic with backoff
- [ ] Implement /user/domains/{domain}/dns instructions page
- [ ] Implement /org/{slug}/domains/{domain}/dns instructions page

## Custom Domains - SSL Automation (PART 35)

- [ ] Implement Let's Encrypt HTTP-01 challenge
- [ ] Implement Let's Encrypt DNS-01 challenge
- [ ] Implement SSL certificate storage (encrypted in database)
- [ ] Implement SSL renewal scheduler task
- [ ] Implement /user/domains/{domain}/ssl page
- [ ] Implement SSL status monitoring

## Admin Moderation (PART 23)

- [ ] Implement /admin/server/moderation/users page
- [ ] Implement /admin/server/moderation/users/{id} detail page
- [ ] Implement /admin/server/moderation/orgs page
- [ ] Implement /admin/server/moderation/orgs/{slug} detail page
- [ ] Implement /admin/server/domains page
- [ ] Implement user suspension/activation
- [ ] Implement org suspension
- [ ] Implement domain suspension

## Web Frontend (PART 17)

- [ ] Create HTML templates for /auth/register
- [ ] Create HTML templates for /auth/login
- [ ] Create HTML templates for /user/profile
- [ ] Create HTML templates for /user/settings
- [ ] Create HTML templates for /org/* pages
- [ ] Create HTML templates for /user/domains/* pages
- [ ] Implement mobile-first CSS per PART 17
- [ ] Implement dark/light/auto theme switching

## API Endpoints (PART 20)

- [ ] Implement /api/v1/auth/register
- [ ] Implement /api/v1/auth/login
- [ ] Implement /api/v1/user/* API routes
- [ ] Implement /api/v1/org/{slug}/* API routes
- [ ] Implement /api/v1/user/domains/* API routes
- [ ] Implement /api/v1/org/{slug}/domains/* API routes
- [ ] Implement API authentication (Bearer token)

## Testing (PART 13)

- [ ] Write tests for username validation
- [ ] Write tests for user registration
- [ ] Write tests for organization creation
- [ ] Write tests for custom domain verification
- [ ] Write integration tests for auth flows

## Documentation (PART 33)

- [ ] Create docs/admin.md section for user moderation
- [ ] Update docs/configuration.md with new config options
- [ ] Update docs/api.md with user/org/domain endpoints
- [ ] Add examples for custom domain setup
