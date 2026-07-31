# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

## GraphQL resolvers (PART 14 — REQUIRED)

- `src/graphql/` has `schema.go` + `graphql.go` but no `resolvers.go`.
  `executeQuery` was changed during the audit to return an honest
  `{"data":null,"errors":[{"message":"GraphQL resolvers are not yet implemented"}]}`
  instead of a fake healthy response. Implement real resolvers backed by the
  URL/user/org services so the REQUIRED GraphQL endpoint actually works.

## CI/CD workflows (PART 28 — gap)

- `.github/workflows/` is empty. PART 28 requires security workflow(s) plus
  `ci.yml` / `release.yml`. Create security-only workflows first, then
  `ci.yml`/`release.yml` last, per cicd_conventions.md. Third-party Actions
  pinned to full commit SHA, never tags.

## DSN credential escaping (regression-risk)

- `src/server/store/factory.go` — `buildPostgresDSN` (key=value),
  `buildMySQLDSN` (`user:password@tcp`), and `buildSQLServerDSN` (URL) do not
  escape credentials containing special characters (spaces, `@`, `:`, `/`, `'`).
  A password with these chars would produce a malformed DSN or connect to the
  wrong target. Fix per-driver (pq quoting / url.UserPassword / mssql URL
  encoding) with tests — deferred because each driver quotes differently and a
  naive fix risks breaking currently-working simple passwords.

## SMTP pre-flight (robustness)

- Email send path assumes `SMTPConfigured` implies a reachable server. Add a
  connect/dial pre-flight (or send-time error surfacing) so misconfigured SMTP
  fails loudly at config/test time rather than silently dropping mail.

## Recovery-key / backup-code serialization (now JSON, verify migration)

- `totp.UseRecoveryKey` was moved off fragile string-split parsing to
  `encoding/json` + a transaction during the audit. Confirm all existing rows
  in `totp_secrets.backup_codes` are valid JSON arrays (they should be, since
  generation already wrote JSON) — add a one-time migration/validation if any
  legacy non-JSON rows could exist.
