# Implementation Backlog

These are project implementation items required for AI.md alignment. AI.md itself remains read-only; any AI.md defects belong in the parent SERVER template inbox.

- [x] Apply scheduled `update.defer_days` to GitHub release `published_at` dates; manual update checks bypass the delay.
- [x] Schedule renewal for DNS-01 custom-domain certificates before expiry, including configured renewal window, retries, status updates, and admin visibility.
- [x] Implement the configured OIDC browser sign-in flow with state, nonce, PKCE, provider discovery, identity mapping, and first-login handling.
- [x] Complete PGP key generation, encrypted private-key storage, rotation/retirement, public-key publication, and persisted keyserver retries.
- [x] Implement outbound webhook delivery with signing, retry/backoff, and delivery status; preserve disabled-by-default behavior.
- [x] Render admin configuration pages through the shared layout with localized navigation and content.
- [x] Add required PDF export support to organization audit exports and keep JSON, CSV, and PDF snapshot metadata/hash behavior consistent.
- [x] Add Server Admin self-profile username updates through the authenticated profile page and API.
- [ ] Complete cluster peer persistence, registration/join, heartbeat, liveness, admin status, and distributed scheduler behavior.
- [x] Implement PART 32.2 I2P eepsite providers, persisted destination identity, container-aware default, dedicated backend listener, health task, and admin/status integration.
- [ ] Resolve and implement the IDEA.md billing capability: IDEA describes operator billing, plans, invoicing, dunning, and usage tracking, but AI.md has no billing part or implementation contract and SPEC.md activates only PARTS 34–36.
- [ ] Resolve and implement the IDEA.md federation capability: IDEA describes signed peer announcements, paginated sync, and discovery, but AI.md has no federation part or implementation contract and SPEC.md activates only PARTS 34–36.
- [ ] Complete remaining Admin self-service: secure email/password changes, token rotation, TOTP/passkey enrollment, recovery codes, and per-event notification preferences using admin-specific verification and persistence flows.
- [ ] Apply valid admin-path changes with config persistence and a graceful process reload; current handlers reject changes because the router cannot be rebuilt safely.
- [ ] Complete first-run setup across the browser and API: follow all six setup endpoints, support 2FA and HTTPS/certificate settings, preserve the no-JS path, and keep `docs/installation.md` aligned.
- [ ] Complete the contact report flow required by PART 17: abuse/DMCA/privacy type selection, field allowlists and limits, anti-spam token/review steps, fallback recipient resolution, and matching API behavior.
- [ ] Implement the project help content (`server.pages.help.content`) from IDEA.md and expose it through the Help page and API rather than the generic translation-only help content.
- [ ] Add admin controls for privacy deployment settings and sanitized page-content extensions (About, Privacy, Terms, Help, Contact, and shared branding content) as required by PART 17; reconcile the built-in privacy editor ambiguity in TODO.SERVER before making those text fields editable.
- [x] Complete security-report PGP keypair generation, encrypted private-key storage, rotation, fingerprint/public-key publication, and keyserver retry persistence.
- [x] Complete the PWA requirements: base-path-aware service-worker registration and safe update/offline lifecycle, localized install/offline prompts, valid install icons and metadata.
- [ ] Consolidate administrator and regular-user password login at `/server/auth/login`, remove the separate admin login route, and include admin MFA in the shared flow as required by AI.md.
- [ ] Implement the AI.md API login contract: issue scoped 24-hour bearer tokens, and use hashed, single-use five-minute partial-session values for MFA continuations instead of returning session IDs.
- [ ] Implement AI.md URL/FQDN resolution: validate domains against mode and public suffix, enforce the DOMAIN allowlist, detect trusted-proxy hosts safely, learn subdomain/wildcard patterns with the configured sample window, update CORS and URL output at request time, and keep unresolved FQDN empty.
- [ ] Implement AI.md configuration area files and reload: merge validated `yaml.d/*.yml` with `server.yml` and `secrets.yml`, persist each setting to its owning file, watch and reload changes safely, and integrate shared settings synchronization with cluster mode.
- [ ] Localize every server and admin page, API error/status message, and user-visible configuration default through translation keys in all required locales, including newly added cluster, PGP, lockout, and self-service pages.
