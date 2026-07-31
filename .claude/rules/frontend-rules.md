# Frontend Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER build a frontend that only wraps the API without full browser functionality — ALL features MUST work in browser (PART 16)
- NEVER use JS frameworks (React/Vue/etc.) — pure vanilla JS only
- NEVER use inline CSS or inline `<script>`/`on*` attributes — CSP blocks them; external files only
- NEVER use default browser JS UI (`alert()`, `confirm()`, `prompt()`) — use custom modals/toasts
- NEVER let the site break with JavaScript disabled — JS enhances, never enables core functionality
- NEVER let long unbreakable strings (IPv6, .onion, tokens, hashes, UUIDs) overflow their container — always `word-break: break-all` / `overflow-wrap: break-word` or horizontal scroll
- NEVER use desktop-first CSS (`max-width` media queries) — mobile-first only (base = mobile, `min-width` queries scale up)
- NEVER pass user-controlled/untrusted content through `template.HTML` unless it went through an approved sanitizer (repo blobs, pasted text, markdown, uploads are data, not templates)
- NEVER show blank space for empty lists/tables/data views — every one MUST have a proper empty state
- NEVER store session tokens in localStorage or IndexedDB — session lives in `HttpOnly` + `Secure` + `SameSite` cookie only, never touched by JS
- NEVER use a toast for something requiring a decision/input ("Are you sure?", password entry) — use a modal; never use a modal for simple confirmations ("Saved") — use a toast
- NEVER stack more than one modal at a time — queue instead
- NEVER show unstyled/plain error pages — 404/500/502/503 etc. MUST use the site theme system (`error.tmpl`)
- NEVER let a page template skip header/nav/footer partials — every page MUST include them, no page defines its own
- NEVER embed security-sensitive data needing frequent updates in the binary (e.g. blocklists) — only templates/static assets are embedded
- NEVER inject analytics/tracking scripts or send data to trackers — none, ever
- NEVER use generic placeholder content on standard pages (About, Privacy, Terms, etc.) — content MUST come from IDEA.md
- NEVER render `server.contact.admin.email` on the public contact/report page — it is never public
- **Admin panel (PART 17):**
  - NEVER link to `/server/{admin_path}` from ANY public route — no mentions anywhere on `/**`
  - NEVER add direct children under `/server/{admin_path}/` other than `{admin_username}` and `config`
  - NEVER put server management routes at admin root — e.g. `/server/{admin_path}/settings` is WRONG, use `/server/{admin_path}/config/settings`
  - NEVER bypass admin auth for "quick testing" — `--debug` adds verbosity only, never bypasses auth
  - NEVER let admin session and user session share state — fully separate sessions/cookies

## CRITICAL - ALWAYS DO

- ALWAYS build mobile-first: base CSS = mobile, `@media (min-width: 768px/1024px/1280px)` to scale up
- ALWAYS normalize URLs first in the middleware chain (`URLNormalizeMiddleware`) — strip trailing slash except root `/` and explicit file paths, 301 redirect to canonical
- ALWAYS detect client type (browser/CLI/curl/Accept header) and respond HTML vs text vs JSON accordingly for every frontend route
- ALWAYS support full CRUD via HTML forms (browser), JSON API (`/api/{api_version}/...`), and form-encoded/text frontend routes (CLI/scripting)
- ALWAYS disable the submit button immediately on click and show an in-progress label (Saving…, Submitting…), re-enabling on success OR error; preserve button width
- ALWAYS give copy buttons a visible "Copied!" confirmation (checkmark + i18n label `copied`, 2s revert, `aria-live="polite"`)
- ALWAYS use the POST-redirect-GET pattern with a one-shot flash message for non-AJAX form POSTs (no-JS fallback) — toasts are the JS enhancement layered on top, never the only feedback channel
- ALWAYS end non-HTML responses with a single trailing newline (`\n`)
- ALWAYS use Go `html/template` for all HTML
- ALWAYS support both light and dark themes with seamless switching (no reload), WCAG AA contrast (4.5:1 min) in both, visible focus indicators, working keyboard nav and screen reader support in both
- ALWAYS keep the user icon in the header (never in a menu) and the footer at the bottom of the page (never floating)
- ALWAYS serve a dynamically generated `/sitemap.xml`, excluding admin, auth, and API routes
- ALWAYS sanitize `custom_html` (footer, announcements) before rendering — scripts/dangerous elements stripped, never executed
- ALWAYS show the cookie consent banner (fixed bottom, GDPR/privacy) — it is always enabled since sessions/preferences use cookies
- **Admin panel (PART 17):**
  - ALWAYS put every server-management route under `/server/{admin_path}/config/*`
  - ALWAYS keep `/server/{admin_path}/{admin_username}/*` scoped to the admin's own account/profile/preferences/notifications only
  - ALWAYS test admin auth properly in automated tests: unauth requests blocked → setup token creates admin → login works → session works → invalid creds rejected
  - ALWAYS support TOTP and Passkeys for every server admin
  - ALWAYS validate the admin panel's own admin path / security-sensitive fields on save and show inline errors for invalid formats

