# Caslink — Open Tasks

Tasks the audit identified as still pending. Items removed when the underlying
code lands; items added when a new gap is found.

## Authentication hardening

- [ ] "Remember this device" cookie for 2FA
- [ ] WebAuthn / passkey registration + login (DB row read path is stubbed)
- [ ] OAuth2/OIDC social login (Google, GitHub, etc.)

## Organizations

- [ ] Org invite + accept flow (`/orgs/{slug}/members/invite`)
- [ ] Role management UI (`/orgs/{slug}/roles`)
- [ ] Org security / audit log pages and CSV export
- [ ] Ownership transfer + delete-with-confirmation flow
- [ ] URL handlers respect org scope (org-owned links, org token auth)
- [ ] Org-scoped tokens (`/orgs/{slug}/tokens`)

## Custom domains (PART 35)

- [ ] DNS-TXT verification logic (service stub currently flips the row to verified)
- [ ] Detect server public IP for A-record instructions
- [ ] Exponential backoff retry loop for verification
- [ ] DNS instructions page (user + org)
- [ ] Let's Encrypt HTTP-01 challenge
- [ ] Let's Encrypt DNS-01 challenge
- [ ] Store certs encrypted in DB (cert/key columns exist; encryption is missing)
- [ ] SSL renewal scheduler task
- [ ] SSL status page
- [ ] Wire `POST /orgs/{slug}/domains/add` to the domain service (currently returns 501)

## Admin moderation (PART 23)

- [ ] `/server/{adminPath}/moderation/users` list + detail + suspend/unsuspend
- [ ] `/server/{adminPath}/moderation/orgs` list + detail + suspend
- [ ] `/server/{adminPath}/domains` list + suspend
- [ ] `/server/{adminPath}/settings` runtime config overrides
- [ ] Username blocklist enforcement

## Analytics, QR, bulk, billing, federation, notifications

- [ ] Analytics aggregation, GeoIP enrichment, bot exclusion, per-link reports
- [ ] QR SVG + PDF output (only PNG works; SVG falls through to PNG)
- [ ] Bulk CSV/JSON import + export
- [ ] Billing (Stripe / Paddle / PayPal / LemonSqueezy / Manual) — disabled by default
- [ ] Federation server + client (signed messages, `/.well-known/caslink`)
- [ ] Notifications: SMTP works, add SendGrid / SES / SMS / Push providers

## Database

- [ ] Numbered migration runner (`001…N`) in `src/server/store` — current code is `CREATE TABLE IF NOT EXISTS` only
- [ ] Cross-dialect schema: SQLite uses `AUTOINCREMENT`/`DATETIME`; postgres/mysql/sqlserver need `SERIAL`/`BIGSERIAL`/`IDENTITY`, `TIMESTAMP`, and `$N`/`?` placeholders

## Documentation

- [ ] `docs/admin.md` — user/org moderation guide
- [ ] `docs/configuration.md` — full `server.yml` reference
- [ ] `docs/api.md` — REST + GraphQL endpoint reference
- [ ] `docs/custom-domains.md` — DNS + SSL setup walkthrough

## Tests

- [ ] Unit tests for username/email validation, short-code generation, Argon2id round-trip
- [ ] Integration test for register → login → create URL → redirect → click recorded
- [ ] Integration test for password reset flow
- [ ] Integration test for organization create + member join
