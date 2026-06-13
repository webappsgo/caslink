# API Reference

## Interactive Documentation

| Tool | URL |
|------|-----|
| Swagger UI | `/server/docs/swagger` |
| GraphiQL | `/graphiql` |
| OpenAPI JSON spec | `/api/v1/server/swagger` (also aliased at `/api/swagger`) |
| GraphQL schema | `GET /graphql/schema` |

## Overview

All REST endpoints are versioned under `/api/v1/`. The API uses a consistent JSON envelope:

**Success:**

```json
{ "ok": true, "data": { ... } }
```

**Error (RFC 7807):**

```json
{ "ok": false, "error": "CODE", "message": "Human-readable description" }
```

Every request and response carries an `X-Request-ID` header for tracing.

## Authentication

All write endpoints and user-specific read endpoints require a Bearer token:

```
Authorization: Bearer <token>
```

Tokens are issued per-user or per-organization and stored as SHA-256 hashes. The raw token is shown only once at creation. Token prefixes identify the token type:

| Prefix | Scope |
|--------|-------|
| `adm_` | Admin API access |
| `usr_` | User-level API access |
| `org_` | Organization-scoped access |

Manage tokens via the web UI at `/users/tokens` or via the API at `/api/v1/users/tokens`.

## Autodiscovery

```
GET /api/autodiscover
```

Not versioned — call this before you know the server's API version. Returns the server's base URL, API version, capabilities, and Tor `.onion` address (if running).

## Public Endpoints (No Auth)

### Health Check

```
GET /server/healthz       → HTML/JSON/text (content negotiation)
GET /healthz              → same as above (alias)
GET /api/v1/server/healthz → JSON only
GET /api/v1/healthz       → JSON only
GET /api/v1/version       → JSON version info
GET /version              → JSON version info
```

### URL Redirect

```
GET /{code}
```

Redirects to the original URL. Tracks a click (GeoIP, referrer, device) if analytics are enabled.

### URL Info (Public)

```
GET /api/v1/urls/{code}
GET /api/v1/urls/{code}/stats
```

### QR Code

```
GET /api/v1/qr/{code}
GET /api/v1/qr/{code}?size=512&format=svg&error_correction=high
```

Query parameters: `size` (pixels, max 2048), `format` (`png`, `svg`, `pdf`), `error_correction` (`low`, `medium`, `quartile`, `high`).

### Server Information

```
GET /api/v1/server/about
GET /api/v1/server/help
GET /api/v1/server/privacy
GET /api/v1/server/terms
POST /api/v1/server/contact
```

### Authentication (Rate-Limited)

```
POST /api/v1/server/auth/login
POST /api/v1/server/auth/register
```

## URL Management

### Create Short URL (requires auth)

```
POST /api/v1/urls
Authorization: Bearer usr_...
Content-Type: application/json

{
  "url": "https://example.com/very/long/path",
  "custom_code": "ex",          // optional; omit for auto-generated code
  "expiration": "7d",           // optional: 1h | 24h | 7d | 30d | never
  "password": "",               // optional: password-protect the link
  "utm_source": "",             // optional UTM parameters
  "utm_medium": "",
  "utm_campaign": ""
}
```

Response:

```json
{
  "ok": true,
  "data": {
    "code": "ex",
    "short_url": "https://your-domain.com/ex",
    "original_url": "https://example.com/very/long/path",
    "created_at": "2026-06-13T10:00:00Z"
  }
}
```

## User API

All endpoints require `Authorization: Bearer usr_...`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users/` | Current user profile |
| `GET` | `/api/v1/users/tokens` | List API tokens |
| `GET` | `/api/v1/users/settings` | User settings |
| `GET` | `/api/v1/users/security` | Security status (2FA, passkeys) |
| `GET` | `/api/v1/users/urls/export` | Export all user URLs (CSV/JSON) |
| `POST` | `/api/v1/users/urls/import` | Import URLs from CSV/JSON |

## Organization API

All endpoints require `Authorization: Bearer usr_...` or `org_...`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/orgs/` | List organizations the user belongs to |
| `POST` | `/api/v1/orgs/` | Create a new organization |
| `GET` | `/api/v1/orgs/{slug}` | Get organization details |
| `GET` | `/api/v1/orgs/{slug}/members` | List organization members |
| `GET` | `/api/v1/orgs/{slug}/tokens` | List org-scoped API tokens |
| `POST` | `/api/v1/orgs/{slug}/tokens` | Create org-scoped API token |
| `DELETE` | `/api/v1/orgs/{slug}/tokens/{tokenID}` | Revoke org token |
| `POST` | `/api/v1/orgs/{slug}/transfer` | Transfer org ownership |

## Admin API

All endpoints require `Authorization: Bearer adm_...`.

Base path: `/api/v1/server/{admin_path}/` where `admin_path` defaults to `admin`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `.../config/users` | List all users |
| `GET` | `.../config/users/{id}` | Get user detail |
| `POST` | `.../config/users/{id}/suspend` | Suspend a user |
| `POST` | `.../config/users/{id}/activate` | Activate a user |
| `POST` | `.../config/users/{id}/recovery-keys` | Regenerate user recovery keys |
| `GET` | `.../config/settings` | Get server settings |
| `PATCH` | `.../config/settings` | Update server settings |
| `GET` | `.../config/branding` | Get branding config |
| `PATCH` | `.../config/branding` | Update branding config |
| `GET` | `.../config/info` | Server info (version, runtime, uptime) |
| `GET` | `.../config/scheduler` | Scheduler task list and status |
| `GET` | `.../config/maintenance` | Maintenance status |
| `PATCH` | `.../config/maintenance` | Trigger maintenance action |
| `GET` | `.../config/network/tor` | Tor service status and `.onion` address |

## GraphQL

```
POST /graphql
Content-Type: application/json

{ "query": "{ urls { code originalUrl clicks } }" }
```

Explore the schema interactively at `/graphiql` or fetch it as text at `GET /graphql/schema`.

## CSP Violation Reports

Browsers post CSP violation reports to:

```
POST /api/v1/server/reports/csp
```

This endpoint accepts the browser POST and returns `204 No Content`.

## Well-Known Endpoints

| Path | Description |
|------|-------------|
| `/.well-known/security.txt` | RFC 9116 security contact |
| `/.well-known/change-password` | Redirect to password-change page |
| `/.well-known/acme-challenge/{token}` | Let's Encrypt HTTP-01 challenge |

## Static Resources

| Path | Description |
|------|-------------|
| `/robots.txt` | Robots exclusion (admin path disallowed) |
| `/sitemap.xml` | Sitemap for public pages |
| `/locales/{lang}.json` | Embedded locale files (en, es, fr, de, zh, ar, ja) |
| `/static/*` | CSS, JS, fonts, PWA manifest |