## Key Rules

### Frontend route structure (mirrors API)

| API Route | Frontend Route | Page Type |
|---|---|---|
| `GET /api/{v}/users` | `GET /users` | Current user profile |
| `PATCH /api/{v}/users` | `POST /users` | Profile update form |
| `GET /api/{v}/users/tokens` | `GET /users/tokens` | Token list |
| `GET /api/{v}/users/settings` | `GET /users/settings` | Settings page |
| `GET /api/{v}/users/security` | `GET /users/security` | Security settings |
| `GET /api/{v}/orgs` | `GET /orgs` | User's org list |
| `GET /api/{v}/server/about` | `GET /server/about` | About page |

Vanity URLs (`/{username}`, `/{org_name}`) are OPTIONAL — only if user/org profiles are core (requires PART 34/35); registered last, after all explicit routes; reserved-name list must block registration of system/common/technical words.

Route priority: `/api/*` → `/server/{admin_path}/*` → `/server/healthz` → `/static/*` → `/users/*` → `/orgs/*` → reserved names → `/{username}` → `/{org_name}`.

### Client-type detection order

1. `Accept` header (`text/html` / `text/plain` / `application/json`)
2. Our own CLI client → JSON
3. Text-mode browser (lynx/w3m/links) → HTML (no-JS alternative)
4. Browser User-Agent → HTML
5. CLI tool User-Agent (curl/wget/httpie/…) → text
6. Empty UA → text
7. Default → HTML

### Breakpoints

| Breakpoint | Target |
|---|---|
| base (no query) | mobile <768px |
| `min-width: 768px` | tablet+ |
| `min-width: 1024px` | desktop+ |
| `min-width: 1280px` | large desktop (optional) |

### Toast vs Modal

| Use TOAST | Use MODAL |
|---|---|
| Confirmation ("Saved", "Deleted", "Copied") | Requires decision ("Delete this?") |
| Non-blocking info | Requires input (forms, passwords) |
| Transient feedback ("Loading…") | Destructive-action confirmation |
| Errors not needing input | Blocking workflow (login, terms) |

Toast rules: top-right, stack newest-on-top, max 5 visible, 3s auto-dismiss (errors: none, warnings: 5s), click/Escape to dismiss, pause on hover.

### Semantic HTML / components

- `<code>` inline values, `<pre><code>` blocks, `<kbd>` key input, `<samp>` output, `<var>` placeholders, `<time>` dates
- Copy buttons required for: .onion addresses, API tokens, node URLs, git clone URLs. Not needed for: versions, usernames, booleans
- Tables: wrap in `.table-wrapper` with `overflow-x: auto`, `min-width` forces scroll on mobile
- Native `<dialog>` for modals — handles focus trap/Escape/backdrop with zero JS; `<form method="dialog">` to close

### PWA

Must be installable + offline-capable: service worker, cache versioning, update notification, install prompt, manifest with maskable icons, iOS considerations, pass Lighthouse PWA audit.

### CORS / CSRF / Layout

- CORS allow-list resolution: config → DOMAIN → proxy-learned; `*` only as last-resort fallback
- CSRF: cookie posture is first line of defense; validated on state-changing requests
- Layout split: `public.tmpl` (public site) vs `admin.tmpl` (admin panel) — separate layouts, shared theme CSS variables
- Static assets and templates embedded in the binary (except frequently-updated security data like blocklists)

### Admin panel structure (PART 17)

Two user types only inside `/server/{admin_path}`: **Admin** (own routes) and everyone else blocked. Admin credentials live in `users.db` (admins table), never in the config file.

Route hierarchy:

```
/server/{admin_path}/                         → dashboard only
/server/{admin_path}/{admin_username}/*       → admin's own profile/preferences/notifications
/server/{admin_path}/config/*                 → ALL server management
```

Key `config/*` subroutes: `setup`, `settings`, `ssl`, `email`, `scheduler`, `logs`, `logs/audit`, `backup`, `updates`, `info`, `metrics`, `network/tor`, `network/geoip`, `security/auth[/oidc|ldap|saml]`, `security/tokens`, `security/firewall`, `users/`, `orgs/`, `cluster/`, `agents/`.

Settings page sections: General, Process, Branding, SEO, Security, Account Lockout, IP Blocking, SSL/TLS, Authentication, Backup, Email/SMTP, Notifications, Scheduler, URL Detection, Tor (if installed), GeoIP, Blocklists.

Admin panel is required for every project (`caslink`/Caslink included) — no exceptions, no feature gating.

For complete details, see AI.md PART 16, 17
