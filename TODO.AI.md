# TODO.AI.md — Caslink Outstanding Work

Tracks remaining spec gaps found by a full PART-by-PART audit against AI.md
(PARTS 0–36). Items removed once fully implemented and committed. Ordered by
PART number (spec order). Priority tags: [CRIT] security, [HIGH] broken/dead
feature, [MED] missing subsystem, [LOW] polish.

Last full audit: 2026-07-30

---

## PART 5 — Configuration

- [MED] Dual-port support missing. Spec wants `port: "8090,8443"` (simultaneous
  HTTP+HTTPS); `Config.Server.Port` is an `int` with a single listener plus a
  hardcoded `:443` ALPN listener. — `src/config/config.go`, `src/server/server.go`
- [MED] Runtime maintenance-mode / self-healing state machine missing. Only
  offline CLI maintenance + an admin settings page exist. No runtime
  critical-error detection (DB/disk) entering read-only mode, no 503 +
  `Retry-After`/`X-Maintenance-*` responses, no self-healing retry/auto-recover,
  not surfaced in `/server/healthz`. — `src/server`, `handler/health.go`, `middleware.go`
- [LOW] `server.yaml` → `server.yml` startup auto-migration not implemented. — `src/config/config.go`
- [MED] Cluster-mode config sync missing. Config is file-only; no DB source-of-truth
  table, no DB→`server.yml` cache sync (immediate + 5-min), no read-only fallback
  when DB down. — `src/config`, `src/server/store` (cluster scope)

## PART 6 — Application Modes

- [MED] Debug is conflated with development mode. `--debug` just sets mode to
  "development"; `mode.go` has no debug state, so Production+Debug and
  Development+Debug don't exist and pprof is gated on `IsDevelopment()` instead of
  a debug flag. — `src/mode/mode.go`, `src/main.go`, `src/server/server.go`
- [MED] Custom debug endpoints missing. Only pprof registered; spec needs
  `/debug/vars`, `/debug/config`, `/debug/routes`, `/debug/cache`, `/debug/db`,
  `/debug/scheduler`. — new `src/server/debug.go`
- [LOW] First-run port not randomized. main.go hardcodes `Port=64580` when port==0
  instead of a random unused 64000–64999 port (`selectRandomPort()` bypassed on the
  config-save path). — `src/main.go`

## PART 9 / 12 — Caching & Compression

- [MED] No cache abstraction layer. No `src/cache/` package; `memory`/`valkey`/`redis`
  drivers, distributed locks (`SetNX`), cache warming absent (rate-limit/session use
  SQLite — OK single-node, breaks cluster). — new `src/cache/`
- [LOW] Response compression (gzip) middleware not implemented. — `src/server/middleware.go`
- [LOW] Privacy signal handling (DNT / GPC) not implemented. — `src/server/middleware.go`

## PART 10 — Database & Cluster

- [MED] Cluster mode is a static placeholder. No `nodes`/`cluster_locks`/
  `learned_origins` tables, no heartbeat, primary election, distributed locks,
  config sync, or secret distribution; admin "Cluster Nodes" hardcodes
  "Primary (standalone)"; scheduler has no primary-only guard (all nodes would run
  cron). — new `src/cluster/`, `src/scheduler/scheduler.go`

## PART 11 — Security & Logging

- [MED] `app_secrets` table + `installation_secret`/`cookie_signing_key`/
  `csrf_token_secret` rows not implemented (cluster secret sharing, HMAC keys). — `src/server/store/store.go`
- [MED] Coordinated-disclosure Security Reports pipeline not implemented: rotating
  `{security_id}` HMAC token, PGP keypair mgmt, encrypted report bodies,
  `/server/contact?security_id=` mode, `/.well-known/pgp-key.asc`. — new `src/security/`, `handler/pages.go`
- [LOW] `/.well-known/llms.txt` not served (security.txt + change-password are). — `src/server/server.go`

## PART 13 — Health & Versioning

- [LOW] `--version` output missing `Go: {go_version}` and `OS/Arch: {GOOS}/{GOARCH}`
  lines. — `src/main.go` `printVersion`
- [LOW] Health payload gaps: `ChecksInfo` lacks `cache`/`cluster`; `HealthResponse`
  lacks `PendingRestart`/`RestartReason`; `/healthz` root alias always mounted rather
  than gated on `server.healthz.root.enabled`. — `handler/health.go`

## PART 14 — API Structure

- [MED] Content negotiation / client-type detection entirely missing. Spec mandates
  `src/common/httputil/` with `HTML2TextConverter`, non-interactive/text-browser/
  http-tool/our-CLI detection, `.txt`-extension + `Accept: text/plain` output on all
  `/api/**`, smart text/HTML on all frontend routes. None exists. — new `src/common/httputil/` + handler/middleware wiring
