## Project description

Caslink is a secure, mobile-first, fully self-hosted URL shortener written in Go that ships as a single static binary with zero external dependencies. It targets individuals and teams who want the control of self-hosting without the operational complexity of multi-service stacks. Any visitor can shorten links, track clicks, generate QR codes, and manage custom branded domains — all features ship to all users with no tier gating. Organizations layer collaborative ownership and RBAC on top of the individual model. A built-in billing module lets operators optionally monetize their instance, and a federation protocol lets instances discover and sync public links with one another.

## Project variables

project_name:     caslink
project_org:      casapps
internal_name:    caslink
app_name:         Caslink
official_site:    https://caslink.casapps.us
maintainer_name:  casjay
maintainer_email: git-admin@casjaysdev.pro
api_version:      v1

## Business logic

**Target users:**
- Individuals who want a self-hosted alternative to commercial link shorteners
- Small teams and organizations needing collaborative link management
- Operators wanting to run a monetized link-shortening service for others

**Features:**
- **URL shortening**: random alphanumeric short codes (default 6 chars) or custom slugs; configurable min length; reserved word blocking
- **Link options**: password protection, geo-restriction, device targeting, expiration date, public/private visibility, UTM passthrough
- **Click analytics**: per-click recording of timestamp, referrer, user-agent, device type, country and city via GeoIP; real-time dashboard per link; aggregate reports across owned links; bot exclusion configurable
- **QR codes**: on-demand generation for any short link in PNG, SVG, or PDF; customizable color, logo overlay, error correction level, and quiet zone
- **Bulk operations**: import links from CSV or JSON; export owned links to CSV or JSON
- **Multi-user accounts**: public registration (operator-configurable); Argon2id password hashing; TOTP 2FA (RFC 6238, QR setup, 10 single-use recovery codes); Passkeys/WebAuthn; "remember this device" (30-day cookie); OAuth2/OIDC social login (Google, GitHub, etc.)
- **Organizations**: up to 5 orgs per user (configurable); roles: owner, admin, member; org-scoped API tokens; ownership transfer; audit log for all permission and link changes
- **Custom domains**: up to 5 per user, 20 per org (configurable); DNS TXT verification with exponential-backoff retry; automatic SSL via Let's Encrypt (HTTP-01, DNS-01); apex and subdomain support
- **Admin panel**: server-wide configuration, user and org moderation, domain management, scheduler status, manual backup trigger, system health — see PART 17
- **Billing** (optional, operator opt-in): subscription plans with monthly/yearly pricing; Stripe, Paddle, PayPal, LemonSqueezy, or Manual provider; invoicing, dunning, usage tracking; disabled by default
- **Federation** (optional): peer instances exchange signed URL announcements; paginated pull sync with since/until range; discovery via `/.well-known/caslink`; disabled by default

**Data models:**
- Link: short_code, long_url, title, description, user_id, org_id, custom_code flag, password_hash (Argon2id), expires_at, public/private, tags, UTM fields
- Click: url_id, ip_hash (anonymized), country, city, user_agent, browser, OS, device, referrer, is_bot, clicked_at
- User: username (unique), email (unique, optional), password_hash (Argon2id), TOTP secret, passkey credentials, recovery keys, trusted devices, sessions
- Organization: name, slug, owner_id, members (with roles), org-scoped API tokens
- CustomDomain: domain, owner_type (user/org), owner_id, verification_token, ssl_cert (encrypted), verified_at

**Business rules:**
- Short codes are random alphanumeric; minimum length configurable (default 6); custom codes 3–50 chars
- Passwords hashed with Argon2id — see PART 11; never bcrypt
- API tokens follow `{prefix}_{32_alphanumeric}` format: `adm_` (admin), `usr_` (user), `org_` (org) — see PART 11
- Session tokens are cryptographically random opaque IDs stored as hashed values; admin sessions in server.db (`admin_sessions`), user sessions in users.db (`user_sessions`) — see PART 10
- Rate limiting: 5 login attempts per 15 min, 3 password-reset per 1 hour — see PART 11
- Enumeration mitigation: identical error messages and constant-time comparison for wrong-password vs no-such-user
- GeoIP used as risk signal only, never as sole access gate — see PART 20
- Audit log append-only; never contains raw credentials — see PART 11
- Billing is purely optional monetization; all features available to all users regardless of billing status
- Federation disabled by default; operators opt in via config

**Endpoints (WHAT, not paths — see PART 14 for route patterns):**
- Resolve/redirect a short code to its destination (301 or 302, configurable per link)
- Create a short link (authenticated users; anonymous creation configurable)
- Get link details and click statistics
- Update or delete a link
- Bulk import links from CSV or JSON
- Bulk export owned links
- Generate QR code for a link
- User registration, login, logout, password reset, 2FA setup/verify
- Manage user profile, settings, API tokens, sessions, security settings
- Create, manage, and leave organizations; invite members; transfer ownership
- Add and verify custom domains; manage SSL certificates
- Admin: user and org moderation, server config, scheduler, backup, system health
- Server info pages: about, help, privacy, terms, contact

**Data sources:**
- SQLite (default, single-node) or PostgreSQL/MySQL/SQL Server for cluster deployments — see PART 10
- GeoIP database (sapics/ip-location-db, no API key required) stored at `{data_dir}/security/geoip/` and updated on schedule — see PART 20
- IP/domain blocklists downloaded from configured sources and cached at `{data_dir}/security/blocklists/` — see PART 19
- CVE/security feeds cached at `{data_dir}/security/cve/` — see PART 19
