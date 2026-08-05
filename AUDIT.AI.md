# Project Audit — Extended PART 13-36 passes

Started: 2026-08-05

Five parallel investigators walked PART 13-15, 16-17, 18-23, 24-28+32, and 34-36.
Bounded security/logic/config findings are fixed directly (one commit each below).
Feature-sized gaps are deferred to TODO.AI.md with reasoning (never silently dropped).

## Fix now — security / logic

- [x] updater: installs unverified binary when release has no `.sha256` asset — refuse (PART 23) `src/updater/update.go` — FIXED
- [x] service: privilege drop runs BEFORE port bind — bind privileged port while root, then drop (PART 8/24) `src/main.go` + `src/server/server.go` — FIXED
- [x] scheduler: 7 non-skippable tasks (session/token cleanup, log_rotation, healthcheck_self, tor_health, cluster_heartbeat, ssl_renewal) are disableable via config — force enabled (PART 19) `src/scheduler/scheduler.go` — FIXED
- [ ] backup: no disk-space pre-check; `backup.skipped_disk_full` never emitted (PART 22) `src/backup/backup.go`
- [ ] tor: `Start()` overwrites existing torrc via updateTorrc — only create-if-absent on startup (PART 32) `src/tor/service.go`
- [ ] metrics: `/{code}` short-slug path label is unbounded cardinality — use chi RoutePattern (PART 21) `src/metrics/metrics.go`
- [ ] domains: AddDomain never enforces max_domains_per_user/org, reserved, or blocked_patterns though error vars already defined (PART 36) `src/.../domain.go`

## Fix now — config / docs

- [ ] docker: Dockerfile bakes LABEL blocks (must be CI metadata-action) (PART 27) `docker/Dockerfile`
- [ ] docker: Dockerfile sets `ENV MODE=development` on the production image (PART 27) `docker/Dockerfile`
- [ ] service: Linux escalation order missing `su` and `doas` (PART 24) `src/svcmgr/svcmgr_linux.go`
- [ ] health: `checks.cache` field missing from health response (PART 13) `src/server/handler/health.go`
- [ ] health: `/healthz` root alias registered unconditionally — gate on `server.healthz.root.enabled` (PART 13) `src/server/server.go` + config
- [ ] api: unversioned `/api/healthz` alias missing; non-spec `/api/v1/healthz` present instead (PART 14) `src/server/server.go`
- [ ] api: list default page size 50, spec says 250 (PART 14) `src/server/handler/helpers.go`
- [ ] admin: `/server/admin/help` is an illegal direct child — move under `config/` (PART 17) `src/server/server.go`
- [ ] css: desktop-first `max-width` breakpoint — invert to mobile-first (PART 16) `src/server/tmpl/static/css/app.css`

## Deferred to TODO.AI.md (feature-sized or needs unavailable verification)

- API version abstraction (`server.api_version` + `APIBasePath()`, drop hardcoded `v1`) — pervasive design change
- SSL 4-step cert lookup order (certbot/local cert discovery before autocert) — feature gap
- Service user UID/GID safe-range (200-899, UID==GID, reserved-ID skip) — needs root Linux host to verify
- Frontend CSP externalization (serve external css/js, move inline handlers to app.js, replace confirm/alert with <dialog>, restore `script-src 'self'`) — large, needs browser verification
- PART 36 full custom-domains (TXT ownership token, resolver middleware, API+admin routes, DNS-01, scheduled verify-retry/renew/cleanup, caching, rate limit, WebUI) — feature build
- PART 34 registration modes (open/invite/admin_only/disabled) + user invite flow — feature build
- PART 35 org creation mode + per-org visibility — feature build
- Admin panel WebUI templates + cookie consent banner — feature build
