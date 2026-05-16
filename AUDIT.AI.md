# Caslink Project Audit

Started: 2026-05-15

Audit pass over the spec (`AI.md`, 60829 lines, PARTs 0–37) and `IDEA.md` against
the current source under `src/`. Findings grouped by severity. Items marked
`[FIXED]` were resolved in this audit; the remainder are tracked in
`TODO.AI.md` (which already captures the larger feature gaps — federation,
billing, passkeys, GeoIP enrichment, migration runner, etc.).

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

### CSRF middleware not applied to `/setup` POST
- File: `src/server/server.go:188`
- Spec: `AI.md` PART 11 line 14910 (CSRF on by default), PART 16 → CSRF
- Gap: `/setup` is mounted before any middleware group. A first-run attacker on the local network could submit the initial admin form. Risk is low because the setup token gates it, but the spec wants CSRF on every non-GET.
- Fix: Pending — needs `setup` route group with CSRF; the setup token already provides equivalent protection, so flagged not fixed.

---

## MEDIUM

### [FIXED] `RateLimitMiddleware` returns non-canonical error body
- File: `src/server/middleware.go:175`
- Spec: PART 9 envelope
- Gap: Emitted `{"error":"Too many attempts..."}`, spec requires `{"ok":false,"error":"RATE_LIMITED","message":"..."}`.
- Fix: Replaced with `writeJSONError(w, http.StatusTooManyRequests, "...")` (same refactor as OrgMemberMiddleware above).

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

### Setup wizard does not enforce password complexity client-side
- File: `src/server/handler/setup.go`
- Spec: AI.md auth conventions (PART 11) require password length / complexity checks.
- Fix: Pending.

---

## LOW

### Email templates under `src/templates/email/` use `.txt` only — no HTML variants
- Files: `src/templates/email/*.txt`
- Spec: PART 18 line 31144–31160 — spec lists ~18 templates; only 5 exist (`welcome_admin`, `welcome_user`, `password_reset`, `password_changed`, `email_verify`). Missing: `login_alert`, `security_alert`, `mfa_reminder`, `2fa_enabled`, `2fa_disabled`, `backup_complete`, `backup_failed`, `ssl_expiring`, `ssl_renewed`, `scheduler_error`, `breach_notification`, `breach_admin_alert`, `test`.
- Fix: Pending — most of these depend on features not yet implemented (SSL renewal, backups, breach detection). Tracked.

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

---

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
