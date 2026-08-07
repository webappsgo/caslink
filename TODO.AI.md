# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

## Resolved in the 2026-08 admin/backlog completeness pass

- Admin API tokens (PART 11/17): DONE. `/config/security/tokens` GET+POST were
  silent no-op stubs; now wired to a real TokenService (create shows the
  plaintext once and never in a URL/cookie, list, revoke), scoped by
  owner_type+owner_id so one owner can never see or revoke another's token.
- Admin metrics status page (PART 17/21): DONE. Added the missing
  `/config/metrics` read-only status page + sidebar entry (the /metrics
  endpoint itself was already wired); the bearer token value is never rendered,
  only whether auth is required.
- Updater success-path test gap (PART 23): DONE. Every prior `DoUpdateFor` test
  covered an error path; added `TestDoUpdateFor_SuccessReplacesBinary`, which
  drives the full download → SHA256-verify(match) → installUpdatedBinary →
  atomic replaceBinary path via the `currentExecutable` seam against a
  synthetic target, asserting the target is overwritten and left executable.
- trusted_proxies.additional hostname DNS resolution: VERIFIED ALREADY COMPLETE
  — `src/config/trustedproxy.go` resolves hostname entries via `net.LookupIP`
  with a 5-minute background refresh and a resolved-IP cache checked under read
  lock in `IsTrusted`. No code change needed.
- Cluster join-token subsystem (PART 34): DONE (issuance/consumption half).
  Added `srv_cluster_join_tokens` (idempotent schema), a real `ClusterService`
  (`node_`+32 CSPRNG token, SHA-256 hash-at-rest, 8-char display prefix, 15-min
  TTL, single-use with atomic consume + 90-day reuse lockout, list, revoke),
  and rewired the three `/config/cluster` admin handlers to issue a token shown
  exactly once in a reveal box (never in a URL/redirect), list tokens, and
  revoke pending ones. The remote-DB cluster-mode conversion + node bootstrap
  handshake remains infra-blocked (needs a live remote PostgreSQL/MySQL and a
  second running node) — tracked below.
- Scheduler save/enable/disable/run-now (PART 19): DONE. `/config/scheduler`
  was a read-only task table; now has a running scheduler engine plus a full
  admin-action layer. Web POST `ConfigSchedulerAction` (enable/disable/run,
  PRG redirect with ?saved=/?err=) and API `APIConfigScheduler*` handlers
  (list, GET {id}, PATCH {id} `{"enabled":*bool}`, POST {id}/run, {id}/history)
  are wired. `enabled` is now server.db-authoritative so admin toggles survive
  restart (loadOrInitTaskState preserves the stored bit, forces non-skippable
  tasks on); SetTaskEnabled/RunNow refuse to disable the seven non-skippable
  critical tasks and reject already-running/unknown tasks with typed sentinels.
  Engine-level (scheduler/admin_test.go) and action-level
  (handler/admin_scheduler_test.go) tests added; both packages green in Docker.

## Deferred subsystem builds (each fronts an unbuilt/engine-less subsystem)

Confirmed by direct code search (2026-08). These are not stub-wires — each
needs a real service, schema, and/or engine before its admin page is more than
a facade, so each is a feature-sized build tracked here rather than faked:

- Cluster node bootstrap/join handshake (PART 34, second half): the join-token
  issuance/consumption layer now exists (see resolved list above), but the
  remote-DB cluster-mode conversion, config migration, and peer join handshake
  need a live remote PostgreSQL/MySQL and a second running node — infra-blocked.
- `/config/agents` admin page: fully unbuilt — no AgentService, schema, handler,
  or route; depends on the agent (`caslink-agent`) enrollment subsystem.
- OIDC/LDAP/SAML config sub-pages: DONE. Added `AuthConfig` (OIDC/LDAP/SAML
  provider structs, Normalize/Validate, AES-256-GCM secret encryption at
  rest, MaskedCopy for safe API/UI output) in `src/config/auth.go`; admin
  web pages + JSON API CRUD and a "Test connection" action (backed by
  mockable `extauth` boundaries) in `src/server/handler/admin_auth_providers.go`
  and `src/server/service/extauth/extauth.go`, routed under
  `/server/{admin_path}/config/security/auth/{oidc,ldap,saml}` (admin-session
  + CSRF) and `/api/{api_version}/server/{admin_path}/config/security/auth/{type}/providers[/{provider}][/test]`
  (Bearer admin token). `LDAPProvider.TLSVerify` is a tri-state `*bool`
  defaulting to verification-on so an omitted config field or unchecked
  admin-UI checkbox can never silently disable certificate verification.
  Full test suite (`go build`/`go vet`/`go test ./...`) and CI both green;
  committed as `834f0dee7481`. Full OIDC code exchange / LDAP bind / SAML
  assertion validation for the actual login flow remains a separate,
  not-yet-started feature.
