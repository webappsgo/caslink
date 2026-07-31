# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never use ad-hoc error response shapes — always the canonical `{"ok": bool, "data"/"error"+"message"}` envelope
- Never leak Tier 1 data (internal errors, stack traces, DB details, file paths, secrets) in any public/API response
- Never destructively modify schema — no `DROP`, no `DELETE` of existing columns/tables, no renames (add-only, 3-step deprecate pattern instead)
- Never use migration files or version tracking — schema is `CREATE TABLE IF NOT EXISTS` / idempotent `EnsureSchema` only
- Never let a cluster node run a different app version than its peers
- Never treat a cluster node and an agent as the same thing — agents never share the DB, never use cluster join tokens
- Never return, log, or back up secrets in plaintext exposure — `installation_secret`, `cookie_signing_key`, `csrf_token_secret`, `encryption_key` are never returned in API responses and never logged
- Never log passwords, full API/session tokens, recovery keys, TOTP secrets, private keys, credit card numbers, or full email addresses in audit logs — mask (first 8 chars only for tokens/IDs)
- Never implement database-at-rest encryption in-app — that's the OS's job (LUKS/FileVault/BitLocker); don't fake it
- Never honor `DNT` header by default (only `Sec-GPC` is honored as opt-out by default)
- Never show `Server-Timing` header outside debug mode
- Never let allowlisted IPs bypass authentication, API token validation, CSRF protection, path-traversal checks, or TLS — allowlist bypasses network-layer enforcement only
- Never weaken password policy below compliance-mandated minimums when a compliance standard is enabled (strictest wins)
- Never use default Tor ports (9050/9051) — always `ControlPort 127.0.0.1:auto` and `SocksPort auto`/`0`
- Never use system Tor — the server MUST start/own/stop its own dedicated Tor process
- Never fail server startup because Tor is missing or errors — Tor is fully optional, best-effort, non-blocking
- Never overwrite an existing `torrc` on startup — only regenerate it on an explicit admin config save
- Never expose the `.onion` address without admin authentication

## CRITICAL - ALWAYS DO

- Always use standard error codes (BAD_REQUEST, VALIDATION_FAILED, UNAUTHORIZED, TOKEN_EXPIRED/INVALID, 2FA_REQUIRED/INVALID, FORBIDDEN, ACCOUNT_LOCKED, NOT_FOUND, METHOD_NOT_ALLOWED, CONFLICT, RATE_LIMITED, SERVER_ERROR, MAINTENANCE) with correct HTTP status
- Always make schema changes idempotent and additive with sane defaults for new columns
- Always run retries with exponential backoff (0s,1s,2s,4s,8s, max 30s) only on retryable errors
- Always version-prefix cache keys (`v1:...`) and use TTL/event/tag invalidation appropriately
- Always use context-based query timeouts (SELECT 5s, complex/JOIN 15s, write 10s, bulk 60s, migration 5m, reports 2m)
- Always detect cluster mode automatically (external DB + cache present) — never require manual cluster toggling
- Always follow the cluster heartbeat contract: 30s interval, 90s degraded, 5min offline
- Always resolve split-brain via DB as source of truth + advisory lock + quorum for secret rotation
- Always classify public-endpoint data by tier (Tier 1 never exposed, Tier 2 always safe, Tier 3 debug-only) before returning it
- Always sanitize/validate at input, data-access, output, and transport layers (defense-in-depth) for injection/XSS/enumeration/timing/CSRF
- Always emit the full security header set (CSP, Permissions-Policy, X-Content-Type-Options, Referrer-Policy, COOP/COEP/CORP, HSTS when SSL on, Reporting-Endpoints/NEL) and tighten automatically per IDEA.md compliance declarations
- Always hash API tokens with SHA-256 before storage; never store raw
- Always write audit logs as append-only JSON Lines with required fields (id, time, event, category, severity, actor, result), 0640 perms, rotation-only removal
- Always apply strictest-wins conflict resolution across multiple enabled compliance standards
- Always both IP-block AND account-lock on repeated auth failures (they cover different attack shapes)
- Always let allowlist bypass only network-layer controls (IP block, rate limit, GeoIP, auto-block, account lockout) — never auth/CSRF/tokens/TLS
- Always auto-start the Tor hidden service if the Tor binary is found — no enable/disable flag
- Always create Tor directories/files with 0700/0600 perms owned by the app's running user, and enforce those perms even if the path already exists
- Always run the Tor child process as the same (post-privilege-drop) user as the server
- Always persist the ed25519 hidden-service key so `.onion` address survives restarts, unless explicitly regenerated
- Always validate all Tor config (client + server side) before saving and restart Tor only after successful validation