- [HIGH] Incomplete API CRUD parity: no `GET /api/v1/urls`, no `PATCH`/`DELETE
  /api/v1/urls/{code}`, no `POST`/`DELETE /api/v1/users/tokens`, no `PATCH
  /api/v1/users`. — `handler/url.go`, `handler/user.go`

## PART 15 — SSL/TLS

- [LOW] Full FQDN resolution chain not implemented (public IPv4/IPv6 fallback,
  dev-TLD detection, `GetDisplayURL`/wildcard inference from `DOMAIN` list); only
  `DOMAIN` env + `os.Hostname()`. — `src/ssl/ssl.go`, `src/server/server.go`
- (DNS-01 challenge — see deferred section below; optional per spec.)

## PART 16 — Web Frontend

- [HIGH] Link update & delete do not exist anywhere (no service, no handler, web or
  API); `model.UpdateURLRequest` defined but unused. — `service/url.go`, `handler/url.go`, dashboard UI
- [HIGH] Link-management frontend beyond create is absent: no per-link stats page,
  no edit/delete UI, no web QR-display page, no bulk import/export forms (QR/stats/
  bulk exist only as API). — new templates + web routes
- [MED] Link options under-modeled. `model.URL` supports password + expiration only;
  geo-restriction, device targeting, UTM passthrough, tags, public/private visibility
  (all in IDEA.md) not modeled or exposed. — `src/server/model/url.go`, create form

## PART 17 — Admin Panel

