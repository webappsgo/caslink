# API Reference

Caslink provides a JSON REST API, a GraphQL endpoint, and server-rendered pages. The default REST prefix is `/api/v1`; `server.api.base_path` can change it. The unauthenticated `GET /api/autodiscover` endpoint reports the configured API base path and server capabilities.

## Documentation and response format

- OpenAPI JSON: `/api/swagger` and `{api_base_path}/server/swagger`.
- Browser API documentation: `/server/docs/swagger`.
- GraphQL console: `/server/docs/graphql`.
- GraphQL POST endpoints: `/api/graphql` and `{api_base_path}/server/graphql`.

Successful JSON responses use `{ "ok": true, "data": ... }`. Errors use `{ "ok": false, "error": "CODE", "message": "..." }`. Frontend routes negotiate HTML or plain text; API routes negotiate JSON or plain text. Requests that mutate browser state require the CSRF token. API requests use bearer tokens.

## Authentication

Send tokens in `Authorization: Bearer <token>`. Tokens are shown once when created and stored as SHA-256 hashes. `adm_` tokens are for server administration, `usr_` tokens for user operations, and `org_` tokens for organization-scoped access. A user token cannot access server-admin routes. Create and revoke user tokens through `/users/tokens` or `{api_base_path}/users/tokens`.

## Public endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/autodiscover` | API base path and capability discovery |
| GET | `/server/healthz` | Health page with HTML, JSON, or text negotiation |
| GET | `/healthz` | Same handler when `server.healthz.root.enabled` is true |
| GET | `/api/healthz` | Unversioned JSON health endpoint |
| GET | `{api_base_path}/server/healthz` | Versioned JSON health endpoint |
| GET | `/version`, `{api_base_path}/version` | Version, commit, and build date |
| GET | `/api/swagger`, `{api_base_path}/server/swagger` | OpenAPI JSON |
| POST | `/api/graphql`, `{api_base_path}/server/graphql` | GraphQL queries |
| GET | `{api_base_path}/urls/{code}` | Public link information |
| GET | `{api_base_path}/urls/{code}/stats` | Public link statistics where enabled |
| GET | `{api_base_path}/qr/{code}` | QR code in a supported image/document format |
| GET | `/server/about`, `/server/help`, `/server/privacy`, `/server/terms` | Public information pages |
| GET | `/.well-known/security.txt` | Security contact and disclosure information |
| GET | `/.well-known/change-password` | Password-change destination |
| GET | `/.well-known/acme-challenge/{token}` | Active ACME HTTP-01 challenge response |

The short-link redirect itself is `GET /{code}`. A link may require a password or may be private, expired, or restricted; the handler applies those link settings before redirecting.

## Authentication endpoints

| Method | Path | Access |
|---|---|---|
| POST | `{api_base_path}/server/auth/login` | Public, rate limited |
| POST | `{api_base_path}/server/auth/register` | Public when registration is enabled, rate limited |
| GET/POST | `/server/auth/password/forgot` | Public, rate limited |
| GET/POST | `/server/auth/password/reset/{token}` | Single-use reset token |
| GET/POST | `/server/auth/2fa` | Login challenge |
| GET/POST | `/server/auth/2fa/recovery` | Recovery-code challenge |

The HTML login and registration pages are under `/server/auth/`. Setup is first-run only and is described in [Installation](installation.md).

## Link API

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `{api_base_path}/urls` | Bearer token | List links visible to the token owner |
| POST | `{api_base_path}/urls` | Bearer token | Create a link |
| PATCH | `{api_base_path}/urls/{code}` | Bearer token | Update an owned link |
| DELETE | `{api_base_path}/urls/{code}` | Bearer token | Delete an owned link |
| GET | `{api_base_path}/urls/{code}` | Public | Read public link details |
| GET | `{api_base_path}/urls/{code}/stats` | Public | Read permitted click statistics |
| GET | `{api_base_path}/qr/{code}` | Public | Generate QR image/document |
| GET/POST | `{api_base_path}/users/urls/export` and `/import` | User token | Export or import CSV/JSON links |

