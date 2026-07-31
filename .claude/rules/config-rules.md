# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER use `strconv.ParseBool()` directly — always `config.ParseBool()` / `config.IsTruthy()` (accepts yes/no, enable/disable, oui/non, etc.)
- NEVER put YAML comments inline — always on the line ABOVE the setting (exception: GitHub Actions SHA-pin `# vX.Y.Z` annotations)
- NEVER skip path normalization/validation on config values, HTTP paths, file paths, or API path parameters
- NEVER trust `X-Forwarded-*` headers from a peer not in `trusted_proxies` (private ranges + configured `additional` list)
- NEVER let Tor (`.onion`) requests fall through to clearnet FQDN/email/URLs — no clearnet leakage anywhere in Tor responses
- NEVER fail startup on invalid config — warn and substitute the default instead
- NEVER bypass admin authentication because debug mode is on — debug affects verbosity/diagnostics only, never security checks, in any mode including production
- NEVER expose `/debug/*` or pprof endpoints unless `--debug`/`DEBUG=true` is explicitly set (404 otherwise)
- NEVER expose `server.contact.admin.email` publicly — internal-only, universal fallback
- NEVER let `abuse@{fqdn}` auto-populate — operator must opt in explicitly (RFC 2142 recommends it, but an unprovisioned mailbox bounces)
- NEVER cache the effective contact-role recipient across requests — resolve per dispatch
- NEVER rewrite `r.RemoteAddr` before trust-checking the original TCP peer for `isTrustedPeer()` / proxy header gates
- NEVER run cluster/mixed mode without Valkey/Redis cache — `memory` cache only works for single instance
- NEVER introduce flat aliases/duplicate names for the canonical contact keys (`admin`, `security`, `abuse`, `general`)

## CRITICAL - ALWAYS DO

- ALWAYS run `PathSecurityMiddleware` early — after URL normalize + request-ID, before auth/rate-limit/routing (position 3 of 10)
- ALWAYS use `server.yml` (not `.yaml`) — auto-migrate `server.yaml` → `server.yml` on startup
- ALWAYS treat `server.yml` as source of truth in Single Instance (SQLite) mode; database as source of truth in Cluster mode (server.yml becomes cache+backup)
- ALWAYS sync database config changes to local `server.yml` cache: on change, on startup, and every 5 minutes
- ALWAYS enter maintenance mode (read-only + 503 on writes) on the only two truly critical errors: DB connection failure, or cannot write files (disk full/permissions)
- ALWAYS attempt self-healing continuously in maintenance mode, retry every 30s, auto-recover when resolved
- ALWAYS persist the selected port (random or explicit) to `server.yml` on first run — never re-randomize on restart
- ALWAYS bind privileged ports (<1024) while still root, THEN drop privileges to the `{internal_name}` system user (Unix)
- ALWAYS check actual access AND authorization (not just file permission) for sensitive ops: `--maintenance setup`, `--maintenance restore`, `--maintenance mode`
- ALWAYS sign outbound webhooks with `X-Webhook-Signature` (HMAC-SHA256, per-webhook secret), `X-Webhook-Timestamp`, `X-Webhook-ID` (UUIDv7)
- ALWAYS fall back contact roles per the resolution chain: `security`→`admin`; `abuse`→`general`→`admin`; `general`→`admin`
- ALWAYS honor `MODE=debug`/`--mode debug` as alias for development+debug-on, but let an explicit `--debug`/`DEBUG` env var override it
- ALWAYS validate config on load; on invalid value, log a warning and substitute the default (never crash)

## Key Rules

### Path Security
- `SafePath()` = normalize (`path.Clean`, trim slashes, collapse `//`) + validate (reject `..`, reject uppercase/invalid chars, max 64 chars/segment, max 2048 total)
- `SafeFilePath(baseDir, userPath)` must verify the resolved absolute path stays within `baseDir`
- Middleware order (execute 1→10): URLNormalize → RequestID → PathSecurity → SecurityHeaders → Allowlist → Blocklist → RateLimit → GeoIP → Auth → Logging

### Config Storage
| Mode | Source of truth | server.yml role |
|---|---|---|
| Single Instance (SQLite) | server.yml | Primary |
| Cluster (remote DB) | Database | Bootstrap + cache/backup |

- DB unavailable → read-only mode using cached `server.yml` config; user accounts/sessions/API tokens unavailable
- Server tables: `srv_*` prefix (remote) / `server.db` (SQLite). User tables: `usr_*` prefix / `users.db`

### Maintenance Mode
- States: Normal, Maintenance, Starting
- Write ops → `503` with canonical error body (`ok:false, error:"MAINTENANCE"`), `Retry-After` + `X-Maintenance-*` headers (never body fields)
- `/server/healthz` reports `status: maintenance` with self-healing attempt counters
- Auto-cleanup: disk >90% triggers log/temp/backup cleanup (keep last 5 backups, 7-day log retention by default)

### Boolean Parsing
- Use `config.ParseBool(s, default)` / `config.MustParseBool()` / `config.IsTruthy()` / `config.IsFalsy()` everywhere: env vars, YAML, CLI flags, API params, form inputs
- Truthy: `1 y t yes true on ok enable enabled yep yup yeah aye si oui da hai affirmative accept allow grant sure totally`
- Falsy: `0 n f no false off disable disabled nope nah nay nein non niet iie lie negative reject block revoke deny never noway`
- Case-insensitive; empty string → default; invalid value → error (not silent default)

