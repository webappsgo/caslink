# Features Rules (PART 18, 19, 20, 21, 22, 23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

**Email & Notifications (PART 18)**
- NEVER attempt to send emails without a valid, tested SMTP connection
- NEVER queue emails hoping SMTP will be configured later
- NEVER log "would have sent email" messages when SMTP is unavailable
- NEVER show email-dependent UI options if SMTP is not configured
- NEVER store the SMTP-derived backup password (see Backup) or leak SMTP passwords in logs
- NEVER omit the required disclaimer/visible-link/recipient fields from account emails (welcome, password reset, verify, login alert, security alert, 2FA change, password changed, breach notification)
- NEVER include links in unsolicited-action emails that delete/modify data without prior authentication
- NEVER send both `scheduler_error` AND a more specific failure event (`backup_failed`, `ssl_renewal_failed`) for the same execution — suppress the generic one

**Scheduler (PART 19)**
- NEVER use external schedulers: cron/crond/crontab/systemd timers/at/anacron (Linux), Task Scheduler/schtasks (Windows), launchd (macOS), Kubernetes CronJob, cloud schedulers (AWS/Azure/GCP) — no exceptions, ever
- NEVER tell a user to set up cron instead of the built-in scheduler — always redirect to the admin panel scheduler config
- NEVER let more than one cluster node run a "Global Task" simultaneously (must use DB-backed locking)
- NEVER disable the critical/non-skippable tasks: `session_cleanup`, `token_cleanup`, `log_rotation`, `healthcheck_self`, `tor_health`, `cluster_heartbeat`, `ssl_renewal`

