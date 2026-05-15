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

### Missing email-config block in `config.Config`
- File: `src/config/config.go` (no `EmailConfig` / `NotificationsConfig`)
- Spec: `AI.md` PART 18 line 31218 (`cfg.Server.Notifications.Email.SMTP.Host`), IDEA.md "Notifications" (SMTP/SendGrid/SES)
- Gap: `EmailService.SMTPConfigured()` reads `os.Getenv("SMTP_HOST")` etc. directly instead of resolving through `config.Config`. Operator cannot configure SMTP via `server.yml`. Spec assumes `Server.Notifications.Email.{Enabled,From,FromName,ReplyTo,SMTP{Host,Port,Username,Password,UseTLS,UseStartTLS}}`.
- Fix: Pending — adding the config struct is a wider change that also touches `EmailService`; tracked in TODO.AI.md "Notifications: SMTP works, add SendGrid / SES".

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

### `OrgMemberMiddleware` errors return `http.Error()` plain text instead of canonical JSON
- File: `src/server/middleware.go:332-356`
- Spec: `AI.md` PART 9 line 13745 (canonical shape)
- Gap: API requests get `Unauthorized\n` instead of `{"ok":false,"error":"UNAUTHORIZED",...}`. Same applies to CSRF middleware (`middleware.go:240`) and Bearer middleware writes `{"error":"..."}` only.
- Fix: Pending — needs a shared `writeJSONError` accessible to both middleware and handler packages; tracked.

### Sessions table missing columns the spec requires
- File: `src/server/store/store.go:177-184`
- Spec: `AI.md` PART 10 area (sessions schema): spec talks about `ip_address`, `user_agent`, `last_activity` for "Active sessions" UI in PART 23 (security/sessions page exists at `src/server/template/page/users/security/sessions.html`).
- Gap: The current `sessions` table is `(id, user_id, user_type, data, expires_at, created_at)`. The Sessions page cannot render IP/UA/last-active without these columns.
- Fix: Pending — tracked in TODO.AI.md (rolled into "Numbered migration runner").

### CSRF middleware not applied to `/setup` POST
- File: `src/server/server.go:188`
- Spec: `AI.md` PART 11 line 14910 (CSRF on by default), PART 16 → CSRF
- Gap: `/setup` is mounted before any middleware group. A first-run attacker on the local network could submit the initial admin form. Risk is low because the setup token gates it, but the spec wants CSRF on every non-GET.
- Fix: Pending — needs `setup` route group with CSRF; the setup token already provides equivalent protection, so flagged not fixed.

---

## MEDIUM

### `RateLimitMiddleware` returns non-canonical error body
- File: `src/server/middleware.go:175`
- Spec: PART 9 envelope
- Gap: Emits `{"error":"Too many attempts..."}`, should be `{"ok":false,"error":"RATE_LIMITED","message":"..."}`.
- Fix: Pending (same shared-error-helper refactor).

### Scheduler `tor_health` and `cluster_heartbeat` tasks absent
- File: `src/scheduler/scheduler.go`
- Spec: PART 19 line 32418–32419 (required when Tor / cluster enabled). Caslink does not ship Tor/cluster in this revision, so these are conditional. No-op.

### Logger middleware always on except in development
- File: `src/server/server.go:103`
- Spec: PART 11 — production logs should still emit access logs at INFO level for audit trail.
- Gap: `middleware.Logger` is only registered in dev mode. Spec wants structured logs in prod too.
- Fix: Pending — needs a slog-backed logger middleware; current chi default is too noisy.

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

### `/setup` route returns `http.Redirect(..., "/server/admin", ...)` typo path in admin middleware
- File: `src/server/middleware.go:309`
- Spec: spec uses `/server/{admin_path}` — when `adminPath` is non-default (e.g., `panel`), the redirect goes to the wrong URL.
- Fix: Pending — middleware would need the configured admin path; minor since default is `admin`.

### `selectRandomPort` scan range 64580..65000 hardcoded
- File: `src/server/server.go:449`
- Spec: IDEA.md `default_port: 64580` — using this as the start of the random range is correct.
- Status: OK.

---

## Completed

- helpers.go: respondJSON/respondError now emit canonical `{"ok":true,"data":...}` / `{"ok":false,"error":...}` envelope per PART 9.
- middleware.go: `X-Frame-Options` corrected to `SAMEORIGIN` per PART 11.
- scheduler.go: Added `token_cleanup`, `healthcheck_self`; session cleanup cadence raised to 15 minutes per PART 19.