### Environment Variables
- **Runtime (always checked):** `NO_COLOR`, `TERM`, `DOMAIN`, `MODE`, `DATABASE_DRIVER`, `DATABASE_URL`, `SMTP_*`
- **Init-only (first run only, then ignored):** `CONFIG_DIR`, `DATA_DIR`, `LOG_DIR`, `DATABASE_DIR`, `BACKUP_DIR`, `PORT`, `LISTEN`, `APPLICATION_NAME`, `APPLICATION_TAGLINE`
- URL var resolution priority: `{fqdn}`: proxy headers → `DOMAIN` → hostname → `$HOSTNAME` → global IP → localhost. `{proto}`: `X-Forwarded-Proto`/`Ssl`/`Url-Scheme` → TLS detect → http. `{baseurl}`: `X-Forwarded-Prefix`/`Path`/`Script-Name` → `server.baseurl` → `/`

### Config File
- Location: `/etc/{internal_org}/caslink/server.yml` (root) or `~/.config/{internal_org}/caslink/server.yml` (user)
- Design: clean, everything configurable, sane built-in defaults, comments single-line <140 chars, always above the setting

### Port Rules
- Default: random unused port 64000-64999, saved to `server.yml` on first run, persists across restarts
- Port `80` → enables Let's Encrypt HTTP-01; `443` → TLS-ALPN-01 + auto SSL; `0` → OS-assigned
- Dual port format: `"8090,8443"` (HTTP,HTTPS)
- Privileged (<1024) needs escalation on Unix; Windows uses Virtual Service Account (no escalation concept)
- Two run modes: Service (escalated, any port, drops privileges post-bind) vs User (`$USER`, >1024 only)
- Port display: strip `:80`/`:443`, show all other non-standard ports

### Service User / Privilege Drop (Unix)
- System user `{internal_name}` (caslink), group `caslink`, shell `/usr/sbin/nologin`, home `/var/lib/webappsgo/caslink`
- Sequence: root starts → create user/dirs → bind ports → drop to `caslink` user → init config/DB → serve
- Permanent-root only allowed for justified kernel/device-level needs, documented in IDEA.md
- Sensitive ops require AUTHORIZATION not just access: `setup` (first-run OR root OR valid token), `restore` (empty DB OR root OR admin creds), `mode change` (root OR admin creds)

### Application Modes (PART 6)
| State | Mode | Debug |
|---|---|---|
| Production (default) | production | false |
| Production+Debug | production | true |
| Development | development | false |
| Development+Debug | development | true |

- Priority — Mode: `--mode` flag > `MODE` env > default `production`. Debug: `--debug` flag > `DEBUG` env > `--mode debug` alias > default `false`
- `--mode debug` = development + debug on, but explicit `--debug`/`DEBUG` still wins (`MODE=debug DEBUG=false` → dev mode, debug off)
- Production: info logging, debug/pprof disabled, generic errors, caching on, rate limits enforced, all security headers on
- Development: debug logging, debug/pprof still disabled unless `--debug`, detailed errors, caching off (hot reload), relaxed rate limit/CORS
- `--debug`/`DEBUG=true`: enables `/debug/*`, `/debug/pprof/*`, `/debug/vars`, full request/DB/cache logging, memory/goroutine profiling — auth/security checks NEVER bypassed

### Server Configuration (PART 12)
- `baseurl`: default `/`, resolution: `X-Forwarded-Prefix` → `X-Forwarded-Path` → `X-Script-Name` → config/CLI `--baseurl` → `/`
- Request limits defaults: `max_body_size: 10MB`, `read/write_timeout: 30s`, `idle_timeout: 120s`
- Compression: enabled, level 5, targets html/css/js/json/xml
- Trusted proxies: private ranges + link-local always trusted; `additional:` for public CDN/LB IPs/CIDRs/DNS (refreshed every 5 min)
- Tor: priority-0 FQDN resolution when Host matches `tor.onion_address` — proto always `http://`, port always stripped, no proxy-header/IP check; email/URLs must use `tor.contact_email`/`tor.onion_address` only, never clearnet fallback; `Preferred-Languages` omitted from Tor `security.txt`
- Sessions: admin `max_age: 30d`/`idle_timeout: 24h`; user `max_age: 7d`/`idle_timeout: 24h`; `same_site: strict` default, `secure: auto`, `http_only: true`
- Rate limits (per IP, sliding window): read 120/min, write 10/min, health 120/min, global burst 240/min; auth: login 5/15min, password_reset 3/hr, registration 5/hr — `429` + `Retry-After` on limit
- Contact roles: `admin` (never public, required), `security` (public, default `security@{fqdn}`, PGP-encrypted body), `abuse` (opt-in only, no auto default), `general` (contact form) — each supports email + webhooks (telegram/discord/slack/mattermost/pushover/gotify/generic)
- Tracking platforms: google, matomo, piwik, owa, fathom, plausible, umami, simple, cloudflare — each with type-specific ID/URL validation
- Privacy: `data.sold` (default false) drives dynamic consent banner/page text; consent stored in `cookie_consent` cookie (never localStorage, server must read it); cookie categories: essential (always on), preferences, analytics
- Cache: `memory` (single instance default) vs `valkey`/`redis` (REQUIRED for cluster/mixed mode) — via `url` or `host/port/password`, pool_size 10, prefix `{project_name}:`

---
For complete details, see AI.md PART 5, 6, 12
