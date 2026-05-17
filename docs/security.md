# Security

This page documents the security model, authentication, public endpoints, and how to report vulnerabilities.

## Authentication

### Password Authentication

- All passwords are hashed with **Argon2id** (never bcrypt, scrypt, or MD5/SHA).
- Login accepts **username or email** plus password.
- Rate-limited: 5 failed attempts per 15 minutes per IP; account lockout after repeated failures.
- "Wrong password" and "no such user" return identical messages to prevent account enumeration.

### Sessions

- Short-lived **JWT** session tokens issued on login.
- Token lifetime is configurable (`session_lifetime`).
- All session tokens are invalidated on password change.

### API Tokens

- Long-lived **Bearer tokens** for API access.
- Managed at `/user/tokens` in the web UI and via `caslink-cli user tokens`.
- Stored as **SHA-256 hashes** — the raw token is shown only once at creation.
- Org-scoped tokens available for organization API access.

### Two-Factor Authentication (2FA)

| Method | Description |
|--------|-------------|
| TOTP | RFC 6238 time-based one-time passwords (any authenticator app) |
| Passkeys/WebAuthn | Hardware security keys and platform authenticators |
| Remember device | 30-day device trust cookie (configurable) |

### OAuth2 / OIDC Social Login

Social login is supported with Google, GitHub, and any OIDC-compatible provider. Configure in the admin panel under **Settings → Authentication → OAuth2**.

### Password Reset

- Time-limited tokens (24 hours), stored as **SHA-256 hashes** — never in plaintext.
- Token is single-use.
- Account existence is never confirmed on the reset request page (enumeration mitigation).

## Authorization

- **Server Admins** manage the application (separate from regular user accounts).
- **Primary Admin** is the first admin; cannot be deleted.
- **Regular Users** own their own links and cannot see other users' private links.
- **Organization roles**: owner, admin, member — with per-role permissions.

## Transport Security

- All production traffic should be served over **HTTPS**.
- Caslink manages **Let's Encrypt** certificates automatically (HTTP-01 and DNS-01 challenges).
- TLS certificates are stored encrypted in the database.
- Auto-renewal runs via the built-in scheduler.

## Tor Hidden Service

When the `tor` binary is present on the system, Caslink automatically creates a `.onion` hidden service. The address is shown in the admin panel under **Settings → Network → Tor**.

## Public Endpoints

The following endpoints are publicly accessible without authentication:

| Endpoint | Description |
|----------|-------------|
| `/` | Public homepage / link redirect |
| `/{slug}` | Short link redirect |
| `/server/healthz` | Health check (HTML/JSON/text via content negotiation) |
| `/healthz` | Alias for health check |
| `/api/v1/server/healthz` | Health check (JSON) |
| `/server/about` | About page |
| `/server/help` | Help page with API examples |
| `/swagger` | Swagger UI (API documentation) |
| `/graphiql` | GraphQL explorer |

The `/metrics` endpoint (Prometheus) is **internal-only** and never publicly exposed. Optionally protected with bearer token auth.

## Well-Known Namespace

| Path | Purpose |
|------|---------|
| `/.well-known/security.txt` | Security contact and disclosure policy |
| `/.well-known/apple-app-site-association` | iOS universal links |
| `/.well-known/assetlinks.json` | Android app links |
| `/.well-known/acme-challenge/` | Let's Encrypt HTTP-01 challenge |

## Security Headers

All responses include:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: SAMEORIGIN`
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Content-Security-Policy` (configurable; strict default)

## CSRF Protection

All state-mutating web forms are CSRF-protected with a double-submit cookie pattern.

## Rate Limiting

| Endpoint | Limit |
|----------|-------|
| Login | 5 attempts / 15 min |
| Password reset | 3 requests / 1 hour |
| API (general) | Configurable per-user and per-org |
| Registration | Configurable |

## GeoIP

GeoIP is used as a **risk signal only** — never as a sole access gate. VPN/proxy usage is detected and logged. Stored IPs are anonymized when `analytics.anonymize_ips: true` (default on new installs).

## Audit Log

Security-relevant events are written to an append-only audit log:

- Login success/failure
- Password and 2FA changes
- Admin actions
- Organization permission changes
- Data exports
- Token creation and revocation

Audit log entries never contain raw credentials or tokens.

## Reporting Vulnerabilities

Please report security vulnerabilities via the coordinated disclosure process:

- **Email**: See [`.github/SECURITY.md`](https://github.com/casapps/caslink/blob/main/.github/SECURITY.md)
- **Security advisory**: [GitHub Security Advisories](https://github.com/casapps/caslink/security/advisories/new)

Do not open public issues for security vulnerabilities.