- Admin config POST forms missing `_csrf` (PART 11/16): pre-existing bug found
  while building the scheduler action layer. The string-built admin config POST
  forms in `admin_config.go` (e.g. email/ssl/backup/security save forms) omit a
  `_csrf` hidden field, so if/when CSRF validation is enforced on admin state-
  changing requests they will be rejected. The new scheduler forms correctly
  include `_csrf`. Needs a sweep of every admin config form + confirmation of
  the CSRF middleware's enforcement posture on `/server/{admin_path}/config/*`
  before deciding token wiring vs middleware exemption — feature-sized, logged
  rather than fixed inline to avoid scope creep on the scheduler commit.

## Deferred from the 2026-08 extended PART 13-36 audit

These surfaced during the extended audit (tracker was AUDIT.AI.md, now
deleted). Each is feature-sized or needs verification unavailable in this
environment; all bounded security/logic/config findings from that pass were
fixed inline and committed separately.

- Frontend CSP externalization (PART 16): DONE. Every inline `on*` handler and
  every inline `<script>` block was moved into the embedded external
  `src/server/tmpl/static/js/app.js` (folded page modules: theme-set buttons,
  autosubmit, org slug autofill, dashboard create-link fetch, recovery-keys
  download/copy/confirm, passkey WebAuthn register). `base.html` now links
  `/static/css/app.css` and `/static/js/app.js` (previously linked neither — a
  pre-existing bug that left every page unstyled/non-interactive). CSP
  `script-src` tightened from `'self' 'unsafe-inline'` back to `'self'`;
  `style-src` keeps `'unsafe-inline'` per AI.md line 15621. Recovery keys now
  ship as a non-executable `<script type="application/json">` data island.
  The native `window.confirm` used by `form[data-confirm]` is now replaced by
  a shared native `<dialog>` confirm modal (app.js `askConfirm` +
  `.modal-confirm` styles in app.css), matching AI.md's canonical
  `confirmDelete` pattern (showModal/close/returnValue) with role="dialog",
  aria-modal, focus-return, Escape/backdrop, one-modal-at-a-time, and a
  no-<dialog>/no-JS graceful fallthrough (server still authorizes). This
  removes the last browser-default JS UI call; frontend-rules
  ("NEVER use alert()/confirm()/prompt()") is now satisfied. DONE.

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
  Domain-operation rate limiting is now implemented (PART 36): the add
  (POST .../domains → 10/hr per IP) and verify
  (POST .../domains/{domain}/verify → 15/hr per IP, all domains sharing one
  bucket) paths are wrapped with RateLimitMiddleware on all four surfaces
  (web + API, user + org). Verify is the primary abuse vector because each
  attempt triggers an outbound DNS TXT lookup; the shared per-IP-per-rule
  bucket prevents sidestepping the limit by cycling domain strings, and the
  domain cases are ordered ahead of the generic login/register/2fa substring
  cases so a domain literally named e.g. login.example.com is never
  misclassified into a stricter rule (middleware.go + middleware_test.go).
  DNS-01 ACME issuance is now implemented behind a mockable interface
  (src/server/service/acmedns: Issuer/DNSChallengeProvider interfaces,
  ACMEIssuer over golang.org/x/crypto/acme with zero new deps, mock/stub
  provider+issuer for tests). DomainService gained EnableDNS01SSL (wired in
  server.go when a 32-byte encryption_key is present), SetDNSProvider
  (validates the provider factory, JSON+AES-256-GCM-encrypts the DNS
  credentials into ssl_provider/ssl_credentials, ssl_challenge='dns-01',
  owner-scoped), and IssueDNS01Cert (owner-scoped force-issue/renew: requires
  verified+active, decrypts creds, runs the ACME order, AES-256-GCM-encrypts
  BOTH cert and key into ssl_cert_pem/ssl_key_pem, sets ssl_status='active'/
  ssl_enabled=1/ssl_issued_at/ssl_expires_at, clears ssl_last_error, purges
  the resolve cache, and calls the onCertChange hook). Failures record
  ssl_status='error'+ssl_last_error and map to SSL_PROVIDER_INVALID/
  SSL_CREDENTIALS_INVALID/SSL_CHALLENGE_FAILED/SSL_ISSUANCE_FAILED. Unit
  tests cover encryption-at-rest, owner isolation, wildcard SAN, and each
  error path (domain_ssl.go + domain_ssl_test.go).
  Slice C is now done: POST /{domain}/ssl and POST /{domain}/ssl/renew are
  registered (rate-limited) on all four domain route blocks (user, org,
  admin-user, admin-org). configureDomainSSL issues synchronously for dns-01
  (validate+encrypt+store provider creds, then IssueDNS01Cert) and reports
  status only for auto/http-01/tls-alpn-01 (autocert mints those on the first
  handshake); renewDomainSSL forces a dns-01 re-issue. SSLProviderRequest
  defaults Challenge to dns-01 when a Provider is given, else auto.
  CertificateFor is wired into the server TLS config's GetCertificate: it
  serves the DB-stored (decrypted, memoised) DNS-01 cert for matching SNI and
  returns nil to fall through to autocert; purgeCachedCert evicts the memo on
  every (re)issue. Handler + service tests cover all four endpoints, auth/role
  gating, and the handshake cert path (real self-signed cert, cache reuse,
  case/port normalisation, purge). End-to-end issuance against a live CA still
  needs a real DNS provider account/credential (INFRA-BLOCKED — e.g. a
  Cloudflare api_token) but the full code path is mock-tested.
  (Scheduled SSL renewal and HTTP-01/TLS-ALPN-01 issuance are already
  handled automatically by autocert.Manager — the ssl_renewal scheduler
  task is an intentional no-op.)

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
  "won't appear in public listings" is already satisfied. Org slug
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
  The invite-mode creation flow is now implemented (`InviteKindOrgCreation`,
  `OrganizationsConfig.OrgCreationInviteAllowed`, `hasValidCreationInvite`,
  invite token threaded through the web/API create paths and consumed single-use
  on success bound to the new owner) and the admin panel can issue/list/revoke
  org-creation invites (`ConfigOrgsInvites`/`ConfigOrgsInvitesAction`/
  `ConfigOrgsInvitesRevoke`, "Org Invites" nav, one-time `/orgs/new?invite=`
  link). admin_only admin-initiated creation is now implemented too:
  `ConfigOrgsCreate`/`ConfigOrgsCreateAction` (admin route `/config/orgs/create`,
  "Create Org" nav) resolve a chosen user by username/email via
  `AuthService.GetUserByIdentifier` and provision an org owned by that user,
  enforcing the owner's `max_per_user` limit but bypassing the creation-mode
  gate (admin always provisions). All four PART 35 creation modes
  (open/invite/admin_only/disabled) are now fully implemented and enforced.
  Organization creation-modes work complete.

- Cookie-consent banner (PART 12/16): DONE. Always-on GDPR/CCPA banner is now
  server-rendered from `server.privacy.consent` config, gated on the presence
  of the `cookie_consent` cookie (`newPageData` reads it; `base.html` renders
  `{{if not .HasConsentCookie}}`). Accept/Decline are plain POST forms to
  `POST /server/consent` (`PagesHandler.Consent`) that store the
  `{essential,preferences,analytics,timestamp}` state as URL-escaped JSON in
  the `cookie_consent` cookie (Path=/, MaxAge 1y, SameSite=Lax, Secure when
  TLS) and redirect back to a validated same-origin path (open-redirect
  rejected). Works fully without JS; app.js adds a background-fetch + remove
  enhancement. Data-sold message swap via `PrivacyConfig.GetConsentMessage`.
  Handler + newPageData unit tests added and green. Also fixed a pre-existing
  frontend-rules "NEVER inline CSS" violation on the nav language selector
  (moved inline `style=` to `.lang-form`/`.lang-select` classes).

- Admin panel WebUI completeness (PART 17): the full set of admin config
  templates is a UI feature build needing browser verification. Deferred
  (browser-verification-gated).
