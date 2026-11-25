# Implementation Backlog

These are project implementation items required for AI.md alignment. AI.md itself remains read-only; any AI.md defects belong in the parent SERVER template inbox.

- [x] Apply scheduled `update.defer_days` to GitHub release `published_at` dates; manual update checks bypass the delay.
- [x] Schedule renewal for DNS-01 custom-domain certificates before expiry, including configured renewal window, retries, status updates, and admin visibility.
- [ ] Implement the configured OIDC browser sign-in flow with state, nonce, PKCE, provider discovery, identity mapping, and first-login handling.
- [ ] Complete PGP key generation, encrypted private-key storage, rotation/retirement, public-key publication, and persisted keyserver retries.
- [ ] Implement outbound webhook delivery with signing, retry/backoff, and delivery status; preserve disabled-by-default behavior.
- [x] Render admin configuration pages through the shared layout with localized navigation and content.
- [ ] Add required PDF export support to organization audit exports and keep JSON, CSV, and PDF snapshot metadata/hash behavior consistent.
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
- [ ] Complete security-report PGP keypair generation, encrypted private-key storage, rotation, fingerprint/public-key publication, and keyserver retry persistence; the public key endpoint currently remains unavailable.
- [ ] Complete the PWA requirements: root-scope service-worker registration and safe update/offline lifecycle, valid install icons and metadata, and install/update prompts that match AI.md.
- [ ] Consolidate administrator and regular-user password login at `/server/auth/login`, remove the separate admin login route, and include admin MFA in the shared flow as required by AI.md.
- [ ] Implement the AI.md API login contract: issue scoped 24-hour bearer tokens, and use hashed, single-use five-minute partial-session values for MFA continuations instead of returning session IDs.