The server validates destinations, custom codes, ownership, expiration, passwords, visibility, and other configured restrictions. Consult the live OpenAPI document for request schemas and response fields.

## User API

All paths below are beneath `{api_base_path}/users` and require a user bearer token.

| Method | Path | Purpose |
|---|---|---|
| GET/PATCH | `/` | Read or update the current profile |
| GET/POST/DELETE | `/tokens`, `/tokens/{id}` | Manage API tokens |
| GET | `/settings` | Read user settings |
| GET | `/security` | Read security status |
| GET | `/urls/export` | Export owned links |
| POST | `/urls/import` | Import links |
| GET/POST | `/domains`, `/domains/{domain}/verify` | Add and verify custom domains |
| GET | `/domains/{domain}`, `/dns`, `/ssl` | Read domain, DNS, and SSL state |
| POST | `/domains/{domain}/ssl`, `/ssl/renew` | Configure or renew domain SSL |
| DELETE | `/domains/{domain}` | Remove a custom domain |

WebAuthn/passkey ceremonies are session-protected at `/users/passkeys/begin-register`, `/finish-register`, `/begin-login`, and `/finish-login`.

## Organization API

All paths are beneath `{api_base_path}/orgs` and require a bearer token with appropriate organization membership and role.

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/` | List or create organizations |
| GET/PATCH/DELETE | `/{slug}` | Read, update, or delete an organization |
| GET | `/{slug}/members` | List members |
| GET/POST/DELETE | `/{slug}/invites`, `/{slug}/invites/{inviteID}` | Manage invitations |
| GET/POST/DELETE | `/{slug}/tokens`, `/{slug}/tokens/{tokenID}` | Manage organization tokens |
| POST | `/{slug}/transfer` | Transfer ownership |
| GET/POST | `/{slug}/domains` | List or add organization domains |
| GET | `/{slug}/domains/{domain}` and `/dns`, `/ssl` | Read domain state |
| POST | `/{slug}/domains/{domain}/verify`, `/ssl`, `/ssl/renew` | Verify or configure SSL |
| DELETE | `/{slug}/domains/{domain}` | Remove a domain |

## Admin API

Administrative paths are beneath `{api_base_path}/server/{admin_path}`. First-run `/setup/*` endpoints use the one-time setup token and setup session; after setup, administrative endpoints require an `adm_` token. `{admin_path}` is configured locally and is deliberately not disclosed by autodiscovery. The API includes user moderation and recovery-key regeneration; server settings, branding, info, and maintenance; scheduler tasks and history; custom-domain moderation; and OIDC, LDAP, and SAML provider management. The OpenAPI document lists the exact method, schema, and permissions for each endpoint.

## Contact, preferences, and security reporting

Public server information APIs are beneath `{api_base_path}/server/`: `about`, `help`, `privacy`, `terms`, `preferences`, `preferences/sync`, `preferences/sync/import`, `security`, `security/policy`, and `security/thanks`. Privacy returns the dynamic policy in the `summary`, `cookies`, `data`, `tracking`, `retention`, `third_party`, and `content` fields; `ccpa` is included only when data sales are enabled. Terms returns `{title, content, format}` with built-in Markdown defaults. `POST /server/contact` is the browser form; its API counterpart is `{api_base_path}/server/contact`. Security-report submission uses the rotating `security_id` shown by the security contact flow. See [Security](security.md) for the disclosure process and report encryption.

## I2P administration

Authenticated server-admin clients can read and update I2P settings at `GET` and `PATCH /api/v1/server/{admin_path}/network/i2p`. `POST /api/v1/server/{admin_path}/network/i2p/validate` validates a candidate configuration without saving it. `POST /api/v1/server/{admin_path}/network/i2p/restart` restarts the configured provider. `POST /api/v1/server/{admin_path}/network/i2p/regenerate` requires `{"confirm":true}` and replaces the persisted destination key. The public `/api/healthz` response reports I2P feature status; `/api/autodiscover` includes the address only when I2P is enabled, running, and `server.i2p.expose` is true.
