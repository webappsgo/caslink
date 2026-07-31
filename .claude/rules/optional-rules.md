# Optional Features Rules (PART 34, 35, 36)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Activation status for this project

**PART 34 (Multi-User), PART 35 (Organizations), and PART 36 (Custom Domains) have been formally flipped to REQUIRED for caslink, per the OPTIONAL to REQUIRED flip mechanism documented in AI.md (around line 54780).**

- This project's `AI.md` PART 34/35/36 headings now read `(REQUIRED - NON-NEGOTIABLE)`, not `(OPTIONAL - NON-NEGOTIABLE WHEN IMPLEMENTED)` — the one sanctioned per-project edit to the otherwise read-only AI.md.
- `IDEA.md ## Project variables` sets `multi_user: true`, `organizations: true`, `custom_domains: true` to match.
- `IDEA.md`'s "Business logic to Features" section already describes **Multi-user accounts**, **Organizations** (up to 5/user, roles owner/admin/member), and **Custom domains** (up to 5/user, 20/org, DNS TXT verification, Let's Encrypt) as shipped Caslink features, and the git history confirms all three are implemented.
- **Conclusion: treat PARTs 34-36 as fully mandatory for caslink; every requirement, table, and rule in those PARTs applies in full.** Flipping is one-way; do not un-flip without a data migration (see AI.md flip-mechanism rules).

## CRITICAL - NEVER DO

- Never implement organizations before multi-user (PART 35 explicitly requires PART 34 first)
- Never confuse `server.orgs.creation.mode` (server-level: who can create an org) with an org's own `visibility: public/private` (org-level: who can see it)
- Never let a Server Admin (PART 17) set a regular user's password directly — only the user sets it, via invite/activation link or reset flow
- Never treat registration `mode` as controlling existing-user login or profile visibility — it only gates NEW account creation
- Never skip TXT record ownership verification before issuing SSL for a custom domain
- Never allow apex/subdomain/wildcard custom domains that match `reserved` entries (`localhost`, `*.local`, `*.test`, `*.example`, `*.invalid`) or `blocked_patterns` (gov/mil/edu TLDs)
- Never implement custom domains partially — the full PART 36 checklist is mandatory
- Never let `disabled` registration/org-creation mode reject existing invite/activation links silently without also blocking new ones — explicitly reject unused links once mode changes to `disabled`

## CRITICAL - ALWAYS DO

- Always gate multi-user behind `server.users.enabled` (default `false` = admin-only) and organizations/custom-domains behind their own `enabled` flags
- Always default registration mode to `open` when multi-user is enabled
- Always respect `auto_register` and username-collision rules for OIDC/LDAP/SAML-backed users on first login (counts as new account creation)
- Always use `org`/`orgs` in routes, config, and DB regardless of user-facing label (team/workspace/group)
- Always enforce `max_domains_per_user` (5) and `max_domains_per_org` (20) per IDEA.md's stated limits
- Always renew custom-domain SSL certs `ssl_renewal_days` (7) before expiry via scheduled task
- Always encrypt stored DNS provider API tokens/credentials for custom domain SSL challenges

## Key Rules

### Multi-user (PART 34)

| Registration mode | Public register | Admin invite | Direct admin create | Default |
|---|---|---|---|---|
| `open` | Yes | Optional | Optional | **Yes** |
| `invite` | No | Required | No | No |
| `admin_only` | No | No | Required | No |
| `disabled` | No | No | No | No |

Roles: `admin` (full access), `user` (own profile/tokens); custom roles definable via `server.users.roles.permissions`. Invite links: single-use by default, 7-day expiry, only Server Admin issues them.

### Organizations (PART 35)

Needed when users collaborate as teams with shared resources/billing (per IDEA.md: up to 5 orgs/user). Creation modes mirror user registration: `open` (default, any authenticated user), `invite`, `admin_only`, `disabled`. Roles: owner, admin, member. `server.orgs.creation.mode` is a server-wide policy distinct from per-org `visibility`.

### Custom domains (PART 36)

| Setting | Value (per IDEA.md) |
|---|---|
| `max_domains_per_user` | 5 |
| `max_domains_per_org` | 20 |
| `require_ssl` | true |
| Verification | DNS TXT, exponential-backoff retry, `verification_ttl: 24h` |
| SSL | Let's Encrypt — HTTP-01, TLS-ALPN-01, or DNS-01 |
| `ssl_renewal_days` | 7 |

Implementation checklist (all required once active): feature flag, `custom_domains`/`custom_domain_audit` tables, domain resolver middleware, user + org web/API routes, admin routes, TXT verification, ACME SSL issuance, scheduled verification-retry/SSL-renewal/cleanup tasks, domain caching, rate limiting, WebUI pages.

For complete details, see AI.md PART 34, 35, 36
