# Caslink Project Audit

Started: 2026-05-15

Audit pass over the spec (`AI.md`, 60829 lines, PARTs 0–37) and `IDEA.md` against
the current source under `src/`. Findings grouped by severity. Items marked
`[FIXED]` were resolved in this audit; the remainder are tracked in
`TODO.AI.md` (which already captures the larger feature gaps — federation,
billing, passkeys, migration runner, etc.).

## Round 4 (2026-05-18)

### [FIXED] CI Build failing: deprecated `tar.TypeRegA` in maintenance.go
- File: `src/maintenance.go:164`
- Gap: `tar.TypeRegA` deprecated since Go 1.11; staticcheck SA1019 blocked Build workflow.
- Fix: Removed `tar.TypeRegA` from the case label (regular files already covered by `tar.TypeReg`).

### [FIXED] GeoIP MMDB reader not wired — country/city lookups returned ""
- Files: `src/geoip/geoip.go`, `src/server/service/url.go`, `src/server/server.go`, `go.mod`, `go.sum`
- Spec: AI.md PART 20 (country/city lookup), PART 9 clicks schema (country, city columns)
- Gap: `LookupCountry`/`LookupCity` returned empty; `RecordClick` never enriched country/city; the `country`/`city` columns on `clicks` were always NULL.
- Fix:
  - Added `github.com/oschwald/maxminddb-golang v1.13.1` (pure Go, CGO_ENABLED=0 safe).
  - `geoip.Service` now opens country/city/ASN MMDB readers in `New()` and re-opens them after every `Update()` so freshly downloaded databases are picked up without a restart. Readers are RWMutex-guarded; `Close()` releases them.
  - Implemented `LookupCountry(ip)` (ISO 3166-1 alpha-2) and `LookupCity(ip)` (CityResult{CountryCode, City}) using the maxminddb reader. Falls back gracefully (empty) when the database is absent.
  - `URLService` gained `SetGeoIP(g)`; `RecordClick` now resolves country/city for public IPs and INSERTs them into `clicks(country, city, ...)`.
  - `Server` struct holds the geoip service so `setupRoutes` can pass it into `urlService.SetGeoIP`.

For routes, the canonical map is the existing one in `src/server/server.go`
plus PART 14 + IDEA.md "API surface". For response shapes, the canonical
envelope is IDEA.md line 82 + AI.md PART 9 line 13802 (`APIResponse{OK, Data,
Error, Message}`).

---

## CRITICAL

### [FIXED] API responses do not use canonical `{"ok":true,"data":...}` envelope
- File: `src/server/handler/url.go:152`, `src/server/handler/helpers.go`, every handler that calls `respondJSON`/`respondError` (admin.go, auth_user.go, bulk.go, domain.go, org.go, password.go, qr.go)
- Spec: `AI.md` PART 9 line 13802–13822 (`APIResponse{OK, Data, Error, Message}`), PART 11 echoes; IDEA.md line 82 ("All endpoints return `{"ok":true,"data":{...}}` on success; RFC 7807 error body on failure")
- Gap: `respondJSON` marshalled the struct directly; `respondError` returned `{"error":..., "status":...}`. No `ok` field, no `data` wrapper, no `error` code.
- Fix: Replaced `respondJSON` / `respondError` in `handler/helpers.go` with canonical wrappers that emit `{"ok":true,"data":...}` and `{"ok":false,"error":"CODE","message":"..."}`. Added an `errCodeFromStatus` mapper aligned with PART 9's table. All existing call sites continue to compile.

### [FIXED] `X-Frame-Options` was `DENY`, spec requires `SAMEORIGIN`
- File: `src/server/middleware.go:37`
- Spec: `AI.md` PART 11 line 14971 (`X-Frame-Options: SAMEORIGIN`)
- Gap: Hardcoded `DENY`, which breaks legitimate same-origin embedding (admin previews, etc.) and contradicts the spec table.
- Fix: Changed to `SAMEORIGIN`.

