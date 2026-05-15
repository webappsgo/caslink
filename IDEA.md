## Project description

Caslink is a secure, mobile-first, fully self-hosted URL shortener written in Go that ships as a single static binary with zero external dependencies. It targets individuals and teams who want the control of self-hosting without the operational complexity of multi-service stacks. Any user can shorten links, track clicks, generate QR codes, and manage custom branded domains — all features ship to all users, no tier gating. Organizations layer collaborative ownership and RBAC on top of the individual model. A built-in billing module lets operators optionally monetize their instance, and a federation protocol lets instances discover and sync public links with one another.

## Project variables

project_name: caslink
project_org: casapps
internal_name: caslink
internal_org: casapps
app_name: Caslink
official_site: https://caslink.casapps.us
default_port: 64580
config_dir: /etc/casapps/caslink
data_dir: /var/lib/casapps/caslink
config_file: server.yml
env_prefix: CASLINK
go_module: github.com/casjaysdevdocker/caslink
container_registry: ghcr.io
docker_image: casapps/caslink

## Business logic

### Core URL shortening

- Any visitor can resolve a short code to its destination via a redirect (301/302 configurable per link).
- Authenticated users create short links with optional custom codes, titles, tags, and expiration dates.
- Short codes are random alphanumeric (min length configurable, default 6) or user-supplied custom codes.
- A link may be public (visible in the directory) or private (owner + org members only).
- Links support click-through password protection, geo-restriction, and device targeting.
- Bulk import from CSV or JSON; bulk export of owned links to CSV or JSON.

### Users and authentication

- Public registration is on by default; operators may disable it via config.
- Login accepts username or email + password (Argon2id hashed, never bcrypt).
- Session tokens are short-lived JWTs; API access uses long-lived Bearer tokens managed under `/user/tokens`.
- Second factor options: TOTP (RFC 6238, QR setup flow, 10 recovery codes in `a1b2c3d4-e5f6` format), Passkeys/WebAuthn, "remember this device" cookie.
- OAuth2/OIDC social login supported (Google, GitHub, etc.) via configurable providers.
- Password reset flow: email a time-limited token (24 h expiry, SHA-256 hashed in DB, never stored raw).
- Users own their links; they cannot see other users' private links.

### Organizations

- Users may create up to 5 organizations (configurable limit).
- Roles: owner, admin, member — each with decreasing write permissions.
- Org members collaborate on links owned by the org.
- Org API tokens scoped to the org; separate from user tokens.
- Org ownership transfer available; deleting an org requires owner confirmation.
- Audit log tracks all permission changes, member additions, and link mutations within an org.

### Custom domains

- Each user may attach up to 5 custom domains (configurable).
- Each org may attach up to 20 custom domains (configurable).
- Ownership verified via DNS TXT record; server discovers its own public IP for A-record instructions.
- Verification retries with exponential backoff.
- Automatic SSL via Let's Encrypt (HTTP-01 and DNS-01 challenges); certificates stored encrypted in the database.
- Renewal scheduled automatically; SSL status surfaced in the UI.
- Apex domains and subdomains both supported.

### Analytics

- Every click is recorded: timestamp, referrer, user-agent, device type, country, city (GeoIP2 database).
- IPs anonymized before storage when `analytics.anonymize_ips` is true.
- Real-time dashboard per link: clicks over time, top referrers, top countries, device breakdown.
- Aggregate reports across all owned links.
- GeoIP database updated on a schedule; stored under `{data_dir}/security/geoip/`.
- Bots excluded from counts when `analytics.exclude_bots` is true.

### QR codes

- Generated on demand for any short link.
- Output formats: PNG, SVG, PDF.
- Customizable: color, logo overlay, error correction level, quiet zone size.

### API surface

- REST API at `/api/v1/` — versioned, plural nouns, no trailing slash.
- GraphQL API at `/graphql` for flexible querying.
- OpenAPI/Swagger spec served at `/api/docs`.
- All endpoints return `{"ok":true,"data":{...}}` on success; RFC 7807 error body on failure.
- X-Request-ID propagated through every request.
- Rate limiting on all auth endpoints (login, register, password reset, OTP) with exponential backoff and lockout.

### Admin panel

- `/admin` — server-wide configuration, user moderation, org moderation, domain management.
- Operators can suspend/unsuspend users, orgs, and custom domains.
- Username blocklist prevents impersonation of well-known names.
- Maintenance: manual backup trigger, migration status, server health.

### Billing (optional, operator-controlled)

- Plans with monthly/yearly pricing in cents, trial days, per-plan feature limits.
- Payment providers: Stripe, Paddle, PayPal, LemonSqueezy, Manual.
- Subscriptions, invoicing, dunning (failed-payment retry), usage tracking.
- Webhook handling for provider events.
- Billing is disabled by default; operators opt in via config.

### Federation (optional)

- Instances may peer with other Caslink instances.
- Signed federation messages (`FederationMessage`) carry URL announcements and instance announcements.
- Sync protocol: paginated pull with `since`/`until` range, tag/click/bot filters.
- Instance discovery via `/.well-known/caslink` endpoint.
- Keys managed per instance; signatures verified on receipt.
- Federation is disabled by default.

### Notifications

- Email: SMTP (gomail), SendGrid, Amazon SES.
- SMS provider (pluggable).
- Push notifications provider (pluggable).
- Webhook delivery with queue and retry.
- Notification templates stored in DB; customizable by operator.
- Events: registration, password reset, 2FA changes, link expiration warnings, billing events.

### Scheduler

- Built-in cron scheduler (robfig/cron); no host cron or systemd dependency.
- Scheduled tasks: SSL renewal check, GeoIP database update, expired link cleanup, analytics aggregation, dunning retries, audit log compaction.

### Database

- SQLite (default, single-node), PostgreSQL, MySQL, SQL Server.
- Schema applied at startup via numbered Go migration files (001–006); idempotent, no separate migration runner binary required.
- All queries parameterized; no `SELECT *`; connection pool limits enforced.
- Transactions with `defer tx.Rollback()` pattern.
- Passwords, tokens, and secrets never stored raw; SHA-256 or Argon2id always.

### Security model

- Fail-closed: unauthenticated requests see only public links and the auth flow.
- CSRF protection on all state-mutating web routes.
- XSS prevention via template escaping.
- SQL injection prevented via parameterized queries.
- Path traversal guards on any file-serving route.
- Audit log is append-only; logs never contain raw credentials.
- Enumeration mitigation: identical error messages and constant-time comparison for "wrong password" vs "no such user".
- GeoIP used as a risk signal only, never as a sole access gate.

### Configuration hierarchy

1. Defaults (hardcoded sane values — zero-config startup works).
2. Config file (`{config_dir}/server.yml`).
3. Environment variables (`CASLINK_*` prefix).
4. CLI flags (`--port`, `--data`, `--config`, `--mode`, `--debug`).
5. Admin panel (runtime overrides stored in DB).

### Roadmap

- Mobile apps (iOS, Android)
- Browser extensions
- Link-in-bio pages
- Advanced link rotation
- A/B testing for links
- Link expiration notifications (email/push)
- Webhook events for link clicks
