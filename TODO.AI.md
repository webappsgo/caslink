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
  routes (PART 14): GET/POST /api/v1/users/domains,
  GET/DELETE /api/v1/users/domains/{domain},
  GET /api/v1/users/domains/{domain}/dns, and
  POST /api/v1/users/domains/{domain}/verify, plus the org equivalents under
  /api/v1/orgs/{slug}/domains — all bearer-or-session authenticated, with the
  same ownership scoping and owner/admin role gating as the web handlers.
  DeleteOwnedDomain enforces owner_type+owner_id in the DELETE so cross-owner
  deletes 404 and never remove another owner's row (tested); reads of another
  owner's domain 404 rather than leak the record.
  The read-only SSL-status endpoint is now implemented too:
  GET /api/v1/users/domains/{domain}/ssl and the org equivalent
  GET /api/v1/orgs/{slug}/domains/{domain}/ssl (plus the web-mux
  variants) report DomainService.SSLStatusFor — {enabled, status,
  expires_at, challenge_type (http-01 non-wildcard / dns-01 wildcard),
  auto_managed (!wildcard), eligible (IsDomainVerifiedActive)} — with the
  same owner/membership scoping as the detail routes (owner reads 200,
  cross-user 404, org member 200, non-member 403; tested). It is
  deliberately read-only: non-wildcard issuance and renewal are already
  automatic via the server's autocert.Manager dynamic HostPolicy, so the
  endpoint never triggers issuance.
  The custom-domain resolver middleware and domain caching are now
  implemented: DomainService.Resolve(host) resolves a verified+active,
  non-wildcard domain (host normalized for port/case/trailing-dot) via a 60s
  in-memory positive+negative cache, invalidated on VerifyDomain success;
  server.CustomDomainMiddleware attaches the resolved *CustomDomain to the
  request context (attach-on-match, pass-through-on-miss so main-site and
  health traffic is never broken) and is registered via router.Use before any
  route mount; and RedirectURL now routes short-code lookups through the new
  owner-scoped URLService.GetURLByCodeForOwner when a request arrives on a
  custom domain, so a custom domain serves only its owner's links.
  The admin domain API is now implemented: the RequireBearerAdmin-gated
  /api/v1/server/{admin_path}/config/domains subtree (GET list paginated,
  GET/{domain}, DELETE/{domain} force-delete, POST/{domain}/suspend,
  POST/{domain}/unsuspend) backed by DomainService
  GetDomainByName/ListAllDomains/SuspendDomain/UnsuspendDomain/
  AdminDeleteDomain, each writing a custom_domain_audit row plus a
  server-wide admin audit.log entry and invalidating the resolve cache.
  The custom_domain_audit lifecycle trail is now wired on the ownership
  paths too: AddDomain records "created" and VerifyDomain records "verified"
  (spec canonical vocabulary created/verified/ssl_issued/suspended/deleted),
  with TestDomainAuditTrail asserting the created/suspended/unsuspended
  sequence. The admin WEB pages are now implemented too: the CSRF-protected,
  no-JS `/server/{admin_path}/config/domains` subtree (GET list paginated,
  GET/{domain} view/manage, POST/{domain}/suspend, POST/{domain}/unsuspend,
  POST/{domain}/delete force-delete) rendered via the admin layout and backed
  by the same DomainService methods as the API, with a "Domains" sidebar nav
  entry and POST-redirect-GET flash flow (admin_domains.go + tests).
  Still remaining:
  POST /{domain}/ssl (DNS-01 provider config + AES-256-GCM credentials),
  POST /{domain}/ssl/renew force-renew (needs autocert-cache purge
  design), DNS-01 ACME issuance and cert persistence, and rate limiting.
  Deferred. (Scheduled SSL renewal and HTTP-01/TLS-ALPN-01 issuance are
  already handled automatically by autocert.Manager — the ssl_renewal
  scheduler task is an intentional no-op.)

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
  admin_only mode and consume it on successful account creation. The
  disabled-mode rejection of existing unused invite/activation links is now
  implemented: `RegistrationConfig.InviteAcceptanceAllowed()` gates the
  acceptance path (`hasValidRegistrationInvite` rejects every token when the
  mode is disabled), and server startup proactively revokes outstanding unused
  user-registration invites via `RevokeUnusedByKind` when the configured mode
  is disabled (server.yml is the source of truth for the mode). PART 34
  registration-mode work is complete.

- PART 35 organization creation policy: the creation gate is implemented —
  `OrganizationsConfig.Creation.Mode` (default open) with
  `NormalizedCreationMode()`/`AuthenticatedCreationAllowed()`, the per-user
  `max_per_user` limit enforced via `OrgService.CountOwnedOrgs`, and both the
  form (`/orgs`) and API (`/api/v1/orgs`) create paths reject with 403 under
  any non-open mode or when the limit is reached. Per-org `visibility`
  (public/private) is now enforced: the `OrgService.CanViewOrg` helper
  (public → any authenticated user; private → members only) gates the web
  dashboard (`OrgDashboard`) and the `APIGetOrg`/`APIGetMembers` API reads, so
  a non-member of a private org gets 404 (not 403) — its existence is never
  leaked — while public orgs are viewable by anyone. Storage/editing of the
  setting already existed (`NormalizeOrgVisibility`, `UpdateOrganization`,
  settings form, `APIUpdateOrg`); this closed the enforcement gap. No public
  org listing/search exists (`ListUserOrganizations` is membership-scoped), so
  "won't appear in public listings" is already satisfied. Still remaining: the
  invite-code creation flow and admin_only admin-initiated creation (both
  depend on the shared invite subsystem). Org slug
  validation is now spec-aligned (PART 35: 2-39 chars, no consecutive
  hyphens, reserved-name blocklist) via the shared
  `validate.ValidateOrgSlug`, used by both the handler and the service. The
  full shared-namespace check is now implemented too: `CheckNameAvailable`
  (src/server/service/namespace.go) consults the reserved blocklist plus both
  the users and organizations tables, and is wired into both creation paths —
  `OrgService.CreateOrganization` (org slug blocked by an existing username or
  slug) and `AuthService.RegisterUser` (username blocked by an existing org
  slug), each returning a generic error to avoid enumeration. Both directions
  are covered by namespace_test.go (TestCreateOrganizationRejectsUsernameCollision
  and TestRegisterUserRejectsOrgSlugCollision). Shared-namespace work complete.

- Admin panel WebUI completeness + cookie-consent banner (PART 16/17): the
  full set of admin config templates and the always-on GDPR cookie-consent
  banner (fixed bottom, server-read `cookie_consent` cookie, essential/
  preferences/analytics categories) are a UI feature build needing browser
  verification. Deferred.