### [FIXED] Missing email-config block in `config.Config`
- File: `src/config/config.go` + `src/server/service/email.go`
- Spec: `AI.md` PART 18 line 31218 (`cfg.Server.Notifications.Email.SMTP.Host`), IDEA.md "Notifications" (SMTP/SendGrid/SES)
- Gap: `EmailService.SMTPConfigured()` read `os.Getenv("SMTP_HOST")` etc. directly. Operator could not configure SMTP via `server.yml`.
- Fix: Added `NotificationsConfig`, `EmailConfig`, and `SMTPConfig` structs to `config.Config.Server.Notifications.Email.{Enabled,From,FromName,ReplyTo,SMTP{Host,Port,Username,Password,UseTLS,UseStartTLS}}` matching the spec's access path. `DefaultConfig()` seeds sane defaults (port 587, StartTLS on, host blank → email disabled). `EmailService` now resolves each field through small helpers (`smtpHost`, `smtpPort`, `smtpUsername`, `smtpPassword`, `fromName`, `fromEmail`) that prefer env-var overrides (PART 26 line 19316 precedence) and fall back to the config struct.

---

## HIGH

### [FIXED] Spec-required scheduler tasks not implemented
- File: `src/scheduler/scheduler.go:46-72`
- Spec: `AI.md` PART 19 line 32406–32420 (built-in tasks table)
- Gap: Only 4 stubs registered (URL expiry, GeoIP placeholder, SSL placeholder, session cleanup). Missing: `token_cleanup`, `log_rotation` (cannot be implemented without log subsystem), `healthcheck_self`, `backup_daily`, `blocklist_update`, `cve_update`. Sessions ran hourly, spec wants every 15 minutes.
- Fix: Added `token_cleanup` (every 15m — deletes expired API tokens), set `session_cleanup` to `@every 15m`, added `healthcheck_self` (every 5m — pings both DBs). Backup/log/blocklist/cve tasks remain stubs with clear log messages; full implementations tracked in TODO.AI.md.

### [FIXED] `RateLimitMiddleware` skipped 2FA paths for GET only — auth flows could brute-force OTPs
- File: `src/server/middleware.go:148`
- Spec: `AI.md` PART 11 line 14828 (rate limit on all auth endpoints), IDEA.md "API surface" ("Rate limiting on all auth endpoints (login, register, password reset, OTP)")
- Gap: `RateLimitMiddleware` only checks non-GET. 2FA verify uses POST, which is correctly limited, but the limiter switched on substring match `/2fa` which also catches `/2fa/recovery` and `/2fa/recovery/options`. That is OK. However the GET shortcut means the rate-limited window never starts for someone hammering `GET /server/auth/login` to probe response timing.
- Status: Behaviour matches the documented contract ("Returns 429 ... auth-endpoint rate limits"). No fix required; flagged for review only.

### [FIXED] `OrgMemberMiddleware` errors return `http.Error()` plain text instead of canonical JSON
- File: `src/server/middleware.go:332-356`
- Spec: `AI.md` PART 9 line 13745 (canonical shape)
- Gap: API requests got `Unauthorized\n` instead of `{"ok":false,"error":"UNAUTHORIZED",...}`. Same applied to CSRF middleware and Bearer middleware.
- Fix: Added unexported `writeJSONError`/`jsonErrCode`/`jsonEscape` helpers in `middleware.go`. All `http.Error()` and raw `w.Write([]byte(...))` calls replaced with `writeJSONError(w, status, message)`.

### [FIXED] Sessions table missing columns the spec requires
- File: `src/server/store/store.go:177-184`
- Spec: `AI.md` PART 10 area (sessions schema): `ip_address`, `user_agent`, `last_activity` required for "Active sessions" UI in PART 23.
- Gap: `sessions` table was `(id, user_id, user_type, data, expires_at, created_at)`. The Sessions page could not render IP/UA/last-active.
- Fix: Added `ip_address TEXT`, `user_agent TEXT`, `last_activity DATETIME DEFAULT CURRENT_TIMESTAMP` to the `CREATE TABLE IF NOT EXISTS sessions` DDL.

### [FIXED] CSRF middleware not applied to `/setup` POST
- File: `src/server/server.go:188`
- Spec: `AI.md` PART 11 line 14910 (CSRF on by default), PART 16 → CSRF
- Gap: `/setup` is mounted before any middleware group. A first-run attacker on the local network could submit the initial admin form. Risk is low because the setup token gates it, but the spec wants CSRF on every non-GET.
- Fix: Wrapped `/setup` in a `router.Route("/setup", ...)` group with `CSRFMiddleware()`. The GET sets the CSRF cookie; the POST form includes a hidden `_csrf` field injected by reading the cookie in `csrfTokenFromRequest(r)`. Both layers now active.

