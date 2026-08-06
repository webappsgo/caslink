# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

- src/updater/update.go `DoUpdateFor()` (lines 97-164): the success path
  from a verified download through `replaceBinary` (lines 148-163) is not
  covered by any test. `currentPath` comes from `os.Executable()` with no
  override, so exercising this branch in a unit test would rename over the
  actual running `go test` binary — unsafe and avoided deliberately (see
  `src/updater/update_test.go`
  `TestDoUpdateFor_ChecksumMismatchStopsBeforeReplace`, which verifies the
  pipeline up to but not through `replaceBinary`). `replaceBinary` itself is
  separately unit-tested end-to-end with synthetic temp files in
  `src/updater/replace_unix_test.go`. Would need `DoUpdateFor` to accept an
  injectable "current binary path" (or a seam around `os.Executable`) to
  close this gap safely. Left unfixed — behavior change to production code,
  out of scope for a test-only pass.

- `server.config.trusted_proxies.additional` entries that are hostnames (not
  IPs/CIDRs) are not DNS-resolved before matching. `isTrustedPeer()` compares
  the raw TCP peer IP against parsed IP/CIDR entries only, so a hostname entry
  silently never matches. Resolving hostnames on the request path would put a
  blocking DNS lookup in front of every request; doing it correctly needs a
  background resolver with a TTL cache refreshed on the 5-minute proxy-refresh
  cycle (per config-rules "refreshed every 5 min"). Deferred — needs that
  resolver design; IP/CIDR entries (the documented common case) work today.

## Deferred from the 2026-08 extended PART 13-36 audit

These surfaced during the extended audit (tracker was AUDIT.AI.md, now
deleted). Each is feature-sized or needs verification unavailable in this
environment; all bounded security/logic/config findings from that pass were
fixed inline and committed separately.

- API version abstraction: the codebase hardcodes `v1` in routes and the
  swagger/health handlers instead of driving it from a `server.api_version`
  config value through an `APIBasePath()` helper (PART 13/14). Making this
  configurable is a pervasive, cross-cutting change touching every route
  registration, every handler doc-path, swagger/graphql generation, and the
  CLI client's base-path resolution — a design change, not a spot fix.
  Deferred; `v1` is correct and stable for the current single API version.

- SSL certificate lookup order (PART 15): startup should probe
  `/etc/letsencrypt/live/{domain}/` → `/etc/letsencrypt/live/{fqdn}/` →
  `{config_dir}/ssl/letsencrypt/{fqdn}/` → `{config_dir}/ssl/local/{fqdn}/`
  before requesting a new cert, and only auto-renew the app-managed
  `{config_dir}/ssl/letsencrypt/` path. The current server uses autocert
  directly without the 4-step discovery of pre-existing certbot/local certs.
  Feature gap — needs the discovery layer plus renewal-ownership gating and a
  real cert/host to verify. Deferred.

- Service user UID/GID safe-range allocation (PART 24): `ensureServiceUser()`
  delegates to `useradd --system`, so it does not enforce the spec's
  UID==GID, 200-899 safe range, or reserved-ID skip. Correct enforcement means
  scanning existing IDs and creating the group/user with an explicit matching
  numeric id; verifying it needs a root Linux host with the real user/group
  databases. Deferred — cannot be exercised in this container-only environment.

- Frontend CSP externalization (PART 16): move any remaining inline handlers
  into external app.js, replace native confirm/alert/prompt with `<dialog>`
  components, and tighten CSP back to `script-src 'self'`. Large UI change that
  needs browser verification of every interactive page. Deferred.

- PART 36 full custom-domains build: AddDomain now enforces limits, reserved,
  and blocked_patterns; VerifyDomain now performs real TXT ownership-token
  verification (net.LookupTXT of _verify.{domain} + constant-time compare),
  AddDomain issues the token and returns dns_instructions, and the schema
  carries verification_token plus the status/ssl_expires indexes. The
  scheduled verify-retry/cleanup task is now implemented: the global,
  skippable `domain_verification` scheduler task (DomainVerificationCron
  @every 30m) re-checks in-TTL pending/failed rows via
  DomainService.RetryPendingVerifications and deletes rows left unverified
  past verification_ttl via CleanupExpiredPendingVerifications. The user and
  org custom-domain API routes now exist too, at CRUD parity with the web
  routes (PART 14): GET/POST /api/v1/users/domains and
  POST /api/v1/users/domains/{domain}/verify, plus the org equivalents under
  /api/v1/orgs/{slug}/domains — all bearer-or-session authenticated, with the
  same ownership scoping and owner/admin role gating as the web handlers.
  Still remaining: resolver middleware, admin web/API routes, DNS-01 ACME
  issuance and cert persistence, scheduled SSL-renewal task, domain caching,
  and rate limiting. Deferred.

- PART 34 registration modes: the open/invite/admin_only/disabled gate is
  implemented — `RegistrationConfig.Mode` (default open),
  `NormalizedMode()`/`PublicSelfRegistrationAllowed()`, and the public
  `/register` GET+POST reject with 403 under any non-open mode. The
  Server-Admin invite/activation-link flow is now implemented end-to-end: the
  admin invite-management page (`/server/{admin_path}/config/users/invites`)
  creates single-use, 7-day-default invites via the shared InviteService
  (one-time acceptance link shown once), lists active invites, and revokes
  them (audit-logged); the public `/register` GET+POST accept a valid
  `?invite=` / hidden-field user-registration token even under invite /
  admin_only mode and consume it on successful account creation. Still
  remaining: explicit rejection of unused links when mode flips to disabled
  (RevokeUnusedByKind is wired in the service but not yet triggered on the
  mode-change save path). Feature build.

- PART 35 organization creation policy: the creation gate is implemented —
  `OrganizationsConfig.Creation.Mode` (default open) with
  `NormalizedCreationMode()`/`AuthenticatedCreationAllowed()`, the per-user
  `max_per_user` limit enforced via `OrgService.CountOwnedOrgs`, and both the
  form (`/orgs`) and API (`/api/v1/orgs`) create paths reject with 403 under
  any non-open mode or when the limit is reached. Still remaining: the
  invite-code creation flow and admin_only admin-initiated creation (both
  depend on the shared invite subsystem), plus per-org `visibility`
  (public/private) which is a distinct org-level setting. Org slug
  validation is now spec-aligned (PART 35: 2-39 chars, no consecutive
  hyphens, reserved-name blocklist) via the shared
  `validate.ValidateOrgSlug`, used by both the handler and the service. The
  one remaining slug gap is the full shared-namespace check — a slug must
  also not collide with an existing username (spec `CheckNameAvailable`);
  today only org-vs-org slug uniqueness is enforced at the data layer.
  Cross-table user/org collision needs the unified namespace lookup and is
  deferred with the rest of this feature build.

- Admin panel WebUI completeness + cookie-consent banner (PART 16/17): the
  full set of admin config templates and the always-on GDPR cookie-consent
  banner (fixed bottom, server-read `cookie_consent` cookie, essential/
  preferences/analytics categories) are a UI feature build needing browser
  verification. Deferred.
