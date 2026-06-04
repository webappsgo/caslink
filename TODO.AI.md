# TODO.AI.md — Caslink Outstanding Work

Tracks remaining unimplemented items by AI.md PART number.
Items are removed once fully implemented and committed.

---

## PART 19 — Scheduler

- [x] `blocklist_update` task: download updated IP/domain blocklists used by URL
      validation when blocklist sources are configured in `server.yml`.
- [x] `cve_update` task: download updated CVE/security databases used by the
      admin security panel when CVE sources are configured in `server.yml`.

**Already implemented:** `backup_daily` (calls `backup.RunBackup`),
`log_rotation` (gzip + prune), `session_cleanup`, `token_cleanup`,
`healthcheck_self`, `expire_urls`, `ssl_renewal`, `geoip_update`,
`tor_health`.

---

## PART 28 — CI/CD Workflows (CRITICAL)

- [ ] `.github/workflows/` is empty — workflows were deliberately removed in
      commit `afd9bb16`. Per `cicd-rules.md` the following are required:
      `build.yml`, `release.yml`, `security.yml`, `beta.yml`, `daily.yml`,
      `docker.yml`. Restore when code is ready to ship (all tests pass, lint
      clean). Previous pinned-SHA versions are in commit `8934543308b1`.
- [ ] `.gitea/workflows/` mirror of the above (same Gitea Actions syntax).
- [ ] `Jenkinsfile` — check if present and spec-compliant.

---

## PART 32 — Tor Hidden Service

- [x] `binetор` Cyrillic alias fixed → `binetor` (ASCII) in `src/tor/service.go`.

---

## PART 34 — Multi-User / WebAuthn

- [x] Admin panel: "force regenerate recovery keys for user" action implemented
      at `POST /server/{adminPath}/config/users/{id}/recovery-keys` (HTML) and
      `POST /api/v1/server/{adminPath}/config/users/{id}/recovery-keys` (JSON API).

---

## PART 35/36 — Organisations / Custom Domains

- [x] Org-scoped API tokens: `GET /api/v1/orgs/{slug}/tokens`,
      `POST /api/v1/orgs/{slug}/tokens`,
      `DELETE /api/v1/orgs/{slug}/tokens/{tokenID}` — routed and implemented.
- [x] Org ownership transfer: `POST /api/v1/orgs/{slug}/transfer` — implemented.

---

## Federation (Out-of-scope for v1)

- [ ] `FederationConfig` struct present; no service, no `/.well-known/federation`,
      no discovery or sync. Deferred.

---

## PART 17 — Admin Panel Config Pages

All 24+ admin config pages are now implemented and committed (6afdf87b).
Routes, handlers, KV-store persistence, and API endpoints are all live.
No remaining PART 17 items.

Last refreshed: 2026-06-04