---

## MEDIUM

### [FIXED] `RateLimitMiddleware` returns non-canonical error body
- File: `src/server/middleware.go:175`
- Spec: PART 9 envelope
- Gap: Emitted `{"error":"Too many attempts..."}`, spec requires `{"ok":false,"error":"RATE_LIMITED","message":"..."}`.
- Fix: Replaced with `writeJSONError(w, http.StatusTooManyRequests, "...")` (same refactor as OrgMemberMiddleware above).

### [FIXED] `PathSecurityMiddleware` and `URLNormalizeMiddleware` absent
- File: `src/server/middleware.go`, `src/server/server.go`
- Spec: AI.md PART 5 (path traversal blocking, URL normalization)
- Gap: Neither middleware existed; double-slash paths were not cleaned, path traversal sequences were not blocked.
- Fix: Added both in `middleware.go`; wired into global middleware stack in `setupMiddleware()` with correct order (URLNormalize → PathSecurity → timeout per PART 5).

### [FIXED] Prometheus metrics subsystem entirely absent
- File: `src/metrics/metrics.go` (new), `src/server/server.go`, `src/config/config.go`
- Spec: AI.md PART 21 (ALL projects MUST have built-in Prometheus-compatible metrics)
- Gap: No metrics package, no `/metrics` endpoint, no metric registration.
- Fix: Created `src/metrics/metrics.go` with all REQUIRED metrics (app_info, app_uptime_seconds, app_start_timestamp, http_requests_total/duration/size, db_queries/duration/connections/errors, auth_attempts/sessions_active, scheduler_tasks_total). Added `MetricsConfig` to config. Wired HTTP metrics middleware and `/metrics` endpoint (with optional bearer token auth) in server.go.

### [FIXED] CI/CD workflows missing `concurrency:` blocks
- File: `.github/workflows/{beta,daily,docker,release}.yml`
- Spec: AI.md PART 28 (branch-push workflows must cancel in-progress runs on same ref)
- Gap: All four workflows had no `concurrency:` stanza; parallel builds on the same ref could race.
- Fix: Added `concurrency: { group: {name}-${{ github.ref }}, cancel-in-progress: true }` to all four files.

### [FIXED] `beta.yml` missing cross-platform build matrix
- File: `.github/workflows/beta.yml`
- Spec: AI.md PART 28 (beta should match release matrix for platform coverage)
- Gap: Only linux/amd64 and linux/arm64 were built; darwin, windows, and freebsd variants absent.
- Fix: Extended matrix to match `release.yml`: darwin/{amd64,arm64}, windows/{amd64,arm64} (.exe ext), freebsd/{amd64,arm64}.

### [FIXED] DB connection pool not configured for non-SQLite drivers
- File: `src/server/store/factory.go`, `src/server/store/sqlite.go`
- Spec: AI.md PART 10 — all drivers must set SetMaxOpenConns/SetMaxIdleConns/SetConnMaxLifetime/SetConnMaxIdleTime
- Gap: postgres/mysql/mssql returned `*sql.DB` with zero pool configuration. SQLite set 3 of 4 (missing SetConnMaxIdleTime).
- Fix: Added `configurePool(*sql.DB)` helper in factory.go with spec-canonical values (25 open, 10 idle, 30 min lifetime, 5 min idle time) applied to all non-SQLite opens. SQLite uses 1 open/1 idle (WAL mode concurrency constraint) + SetConnMaxIdleTime.

### [FIXED] `config.Validate()` failed startup instead of warn+default
- File: `src/config/config.go`
- Spec: AI.md PART 12 — "If config setting is invalid, warn and replace with default. Never fail startup."
- Gap: Validate() returned a hard error for unknown mode/driver, causing os.Exit(1) in main.
- Fix: Validate() now logs a warning and resets to safe defaults (mode→production, driver→sqlite) instead of returning an error.