- [MED] Admin audit-log viewer stubbed ("coming soon. Query the audit_log table
  directly"). Needs a real paginated viewer. — `handler/admin_config.go` `ConfigLogsAudit`

## PART 18 — Email & Notifications

- [LOW] Six notification templates missing from `src/template/email/`:
  `token_regenerated.txt`, `ssl_renewal_failed.txt`, `startup.txt`, `shutdown.txt`,
  `update_available.txt`, `update_installed.txt`.

## PART 19 — Scheduler

- [MED] `update_check` job (daily 06:00) not registered — no updater tie-in (links to
  missing update emails + `defer_days`/`auto_install`). — `src/scheduler/scheduler.go`
- [MED] `cluster_heartbeat` job (30s, cluster mode) not registered. — `src/scheduler/scheduler.go`
- [MED] Task status/history not persisted. `scheduler_tasks`/`scheduler_history` tables
  exist but jobs never write `last_run`/`last_status`/`run_count`/`fail_count`/
  `next_run`, so admin scheduler status shows no real data. — `src/scheduler/scheduler.go`
- [LOW] Retry/backoff (`max_retries`, `retry_delay`, exponential backoff) not
  implemented. — `src/scheduler/scheduler.go`
- [HIGH] External cron library `github.com/robfig/cron/v3` in use — CLAUDE.md
  NEVER-do #5 requires an internal scheduler, not a third-party cron package
  (found by `go-lint` pass, 2026-07-30). — `src/scheduler/scheduler.go` line 15

## PART 20 — GeoIP

- [LOW] City DB IPv6 not downloaded — only `dbip-city-ipv4.mmdb` fetched; spec ships
  city IPv4 + IPv6. — `src/geoip/geoip.go`

## PART 21 — Metrics

- [MED] Several required metric families missing from `src/metrics/metrics.go`:
  cache (`caslink_cache_hits_total`/`_misses_total`/`_evictions_total`/`cache_bytes`),
  system (`disk_total_bytes`/`disk_used_bytes`/`memory_total_bytes`/`_used`),
  go-runtime (`go_mem_alloc_bytes`/`_mem_sys_bytes`/`gc_runs_total`/`gc_pause_total_seconds`),
  ratelimit (`ratelimit_requests_total`/`_blocked_total`),
  `scheduler_task_duration_seconds`,
  cluster (`cluster_nodes_total`/`sync_lag_seconds`/`elections_total`).

## PART 22 — Backup & Restore

- [HIGH] Backup encryption missing: no AES-256-GCM, no Argon2id key derivation, no
  `.tar.gz.enc` output, no compliance-mode enforcement (only plain `.tar.gz`). — `src/backup/backup.go`
- [MED] Backup manifest not written. SHA-256 checksum computed then discarded; no
  `manifest.json` (checksum/encrypted/method). — `src/backup/backup.go`
- [MED] Retention weekly/monthly/yearly + `max_total_size` cap not implemented
  (`keep_weekly`/`keep_monthly`/`keep_yearly`). — `src/backup/backup.go`

## PART 23 — Update Command

- [MED] `defer_days`/`published_at` eligibility filtering not implemented. — `src/updater/update.go`
- [MED] `auto_install` flow not implemented (belongs in updater + the missing
  `update_check` scheduler task). — `src/updater/update.go`, `src/scheduler/scheduler.go`

## PART 24 — Privilege Escalation & Service

- [MED] Linux service install only supports systemd; `install()` returns
  "unsupported init system" for OpenRC/runit/SysVinit/s6 (start/stop handle some but
  install/uninstall don't). — `src/svcmgr/svcmgr_linux.go`
- [MED] System user UID/GID selection algorithm not implemented. `ensureServiceUser()`
  runs bare `useradd --system` with no UID==GID matching, no 200–899 safe range, no
  `reservedIDs` map (spec mandates `findAvailableSystemID`). — `src/svcmgr/svcmgr_linux.go`
- [MED] Uninstall does not delete the system user/group (spec uninstall step 5). — `src/svcmgr/svcmgr_linux.go`
- [LOW] Escalation fallback incomplete — only `sudo`/`pkexec`; spec order is
  `sudo→su→pkexec→doas`. — `src/svcmgr` `escalateIfNeeded()`

## PART 25 — Service Support

- [HIGH] Windows service run handler missing — no `svc.Run`/`Execute` via
  `golang.org/x/sys/windows/svc`; installed Windows service won't respond to the SCM
  and won't run. — `src/svcmgr/svcmgr_windows.go`
- [MED] OpenRC/SysVinit/runit init-script templates absent (same root cause as the
  PART 24 install gap). — `src/svcmgr/svcmgr_linux.go`

## PART 26 — Makefile

- [MED] `Makefile` builds against `golang:alpine` (`GO_DOCKER`, line 44) instead of
  `casjaysdev/go:latest` (found by `go-lint`, 2026-07-30). — `Makefile`
- [MED] `GO_DOCKER` and all `go build` invocations (lines 63, 73, 86, 100) are missing
  `-e GOFLAGS=-buildvcs=false` / inline `-buildvcs=false` — required per
  `~/.claude/memory/go_conventions.md` Docker Build Pattern since `.git` is mounted.
  — `Makefile`
- [LOW] `LDFLAGS` (line 19) is missing `-trimpath` for reproducible builds. — `Makefile`

## PART 27 — Docker

- [MED] Dockerfile builder stage uses `FROM golang:alpine` instead of required
  `casjaysdev/go:latest` (found by `go-lint`, 2026-07-30 — also affects the Makefile,
  see PART 26). — `docker/Dockerfile`
- [MED] `docker/Dockerfile` `go build` (line 19) missing `-buildvcs=false`; LDFLAGS
  (line 20) missing `-trimpath`. — `docker/Dockerfile`
- [LOW] HEALTHCHECK timing wrong — spec `start-period=90s interval=10s timeout=5s`;
  Dockerfile has `10m/5m/15s`. — `docker/Dockerfile`
- [MED] Production `docker-compose.yml` non-compliant: `container_name` should be
  `caslink-app`, `restart` should be `always`, missing `pull_policy: always`,
  `x-logging` anchor, `healthcheck` block, and the `caslink-cache` (valkey) service +
  `CACHE_URL` + `depends_on`; env uses list style and sets forbidden `MODE`. — `docker/docker-compose.yml`

## PART 29 — Testing

- [LOW] Thin unit coverage — only 4 `*_test.go` (password, token, url, username).
  Critical packages untested: auth, totp, webauthn, org, domain, scheduler, backup,
  ssl, updater, config, i18n, tor. — `*_test.go` beside each package

## PART 31 — I18N & A11Y

- [LOW] `<html>` sets `lang` but not `dir`; Arabic (RTL) never renders right-to-left
  though `LanguageInfo.Direction` is computed. — `src/server/tmpl/template/layout/base.html`
- [LOW] No skip-to-content link in base layout despite `nav.skip_to_content` key. — `base.html`
- [LOW] No build-time locale key-parity validator (keys at parity now, nothing
  enforces it). — `src/common/i18n`

## PART 32 — Tor

- [LOW] Outbound-Tor per-user preference not wired. `ShouldUseTor()` unimplemented and
  `GetHTTPClient(useTor)` never called, so outbound requests never route through Tor
  and `use_tor_network` is a no-op. Optional per spec. Hidden-service half is complete.
  — `src/tor/service.go`, callers

## PART 33 — Client & Agent

- [MED] `--user` context flag not wired. Advertised in `--help`/completions but
  `GlobalFlags` has no `User` field, no `@`/`+` smart detection, and `client.do()`
  sends no user/org context header. NON-NEGOTIABLE when PART 34/35 in use. —
  `src/client/cli/commands.go`, `src/client/main.go`
- Note: `caslink-agent` binary is explicitly OPTIONAL per PART 33 — its absence is not
  a gap (Makefile guards it with `if [ -d src/agent ]`).

## PART 34 — Multi-User

- [MED] OIDC / LDAP / SAML external identity providers entirely missing. Spec: "External
  identity support MUST include OIDC, LDAP, and SAML" (multi-named, PKCE+state+nonce,
  role_mapping, admin routes `/server/{admin_path}/config/security/auth/*`);
  autodiscover returns empty `oauth:[]`. Largest single gap. — new
  `src/server/service/oidc|ldap|saml`, config, `handler/admin_config.go`
- [MED] Social OAuth login (Google/GitHub, per IDEA.md) absent — same subsystem.
- [LOW] Avatar upload/management not implemented (model has `Avatar` field; no upload
  handler, raster-only rule). — `handler/user.go` + service
- [LOW] Profile privacy/visibility settings not implemented (no field on user model). —
  `src/server/model/user.go`, user handler/service
- [MED] "Remember this device" 2FA bypass incomplete. `trusted_devices` table exists but
  nothing inserts/checks it; only session-duration rememberMe is wired. —
  `src/server/service/auth.go`, two-factor flow

## PART 35 — Organizations

- [HIGH] Member management backend missing — `members.html` POSTs
  `action=remove/change_role` but `OrgMembers` is GET-only; no AddMember/RemoveMember/
  ChangeRole/Invite. Forms are dead. — `src/server/service/org.go`, `handler/org.go`, `server.go`
- [MED] Member invite flow missing (no generation/redemption despite `allow_invites`/
  invite modes). — org service/handler + new invite table
- [MED] `org_preferences` table missing (spec maps `default_role`/`require_2fa`/
  `allow_invites` to it). — `src/server/store/store.go`
- [MED] Org-level audit log not wired for member/permission/email changes (generic
  `audit_log` exists but org mutations besides transfer record nothing). — `service/org.go`

## PART 36 — Custom Domains

- [HIGH] Automatic Let's Encrypt SSL for custom domains not implemented. Domains created
  with `ssl_status "none"`; no per-domain ACME issuance (`src/ssl/ssl.go` handles only the
  primary cert). — `src/server/service/domain.go` + ssl integration
- [MED] No scheduled DNS re-verification with exponential backoff (verification is manual
  `POST /verify` only; scheduler has no domain-verify job). — `src/scheduler/scheduler.go`, domain service
- [LOW] `IsApex` detection buggy — `service/domain.go:61` uses
  `!strings.Contains(domain, ".")`, so `example.com` is misclassified as non-apex. — `src/server/service/domain.go`

---

## Convention Fixes (CLAUDE.md exit-code rule, non-spec)

- [LOW] `log.Fatalf()` used instead of `os.Exit()` with an explicit sysexits code —
  sets exit code 1 only (found by `go-lint`, 2026-07-30). —
  `src/server/tmpl/tmpl.go` line 114, `src/server/server.go` line 936

## Deferred / intentional (do NOT re-flag)

### PART 15 — DNS-01 Challenge (optional per spec)

DNS-01 Let's Encrypt challenge is optional per AI.md PART 15.
HTTP-01 and TLS-ALPN-01 are both implemented.
DNS-01 not implemented — deferred only because spec marks it optional.

### PART 12 — Config validation uses fmt.Printf instead of structured logger

`config.Validate()` emits warnings via `fmt.Printf`. The spec says "warn and replace
with default" but doesn't prescribe the logging mechanism. Using the app logger would
be better but requires restructuring the config load sequence since the logger is
initialized after config. Low priority.

### PART 28 — CI/CD Workflows

User said "No — leave empty for now" on 2026-06-04.
**All workflow files were deleted in commit `39bc9bf174ee`.**
`.github/workflows/` is intentionally empty — do NOT recreate without explicit user
instruction.

### Federation (out-of-scope for v1)

`FederationConfig` struct present; no service, no `/.well-known/caslink`, no discovery
or sync. Deferred by design — spec marks federation optional.

---

## Bootstrap Status

`.claude/rules/` regenerated from AI.md. All 14 rule files present:
- `ai-rules.md` (PART 0, 1)
- `project-rules.md` (PART 2, 3, 4)
- `config-rules.md` (PART 5, 6, 12)
- `binary-rules.md` (PART 7, 8, 33)
- `backend-rules.md` (PART 9, 10, 11, 32)
- `api-rules.md` (PART 13, 14, 15)
- `frontend-rules.md` (PART 16, 17)
- `features-rules.md` (PART 18-23)
- `service-rules.md` (PART 24, 25)
- `makefile-rules.md` (PART 26)
- `docker-rules.md` (PART 27)
- `cicd-rules.md` (PART 28)
- `optional-rules.md` (PART 34, 35, 36)
- `testing-rules.md` (PART 29, 30, 31)

---

Last refreshed: 2026-07-30
