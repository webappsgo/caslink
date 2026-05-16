# Features Rules (PART 18-23)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## EMAIL & NOTIFICATIONS (PART 18)
- SMTP providers: gomail, SendGrid, Amazon SES
- Auto-detect SMTP settings from common providers
- Notification templates stored in DB, customizable by operator
- Events: registration, password reset, 2FA changes, link expiration, billing

## SCHEDULER (PART 19)
- Built-in cron scheduler (robfig/cron) - NEVER external cron or systemd timers
- Scheduled tasks:
  - SSL renewal check
  - GeoIP database update
  - Expired link cleanup
  - Analytics aggregation
  - Dunning retries
  - Audit log compaction

## GEOIP (PART 20)
- GeoIP2 database for country/city lookup
- Stored at `{data_dir}/security/geoip/`
- Updated on schedule via built-in scheduler
- Used as risk SIGNAL only, never sole access gate
- IPs anonymized before storage when `analytics.anonymize_ips: true`

## METRICS (PART 21)
- Prometheus metrics at `/metrics` - INTERNAL ONLY
- Never expose /metrics publicly
- Optional bearer token auth for /metrics

## BACKUP & RESTORE (PART 22)
- `--maintenance backup` triggers manual backup
- Scheduled backups via built-in scheduler
- Backup location: `{backup_dir}/`

## UPDATE COMMAND (PART 23)
- `--update check` - check for updates
- `--update yes` - apply updates
- `--update branch {stable|beta|daily}` - switch channel

---
For complete details, see AI.md PART 18-23