**GeoIP (PART 20)**
- NEVER embed GeoIP databases in the binary — always download on first run + scheduled update
- NEVER use `geoip2-golang` — use `github.com/oschwald/maxminddb-golang` (ip-location-db's custom `database_type` strings break `geoip2.Open()`)
- NEVER set both `deny_countries` and `allow_countries` expecting deny to apply — `allow_countries` always wins when both are set
- NEVER country-block private/internal (RFC 1918) IPs or allowlisted IPs

**Metrics (PART 21)**
- NEVER expose `/metrics` publicly — internal only (firewall/NetworkPolicy/security group)
- NEVER use a raw client IP, user ID, or request ID as a metric label (unbounded cardinality = memory DoS) — aggregate or cap (e.g. fixed-size LRU); log per-IP detail to structured logs instead
- NEVER omit the `{project_name}_` prefix on metric names
- NEVER use non-base units (ms, KB) — always seconds/bytes with `_seconds`/`_bytes`/`_total` suffixes

**Backup & Restore (PART 22)**
- NEVER store the backup encryption password — it is never persisted, admin must remember it
- NEVER allow scheduled/manual backups to run in compliance mode without an encryption password set (must block)
- NEVER delete existing valid backups if the new backup fails ANY verification check
- NEVER create a backup when free space < 2× the most recent backup size, or disk usage exceeds `disk_threshold` (default 90%) — abort and log `backup.skipped_disk_full`
- NEVER let a restored Primary Admin skip re-authentication via setup token when restoring to a new server
- NEVER re-resolve the backup directory path at cleanup time — use the path cached at startup step 7

**Update Command (PART 23)**
- NEVER install an update without a valid SHA256 checksum asset — refuse unverified updates
- NEVER let `update_check`'s `defer_days` gating apply to a manual `--update check`/`--update yes` — manual is always the true latest
- NEVER auto-install updates unless `update.auto_install: true` is explicitly set (default is notify-only)
- NEVER roll out `auto_install` updates to all cluster nodes simultaneously — node-by-node only
- NEVER surface update-available / version info on public pages (Tier 3 info) — Server Admin routes only; PWA "new version" banner is the sole exception (discloses no server version)

## CRITICAL - ALWAYS DO

**Email & Notifications**
- ALWAYS auto-detect local SMTP on first run (127.0.0.1, docker gateway, default gateway, FQDN, global IPv4, mail./smtp. subdomains — ports 25/465/587) and test the connection on every startup
- ALWAYS fall back to the embedded default template when no custom template exists at `{config_dir}/template/email/`
- ALWAYS use both WebUI notification (toast/banner/center) as the always-available channel, and email only when SMTP works AND the event is critical/needs a permanent record
- ALWAYS prefix test emails with `[TEST]` and log them to the audit log
- ALWAYS validate templates before saving (unknown variables, missing required vars, empty subject/body)

**Scheduler**
- ALWAYS keep the scheduler running continuously from app start to shutdown, state persisted in `server.db`
- ALWAYS run missed tasks on startup if within `catch_up_window` (default 1h)
- ALWAYS use DB-backed locking (5-minute timeout) for Global Tasks in cluster mode
- ALWAYS log every task execution to the audit log and update `last_run`/`last_status`/`run_count`/`fail_count`

**GeoIP**
- ALWAYS bypass country blocking for IPs in `server.security.allowlist`
- ALWAYS skip country blocking with a warning (not an error) if `country.mmdb` is missing

**Metrics**
- ALWAYS follow Prometheus naming: `{project_name}_` prefix, snake_case, `_total`/`_seconds`/`_bytes` suffixes
- ALWAYS normalize dynamic path segments (UUIDs, numeric IDs → `:id`) before using as a label value
- ALWAYS support optional Bearer token auth on `/metrics`

**Backup & Restore**
- ALWAYS verify a backup immediately after creation (file exists, size>0, checksum, decrypt test, manifest parse, content extraction, DB integrity) — ALL checks must pass before old backups are pruned
- ALWAYS require the backup password for `.tar.gz.enc` restores (CLI prompt/flag, WebUI dialog, or API 400 `password_required`)
- ALWAYS require Primary Admin re-authentication via one-time setup token after restoring to a new server; preserve existing credentials
- ALWAYS apply retention priority: yearly > monthly > weekly > daily, oldest deleted first within each tier
- ALWAYS enforce `max_total_size` (hard cap) after count-based retention if set (deletes oldest first, overrides count limits)

**Update Command**
- ALWAYS verify SHA256 checksum of the downloaded binary before replacing the running binary
- ALWAYS use HTTP 404 from GitHub API as "no update available" (not an error)
- ALWAYS honor channel cumulativity: `beta` = beta+stable, `daily` = daily+beta+stable — never propose a release at or below the running version
- ALWAYS use platform-specific binary replacement: Unix = atomic rename over running exe; Windows = rename-to-`.old` + `MOVEFILE_DELAY_UNTIL_REBOOT`

## Key Rules

### Email & Notifications
| Area | Rule |
|------|------|
| Template storage | Embedded defaults in binary; custom overrides in `{config_dir}/template/email/` |
| SMTP config | `server.notifications.email.smtp.*`; overridable via `SMTP_HOST`/`SMTP_PORT`/`SMTP_USERNAME`/`SMTP_PASSWORD`/`SMTP_TLS`/`SMTP_FROM_NAME`/`SMTP_FROM_EMAIL` |
| Default sender | From: `{app_name}`, `no-reply@{fqdn}` (or `no-reply@localhost`) |
| Template format | `Subject: ...` line, `---` separator, plain-text body, `{variable}` syntax |
| Required templates | welcome, password_reset, email_verify, login_alert, security_alert, mfa_reminder, 2fa_enabled/disabled, password_changed, token_regenerated, backup_complete/failed, ssl_expiring/renewed/renewal_failed, scheduler_error, startup, shutdown, update_available/installed, breach_notification, breach_admin_alert, test |
| Global vars | `{app_name}`, `{app_url}`, `{fqdn}`, `{onion_url}`, `{onion_address}`, `{i2p_url}`, `{i2p_address}`, `{admin_email}`, `{recipient_email}`, `{recipient_username}`, `{timestamp}`, `{year}` |
| Notification storage | `admin_notifications` / `user_notifications` tables, 30-day retention, 100 max, real-time via WebSocket |
| Decision logic | active user → WebUI always; SMTP off → WebUI only; critical/security/needs-record-while-away → also email; routine success → no notification |

Admin panel: `/server/{admin_path}/config/email/templates` · Preferences: `/server/{admin_path}/{admin_username}/notifications` and `/users/settings/notifications`.

### Scheduler
| Task | Default Schedule | Skippable |
|------|-------------------|-----------|
| `ssl_renewal` | Daily 03:00 | No |
| `geoip_update` | Weekly Sun 03:00 | Yes |
| `blocklist_update` | Daily 04:00 | Yes |
| `cve_update` | Daily 05:00 | Yes |
| `update_check` | Daily 06:00 | Yes |
| `session_cleanup` | Every 15m | No |
| `token_cleanup` | Every 15m | No |
| `log_rotation` | Daily 00:00 | No |
| `backup_daily` | Daily 02:00 | Yes |
| `backup_hourly` | Hourly (disabled default) | Yes |
| `healthcheck_self` | Every 5m | No |
| `tor_health` | Every 10m | No (if Tor installed) |
| `cluster_heartbeat` | Every 30s | No |

- Config: `server.scheduler.timezone` (default `America/New_York`), `catch_up_window` (default `1h`)
- Cluster: Global Tasks run on one node (ssl_renewal, geoip_update, blocklist_update, cve_update, backup_daily, update_check); Local Tasks run on every node (session_cleanup, token_cleanup, healthcheck_self, cluster_heartbeat)
- Retry: `max_retries: 3`, `retry_delay: 5m`, exponential backoff
- Admin panel: `/server/{admin_path}/config/scheduler` — task list, run now, enable/disable, history (100 entries)
- API: `GET/PATCH /api/{api_version}/server/{admin_path}/config/scheduler[/{id}][/run|/enable|/disable|/history]`
- Shutdown: stop new executions, wait up to 30s for running tasks, force-release locks + mark interrupted for retry on timeout

### GeoIP
| Setting | Default |
|---------|---------|
| `server.geoip.enabled` | true |
| `dir` | `{data_dir}/security/geoip` |
| `deny_countries` / `allow_countries` | `[]` / `[]` (mutually exclusive; allow wins if both set) |
| `databases.asn/country/city` | all true |

- Source: sapics/ip-location-db (GitHub Releases, no API key)
- City DBs ship IPv4/IPv6 as separate MMDB files
- Admin panel: `/server/{admin_path}/config/network/geoip`

### Metrics
| Setting | Default |
|---------|---------|
| `server.metrics.enabled` | true |
| `endpoint` | `/metrics` |
| `include_system` / `include_runtime` | true / true |
| `token` | `""` (no auth) |

Required metric groups: App Info, HTTP, Database (if using DB), Auth. Optional: Cache, Scheduler, Business, System, Go Runtime, Cluster, Tor, Rate Limiting.
Library: `github.com/prometheus/client_golang`. Handler wraps `promhttp.Handler()` with optional `Authorization: Bearer <token>` check.
Admin panel: `/server/{admin_path}/config/metrics`.

### Backup & Restore
| Item | Value |
|------|-------|
| Command | `{project_name} --maintenance backup [filename]` |
| Restore | `{project_name} --maintenance restore <backup-file> [--password ...]` |
| Full backup filename | `{project_name}_backup_YYYY-MM-DD.tar.gz[.enc]` |
| Manual/timestamped | `{project_name}_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]` |
| Daily incremental | `{project_name}-daily.tar.gz[.enc]` (always 1 file) |
| Hourly incremental | `{project_name}-hourly.tar.gz[.enc]` (disabled by default) |
| Encryption | AES-256-GCM, key via Argon2id; required if `server.compliance.enabled: true` |
| Retention | `max_backups` (default 1), `keep_weekly`/`keep_monthly`/`keep_yearly` (default 0), `max_total_size` (default `10%`, overrides counts) |
| Falsey values | `0`, `false`, `no`, `none`, `disable`, `disabled`, `off` |

Contents: `server.yml`, `server.db`, `users.db` always; `template/`, `theme/` if present; `ssl/` with `--include-ssl`; `{data_dir}/` with `--include-data`.
Admin recovery: `{project_name} --maintenance setup` clears admin password + API token, prints one-time setup token; never touches user accounts/data.

### Update Command
| Item | Value |
|------|-------|
| Command | `{project_name} --update [yes\|check\|branch {stable\|beta\|daily}]` |
| Alias | `--maintenance update` = `--update yes` |
| Exit codes | 0 = updated/current, 1 = error |
| Config | `server.update.branch` (default `stable`), `auto_install` (default `false`), `defer_days` (default `0`, 0-365) |
| Source | GitHub Releases API (`{project_org}/{project_name}`) |

Channels cumulative: stable ⊂ beta ⊂ daily. `defer_days` gates the scheduled `update_check` task only, never manual commands. Update flow: check → download to temp → verify SHA256 → platform-specific binary replace → restart (systemd/launchd/rc.d/SCM per platform).

---
For complete details, see AI.md PART 18, 19, 20, 21, 22, 23