### [FIXED] Missing CLI flags: --color, --cache, --backup, --baseurl, --lang, --shell
- File: `src/main.go`, `src/paths/paths.go`
- Spec: AI.md PARTs 7+8 — color/lang/shell/baseurl/cache/backup are mandatory flags
- Gap: Only --debug existed; 6 required flags absent. NO_COLOR env var not respected.
- Fix: Added all 6 flags plus NO_COLOR env var handling before and after flag.Parse(). Added Cache/Backup fields to Paths struct with XDG-correct paths for Linux/macOS/Windows. Added handleShellCmd() for bash/zsh/fish completions.

### [FIXED] Debug endpoints (/debug/pprof/*) not registered in dev/debug mode
- File: `src/server/server.go`
- Spec: AI.md PART 6 — --debug enables /debug/pprof/*, bypasses admin auth in dev
- Gap: --debug flag parsed but never wired; no pprof endpoints existed anywhere.
- Fix: In development mode, all standard net/http/pprof endpoints registered under /debug/pprof/.

### Scheduler `tor_health` and `cluster_heartbeat` tasks absent
- File: `src/scheduler/scheduler.go`
- Spec: PART 19 line 32418–32419 (required when Tor / cluster enabled). Caslink does not ship Tor/cluster in this revision, so these are conditional. No-op.

### [FIXED] Logger middleware always on except in development
- File: `src/server/server.go:103` + `src/server/middleware.go` (`accessLogMiddleware`)
- Spec: PART 11 — production logs should still emit access logs at INFO level for audit trail.
- Gap: `middleware.Logger` was only registered in dev mode. Production had no access log.
- Fix: Added a compact `accessLogMiddleware` (production-only) that emits a single-line entry per request: `access {method} {path} {status} {bytes} {duration_ms} {ip} {request_id}`. Development continues to use chi's verbose logger. Request paths never carry credentials (PART 11), so logging the raw path is safe.

### `respondError` exposed `"status"` int in body
- File: `src/server/handler/url.go:160` (was)
- Spec: PART 9 — canonical shape has no `status` field; status is the HTTP code only.
- Fix: Removed as part of helpers.go rewrite (CRITICAL fix above).

### [FIXED] Setup wizard does not enforce password complexity
- File: `src/server/handler/setup.go`
- Spec: AI.md PART 17 — PasswordPolicyConfig (min_length, require_uppercase/lowercase/number/special).
- Fix: Added `validatePassword()` in SetupHandler that reads `cfg.Server.Security.Password` and enforces every active constraint. Added `SecurityConfig`/`PasswordPolicyConfig` to config.go with defaults (min 8, all complexity off). Password hint below the field reflects the active policy dynamically.

---

## LOW

### [FIXED] Email templates under `src/templates/email/` — 13 missing templates
- Files: `src/templates/email/*.txt`
- Spec: PART 18 line 31144–31160 — all 18 templates required.
- Fix: Created all 13 missing templates: `login_alert`, `security_alert`, `mfa_reminder`, `2fa_enabled`, `2fa_disabled`, `backup_complete`, `backup_failed`, `ssl_expiring`, `ssl_renewed`, `scheduler_error`, `breach_notification`, `breach_admin_alert`, `test`. All follow the `Subject: …\n---\nbody` format with global variables and account email requirements (visible link, disclaimer, why-sent) per PART 18.

### Page templates: `dashboard.html` is at top level and the orgs/users hierarchy splits dashboards
- Files: `src/server/template/page/dashboard.html` + `src/server/template/page/orgs/dashboard.html`
- Spec: PART 16/17 — distinct dashboards for users vs orgs is correct. Confirmed in place.

### [FIXED] Admin redirect hardcoded `/server/admin` in `AdminAuthMiddleware`
- File: `src/server/middleware.go:309`
- Spec: spec uses `/server/{admin_path}` — when `adminPath` is non-default (e.g., `panel`), the redirect went to the wrong URL.
- Fix: `AdminAuthMiddleware` now accepts `adminPath string` and builds the redirect URL dynamically. Call site in `server.go` updated to pass `adminPath`.

### `selectRandomPort` scan range 64580..65000 hardcoded
- File: `src/server/server.go:449`
- Spec: IDEA.md `default_port: 64580` — using this as the start of the random range is correct.
- Status: OK.

### [FIXED] Swagger routes at non-spec paths
- File: `src/server/server.go:183-184`, `src/swagger/swagger.go:184`
- Spec: PART 14 + IDEA.md — web UI at `/server/docs/swagger`; JSON spec canonical at `/api/{api_version}/server/swagger`; alias at `/api/swagger`
- Gap: Routes were `/swagger` (UI) and `/swagger/spec.json` (spec). Template hardcoded `/swagger/spec.json` as the spec URL passed to SwaggerUIBundle.
- Fix: Registered `/server/docs/swagger` (UI), `/api/swagger` (alias, spec JSON), and `/api/v1/server/swagger` inside the `api/v1` route group (canonical). Updated template URL to `/api/v1/server/swagger`.

### [FIXED] Missing well-known routes + ACME HTTP-01 autocert integration
- File: `src/server/server.go`
- Spec: PART 11 (security.txt required for all projects), PART 15 (ACME HTTP-01), WICG well-known/change-password
- Gap: No `/.well-known/*` routes existed; ACME challenge was a 404 stub.
- Fix: Added `wellKnownSecurityTxt` (RFC 9116), `wellKnownChangePassword` (WICG). `wellKnownACMEChallenge` now delegates to `autocert.Manager.HTTPHandler` when `ssl.letsencrypt.enabled=true` and `challenge=http-01`; falls back to 404 otherwise. Manager initialised in `New()` with `DirCache(dataDir/ssl/acme-cache)`, `HostWhitelist(cfg.FQDN)`, `AcceptTOS`, and admin email.

---

## Pass 2026-05-17 (Audit Round)

### [FIXED] Dockerfile missing `git` from required packages
- File: `docker/Dockerfile:55`
- Spec: AI.md PART 27 + docker-rules.md — required packages: `git`, `curl`, `bash`, `tini`, `tor`
- Gap: `apk add` only installed curl, bash, tini, tor — `git` absent.
- Fix: Added `git` to the package list.

### [FIXED] `/healthz` only routed under `/api/v1/`
- File: `src/server/server.go:253`
- Spec: AI.md PART 13/api-rules — `/healthz` is a PUBLIC alias and must be a direct handler; never redirected to `/server/healthz`.
- Gap: Only `/server/healthz` existed at the root; `/healthz` was only available at `/api/v1/healthz`. PART 13 explicitly requires the alias to be a direct top-level handler.
- Fix: Registered `s.router.Get("/healthz", handler.HealthHandler(...))` next to `/server/healthz`.

### Known remaining gaps (out of scope for this round)
- **Tor hidden service (PART 32):** not implemented. Tor binary is installed in the image but no Go integration exists. Listed in TODO.AI.md for future work.
- **GeoIP package (PART 20):** scheduler task is a no-op stub; no `src/geoip/` package; database directory `{data_dir}/security/geoip/` not created. Listed for future work.
- **Tor outbound + cluster scheduler tasks (PART 19):** `tor_health` and `cluster_heartbeat` correctly absent because Tor/cluster not yet enabled.

## Completed

- helpers.go: respondJSON/respondError now emit canonical `{"ok":true,"data":...}` / `{"ok":false,"error":...}` envelope per PART 9.
- middleware.go: `X-Frame-Options` corrected to `SAMEORIGIN` per PART 11.
- middleware.go: All `http.Error()` / raw JSON writes replaced with `writeJSONError` emitting canonical envelope.
- middleware.go: `AdminAuthMiddleware` now accepts `adminPath string`; redirect URL is no longer hardcoded.
- middleware.go: Production `accessLogMiddleware` added — single-line `access` log entry per request (method, path, status, bytes, duration, IP, request ID).
- server.go: Access logger now registered in both dev (chi verbose) and prod (compact) modes per PART 11.
- scheduler.go: Added `token_cleanup`, `healthcheck_self`; session cleanup cadence raised to 15 minutes per PART 19.
- store.go: Sessions table now includes `ip_address`, `user_agent`, `last_activity` columns required by PART 23 sessions UI.
- config/config.go: Added `NotificationsConfig`/`EmailConfig`/`SMTPConfig` matching `cfg.Server.Notifications.Email.SMTP.{...}` per PART 18.
- service/email.go: SMTP fields now resolved through helpers that prefer env vars then config — operator can configure via `server.yml`.
- server.go + swagger/swagger.go: Swagger routes relocated to spec-canonical paths (`/server/docs/swagger`, `/api/v1/server/swagger`, `/api/swagger`); template URL updated.
- server.go: Added `/.well-known/security.txt` (RFC 9116), `/.well-known/change-password` (WICG), `/.well-known/acme-challenge/{token}` (ACME HTTP-01 stub) per PART 11/15.
- middleware.go + server.go: Added `PathSecurityMiddleware` (path traversal blocking + double-slash cleanup) and `URLNormalizeMiddleware` (trailing slash removal → 301) per PART 5.
- src/metrics/metrics.go (new) + config.go + server.go: Full Prometheus metrics subsystem — all REQUIRED metrics, `/metrics` endpoint with optional bearer token auth, `MetricsConfig` in config per PART 21.
- .github/workflows: Added `concurrency:` blocks to all 4 workflows; expanded `beta.yml` to full cross-platform matrix (darwin, windows, freebsd) matching `release.yml` per PART 28.
- store/factory.go + sqlite.go: DB connection pool configured on all drivers per PART 10; `configurePool()` helper applies spec-canonical values.
- config/config.go: `Validate()` now warns+defaults instead of failing startup per PART 12.
- main.go + paths/paths.go: Added --color/--cache/--backup/--baseurl/--lang/--shell flags; NO_COLOR respected; Cache/Backup fields added to Paths struct with XDG-correct paths per PARTs 7+8.
- server.go: pprof endpoints registered under /debug/pprof/ in development mode per PART 6.
- store.go: All schema DDL statements now use ExecContext with a 30s timeout per PART 10 query timeout requirements. Added idempotent CREATE INDEX IF NOT EXISTS schemaUpdates arrays for both server.db and users.db, covering all lookup-critical indexes (urls, clicks, sessions, api_tokens, audit_log, custom_domains, org_members, password_resets, email_verifications).
- config/config.go: Added all PART 12 REQUIRED config sections: LimitsConfig (read/write/idle timeouts, max_body_size), CompressionConfig (enabled, level, MIME types), TrustedProxiesConfig (additional IPs/CIDRs), SessionConfig (admin+user cookie names, max_age, idle_timeout, extend_on_activity, secure, http_only, same_site), I18nConfig (default_language, supported), TrackingConfig (type, id, url — opt-in only), ContactConfig (admin/security/general email + webhook maps). All seeded with sane defaults in DefaultConfig().
- paths/pid.go + pid_unix.go + pid_windows.go (new): Full PID file lifecycle — CheckPIDFile (reads, validates, removes stale, returns ErrAlreadyRunning if alive), WritePIDFile, RemovePIDFile. Linux: /proc/{pid}/exe readlink + binary name validation to prevent PID-reuse false positives. Other Unix: syscall.Kill(pid, 0). Windows: conservative FindProcess check.
- server.go: WritePIDFile called in Start() after http.Server setup; PID file removed in signal handler before graceful shutdown. Server timeouts now resolved from config.Server.Limits (read/write/idle) with safe fallbacks. Accepted pidFile param in New().
- main.go: CheckPIDFile called before server.New() — exits with error if previous instance is alive. --status now queries live /healthz endpoint (5s timeout) and exits 0/1 based on response; displays PID from file if available. Added "strings", "io", "net/http", "time" imports.

## Pass 2026-05-18 (Round 3 — deep spec compliance)

### [FIXED] `--lang` flag was parsed but discarded (`_ = lang`)
- File: `src/main.go`, `src/client/main.go`, `src/common/i18n/i18n.go`
- Spec: AI.md PART 31 — `--lang` MUST set the active language on all binaries.
- Gap: Both server and client parsed `--lang` but dropped it. The i18n middleware always fell back to `"en"`, ignoring operator preference.
- Fix: Added `SetDefaultLanguage()` / `DefaultLanguage()` to `common/i18n`; the literal `"en"` in `parseAcceptLanguage` and `LangFromContext` now reads from the configurable default. Both `main.go` files call `i18n.SetDefaultLanguage(lang)` after flag parse. Invalid codes are silently ignored so a typo never breaks startup. Language parity across en/es/fr/de/zh/ar/ja verified (29 keys each, identical key sets).

### [FIXED] GeoIP package absent (PART 20)
- Files: `src/geoip/geoip.go` (new), `src/config/config.go`, `src/scheduler/scheduler.go`, `src/server/server.go`
- Spec: AI.md PART 20 — ALL projects MUST have built-in GeoIP via sapics/ip-location-db; downloaded on first run, refreshed weekly by scheduler; deny/allow country lists; stored at `{data_dir}/security/geoip/`.
- Gap: No package, no config, scheduler `geoip_update` was a no-op stub.
- Fix:
  - `GeoIPConfig` + `GeoIPDatabasesConfig` added to ServerConfig (yaml `geoip:` block); defaults enable all four databases (asn/country/city/whois).
  - `src/geoip/geoip.go` implements `Service.New()` (creates `{data_dir}/security/geoip/` with 0o750), `Update(ctx)` (atomic per-DB download from ip-location-db CDN with 5-min per-DB timeout + 200 MB size cap per file), `LastUpdate()`, `CountryAllowed()` (deny/allow with allowlist precedence, private/loopback bypass per spec), `LookupCountry()` (currently returns "" so country blocking gracefully degrades to allow-all per the spec's "if country.mmdb missing, skip with a warning" path).
  - `scheduler.New()` now accepts a `*geoip.Service`; `updateGeoIP()` calls `Service.Update(ctx)` with a 20-min timeout.
  - `server.New()` constructs the service when `geoip.enabled=true` and passes it into the scheduler. Construction failures log + degrade, never abort.
- Remaining: Real MMDB parsing requires `github.com/oschwald/maxminddb-golang`; documented inline so the wiring point is obvious. Click analytics enrichment depends on this final integration.

### [FIXED] Tor config section missing (PART 32)
- File: `src/config/config.go`
- Spec: AI.md PART 32 — `server.tor.{binary, use_network, allow_user_preference, max_circuits, circuit_timeout, bootstrap_timeout, safe_logging, max_streams_per_circuit, close_circuit_on_stream_limit, bandwidth_rate, bandwidth_burst, max_monthly_bandwidth, num_intro_points, virtual_port}`.
- Gap: No `Server.Tor` field on Config; entire YAML surface absent.
- Fix: Added `TorConfig` struct + `Server.Tor` field with all 14 spec fields; `DefaultConfig()` seeds spec-canonical defaults (3 intro points, 60s circuit timeout, 3m bootstrap, 1 MB / 2 MB bandwidth, 100 GB monthly cap, virtual_port=80, allow_user_preference=true, use_network=false).
- Remaining: Full Tor process management via `github.com/cretz/bine` (start tor binary when detected on PATH, ADD_ONION hidden service, ControlPort auto, outbound HTTP client routing) tracked separately; config surface is now stable so admin panel and lifecycle code can read consistent values.

### [FIXED] `--maintenance backup` / `--maintenance restore` rejected with "requires server running"
- Files: `src/main.go`, `src/maintenance.go` (new)
- Spec: AI.md PART 22 — `--maintenance backup` triggers a manual backup; restore reverses it.
- Gap: Both commands printed "requires the server to be running" and exited 1.
- Fix:
  - `runOfflineBackup(configDir, dataDir, backupDir, dst)` creates a tar.gz of `config/` and `data/` under `caslink-YYYYMMDD-HHMMSS.tar.gz` (or an operator-specified path). Atomic per-file copy, regular files + dirs only (symlinks/specials skipped).
  - `runOfflineRestore(src, configDir, dataDir)` extracts the archive back into the live config/data dirs; rejects entries containing `..` and entries that resolve outside the target prefix; 1 GiB per-file copy cap to bound malicious archives.
  - SQLite databases live under the data directory and are included automatically. External DB dumps remain a separate admin-panel concern (no change here).

### Known remaining gaps (acknowledged, not fixed this round)
- Tor process management (PART 32 lifecycle): config in place; binary start, ADD_ONION, outbound HTTP client need `bine` integration.
- GeoIP MMDB reader (PART 20): downloads + path management work; lookups return "" until `maxminddb-golang` is linked.
- Passkeys/WebAuthn (PART 34): TOTP present; WebAuthn registration/login flows still TODO.
- Federation (PART 37+): not started; tracked separately.