## Key Rules

### Error Handling (PART 9)
| Code | HTTP | Code | HTTP |
|---|---|---|---|
| BAD_REQUEST | 400 | FORBIDDEN | 403 |
| VALIDATION_FAILED | 400 | ACCOUNT_LOCKED | 403 |
| UNAUTHORIZED | 401 | NOT_FOUND | 404 |
| TOKEN_EXPIRED/INVALID | 401 | METHOD_NOT_ALLOWED | 405 |
| 2FA_REQUIRED/INVALID | 401 | CONFLICT | 409 |
| | | RATE_LIMITED | 429 |
| | | SERVER_ERROR | 500 |
| | | MAINTENANCE | 503 |

- Envelope: `{"ok":true,"data":...}` or `{"ok":false,"error":"CODE","message":"..."}` — canonical shape defined in PART 14
- `SendAPIResponseOK`/`SendAPIResponseError` Go helpers standardize this
- Retry backoff: 0s → 1s → 2s → 4s → 8s, capped at 30s; only for retryable errors (`isRetryable`)

### Caching (PART 9)
- Drivers: memory (default), Valkey, Redis
- Key patterns: `{type}:{id}`, `rate:{type}:{key}`, `lock:{resource}`, versioned `v1:...`

| Data | Default TTL |
|---|---|
| Session tokens | 24h |
| API tokens | no expiry |
| Rate limit counters | 1min |
| User profile | 5min |
| Config | 1min |
| Static asset hash | 24h |
| GeoIP | 7d |
| Blocklist | 1h |
| Page cache | 5min |
| API response cache | 30s |

- Invalidation: TTL, event-driven, version-prefix, tag-based
- Distributed locks via `SETNX` (`acquireLock`/`releaseLock`)

### Database & Cluster (PART 10)
- Schema: `CREATE TABLE IF NOT EXISTS`, idempotent `EnsureSchema`, no migration files, add-only changes with defaults
- Cross-DB: SQLite / PostgreSQL (`ADD COLUMN IF NOT EXISTS` 9.6+) / MySQL (handle error 1060 = column exists)
- Cluster = BASE functionality every project gets: config sync, session sharing, distributed locks, primary election, health monitoring
- Cluster auto-detected when external cache/DB present (requires PostgreSQL/MySQL + Valkey/Redis)
- Cluster node vs Agent: node = same binary, shares DB, joins via cluster join token; agent = separate `{project_name}-agent` binary, Bearer token (`adm_agt_`/`usr_agt_`/`org_agt_`), never shares DB

| Cluster Timing | Value |
|---|---|
| Heartbeat interval | 30s |
| Degraded threshold | 90s |
| Offline threshold | 5min |
| Node states | healthy / degraded / offline / removed |

- Primary election: lowest ID wins, no preemption on primary's return; DB is source of truth on split-brain
- Secret rotation: advisory lock + quorum majority to prevent split-brain
- Removed node: 403 `NODE_REMOVED` triggers local wipe of PGP keys / app_secrets cache
- Connection pool sizing: dev 5/2, small 25/5, medium 50/10, large 100/20; formula `max_open=(available_conns/num_nodes)*0.8`

| Query type | Timeout |
|---|---|
| Simple SELECT | 5s |
| Complex/JOIN | 15s |
| INSERT/UPDATE/DELETE | 10s |
| Bulk ops | 60s |
| Migrations | 5min |
| Reports | 2min |

- Transactions: basic, optimistic locking (version column), serializable isolation for reservations, retry on PostgreSQL 40001 / MySQL 1213

### Security (PART 11)
- Public endpoint tiers: Tier 1 (never shown — internals/secrets), Tier 2 (always public-safe), Tier 3 (debug-mode only)
- Defense-in-depth across input / data-access / output / transport layers for: SQLi, XSS, enumeration, timing oracles, credential stuffing, path traversal, token leakage, CSRF

**Key hierarchy:**
| Key | Size | Rotation |
|---|---|---|
| `installation_secret` | 32B | manual, 7-day grace |
| `cookie_signing_key` | 32B | 90 days auto, 7-day grace |
| `csrf_token_secret` | 32B | on admin pw change + 180 days |
| `server.security.encryption_key` (AES-256-GCM, server.yml) | — | versioned |

