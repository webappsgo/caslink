# Implementation Backlog

These are project implementation items required for AI.md alignment. AI.md itself remains read-only; any AI.md defects belong in the parent SERVER template inbox.

- [ ] Persist `update.defer_days` first-seen state in `server.db` and apply it across restarts; keep manual update checks immediate.
- [ ] Schedule renewal for DNS-01 custom-domain certificates before expiry, including configured renewal window, retries, status updates, and admin visibility.
- [ ] Implement the configured OIDC browser sign-in flow with state, nonce, PKCE, provider discovery, identity mapping, and first-login handling.
- [ ] Complete PGP key generation, encrypted private-key storage, rotation/retirement, public-key publication, and persisted keyserver retries.
- [ ] Implement outbound webhook delivery with signing, retry/backoff, and delivery status; preserve disabled-by-default behavior.
- [ ] Complete the admin configuration pages' shared-layout rendering and localization, including PDF export where required by the API contracts.
- [ ] Implement the Server Admin's own profile, preferences, and notifications routes under `{admin_username}` and their API equivalents.
- [ ] Complete cluster peer persistence, registration/join, heartbeat, liveness, admin status, and distributed scheduler behavior.
- [ ] Implement PART 32.2 I2P eepsite providers, persisted destination identity, container-aware default, dedicated backend listener, health task, and admin/status integration.
- [ ] Resolve and implement the IDEA.md billing capability: IDEA describes operator billing, plans, invoicing, dunning, and usage tracking, but AI.md has no billing part or implementation contract and SPEC.md activates only PARTS 34–36.
- [ ] Resolve and implement the IDEA.md federation capability: IDEA describes signed peer announcements, paginated sync, and discovery, but AI.md has no federation part or implementation contract and SPEC.md activates only PARTS 34–36.
- [ ] Complete Admin self-service APIs for profile updates, password changes, token rotation, TOTP/passkey enrollment, recovery codes, and notifications using admin-specific verification and persistence flows.
- [ ] Apply valid admin-path changes with config persistence and a graceful process reload; current handlers reject changes because the router cannot be rebuilt safely.
- [ ] Make the browser first-run wizard follow the API setup sequence (token verify, account, token, config, security, services, completion) and keep `docs/installation.md` aligned with the implemented flow.