- Security headers: CSP, Permissions-Policy (locked-by-default features), X-Content-Type-Options, X-Frame-Options, Referrer-Policy, COOP/COEP/CORP, HSTS (SSL on), Reporting-Endpoints/Report-To/NEL, Clear-Site-Data on logout
- `Sec-GPC` honored by default; `DNT` not honored
- API token prefixes: `adm_`, `usr_`, `org_` (+ `_agt_` agent variants); SHA-256 hashed at rest only
- Well-known files: `robots.txt`, `security.txt` (RFC 9116) always on; `llms.txt` on; others opt-in via IDEA.md
- Coordinated disclosure: rotating `{security_id}` = HMAC-SHA256(installation_secret, floor(unix/172800)), 48h rotation window (current+previous)

**Logging:**
| File | Default format |
|---|---|
| access.log | apache |
| server.log | text |
| error.log | text |
| audit.log | json only |
| security.log | fail2ban |
| debug.log | text |

- Files = plain ASCII only (no emoji/ANSI); console = pretty, respects `NO_COLOR`/`TERM=dumb`
- Audit log: append-only JSONL, required fields id/time/event/category/severity/actor/result; never log passwords/full tokens/recovery keys/TOTP secrets/private keys/card numbers/full emails; mask to first 8 chars
- Severity levels: info/warn/error/critical

**Compliance (all disabled by default, strictest-wins):** GDPR, CCPA, HIPAA, SOC2, PCI-DSS, ISO27001, FedRAMP, LGPD, PIPEDA, APPI, PDPA — right-to-erasure vs retention resolves to anonymize, not delete

**Breach detection (auto-triggers):**
| Trigger | Threshold | Action |
|---|---|---|
| Brute force | 10/5min/IP | block 1h + alert |
| Credential stuffing | 50/10min cross-account | rate limit + alert |
| Mass export | 10/1h | queue + alert |
| Privilege escalation | any | block session + alert critical |
| API abuse | 10x normal | throttle + alert |

**IP blocking:** temporary (1h, auto-release) / extended (24h) / permanent (manual only); escalation 1st=1h, 2nd/24h=4h, 3rd/24h=24h, 4th+/7d=7d+alert

**Account lockout (separate from IP block):** 5 fails/15min=soft lock 15min; 10/1h=hard lock 1h; 15/24h=permanent (admin unlock or password reset)

**Allowlist:** bypasses IP blocklists, rate limiting, GeoIP blocking, auto-block, account lockout — never bypasses auth, tokens, CSRF, path security, or TLS

**Password policy defaults:** min_length 8 (12 under HIPAA/SOC2/PCI-DSS), complexity off by default (on under compliance), max_age 0 (90d under compliance), history 0 (12 under compliance)

**Encryption strategy:** passwords=Argon2id, API tokens=SHA-256, 2FA secrets=AES-256-GCM (server key), recovery keys=SHA-256, backups=AES-256-GCM (user password if set), DB-at-rest=OS responsibility (not implemented in-app), transit=TLS 1.2+ required

### Tor Hidden Service (PART 32)
- Always enabled if Tor binary found — no toggle; uses `github.com/cretz/bine` (CGO_ENABLED=0 compatible)
- Server binary owns full Tor process lifecycle (start/stop/restart/monitor); dedicated instance, never system Tor
- Hidden service created via `control.AddOnion()` (ED25519-V3 key), forwards `.onion:{virtual_port}` → `127.0.0.1:{server_port}` — not via torrc `HiddenServiceDir`
- `ControlPort 127.0.0.1:auto` on all OSes; `SocksPort auto` if outbound enabled else `SocksPort 0`; never default ports 9050/9051
- Outbound Tor network usage: server-wide `use_network` (default false) + `allow_user_preference` (default true) lets per-user override via `user.preferences.use_tor_network` (null=inherit, true/false=override)
- Directories always under app dirs: `{config_dir}/tor/` (torrc, 0700/0600), `{data_dir}/tor/` (0700), `{data_dir}/tor/site/` (keys, 0700/0600), `{log_dir}/tor.log`
- Tor runs as the same (privilege-dropped) user as the server; child process, dies with parent
- Tor issues are always INFO/WARN, never fatal — server must never fail to start due to Tor
- Vanity addresses: built-in generation up to 6 chars; 7+ chars via external tools (mkp224o) then import
- Config validated server-side before save; changes require Tor restart; regenerate/import both destructive (confirmation + audit log)
- Monthly bandwidth accounting via `AccountingStart`/`AccountingMax`; never an exit relay (`ExitRelay 0`, `ExitPolicy reject *:*`, `ORPort 0`, `DirPort 0`)

---
For complete details, see AI.md PART 9, 10, 11, 32
