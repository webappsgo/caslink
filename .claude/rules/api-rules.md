# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER expose sensitive data in `/server/healthz` or any health route (`/server/healthz`, `/api/{api_version}/server/healthz`, `/api/healthz`): credentials, DB connection strings/host/port/user, internal IPs, file paths, usernames/user counts with names, SMTP host/admin emails, encryption/session secrets, stack traces, `/metrics` status, internal service endpoints
- NEVER return raw connection details for `checks.database`/`checks.cache` — only `"ok"` or `"error"`
- NEVER add sub-routes to `/server/healthz` (no `/server/healthz/db`, no `/server/healthz/**`)
- NEVER hardcode `v1` — always use `APIBasePath()` / `{api_version}`
- NEVER use singular resource names (`/api/{api_version}/user` is wrong; use `/users`)
- NEVER use uppercase or underscores in routes (`Users`, `api_keys` are wrong; use `users`, `api-keys`)
- NEVER end a route with a trailing slash
- NEVER use verbs in routes (`getUsers` wrong; use `GET /users`)
- NEVER keep legacy (removed/changed) endpoints — delete them completely, no redirects, no deprecation shims, no backwards-compat layers
- NEVER layer new routes on top of old code when route rules change — migrate the implementation, delete superseded routes
- NEVER redirect an unversioned `/api/<thing>` alias to its versioned counterpart — mount the SAME handler at both paths, serve directly
- NEVER manually edit `openapi.json` or GraphQL schema files — both are generated at build time from code
- NEVER let Swagger/GraphQL drift from the actual API or from each other
- NEVER put swagger/graphql files outside `src/swagger/` and `src/graphql/` (never in project root)
- NEVER use YAML for the OpenAPI spec — JSON only, no `.yaml`/`.json` suffix routes
- NEVER do client-side routing (SPA), client-side rendering (React/Vue) for initial load, business logic in JavaScript, or require JavaScript for core features
- NEVER use the legacy/forbidden error response shapes: human text in `error` field with code in separate `code` field, `status` duplicated in body, bare `{"error": "..."}` with no `ok`, ad-hoc top-level fields like `reason`/`retry_after`/`self_healing` (use `details` or HTTP headers)
- NEVER implement hundreds of external-API routes for "compatibility" unless the user explicitly asked for route/API/client compatibility — default is feature compatibility only
- NEVER guess RFC behavior — if the app implements an RFC-defined protocol (DNS, DHCP, SMTP, HTTP, FTP, NTP, LDAP, WebDAV, etc.) it MUST be FULLY RFC-compliant, not partially
- NEVER use icons/ASCII art/colors in log files or log stdout output — logs are ALWAYS raw text
- NEVER auto-renew certs under `/etc/letsencrypt/live/**` (system/certbot owns those) or under `{config_dir}/ssl/local/{fqdn}/` (user-managed, manual only)
- NEVER set `DOMAIN` to an overlay address (`.onion`, `.i2p`, `.exit`) — these are app-generated
- NEVER show the setup token more than once

## CRITICAL - ALWAYS DO

- ALWAYS version API routes: `/api/{api_version}/...`
- ALWAYS use plural nouns for resources: `/users`, `/orgs`, not `/user`, `/org`
- ALWAYS use lowercase, hyphen-separated multi-word routes: `/api-keys`
- ALWAYS prefer path params for resource identity, query params for filtering/sorting/pagination
- ALWAYS make frontend routes fully functional (forms work, CRUD complete, error handling, validation, state sync, no dead ends) and working WITHOUT JavaScript (progressive enhancement)
- ALWAYS do validation, business logic, formatting, pagination/sorting, search/filtering, HTML rendering, and state management on the SERVER — client JS is enhancement only
- ALWAYS keep Swagger and GraphQL in sync with each other and with the live API, regenerated at build time
- ALWAYS theme Swagger UI and GraphiQL to match the project-wide light/dark/auto theme system
- ALWAYS accept an auth token from any PART 8 header or `?token=` query param, first-found-wins, for auth-protected API endpoints
- ALWAYS use the canonical error shape: `{"ok": false, "error": "CODE", "message": "...", "details": {...}}` — HTTP status carries the status
- ALWAYS use the canonical success/action shape: `{"ok": true, "data": {...}}`
- ALWAYS paginate with default limit 250: `{"data": [...], "pagination": {"page", "limit", "total", "pages"}}`
- ALWAYS end every response/file (JSON, TXT, HTML, XML, YAML, Go, CSS, JS) with exactly one trailing newline
- ALWAYS use 2-space indent for HTML/JSON/YAML/CSS/JS; tabs for Go and Makefiles
- ALWAYS research the target service's actual API docs before implementing external compatibility — never guess
- ALWAYS default external "compatible with X" requests to feature/behavior compatibility using our own routes, unless the user explicitly asked for route/API/client compatibility
- ALWAYS implement full RFC compliance when building a protocol server (research ALL relevant RFCs, document which are implemented in AI.md)
- ALWAYS support all three Let's Encrypt challenge types (HTTP-01, TLS-ALPN-01, DNS-01) and all lego-supported DNS providers via dynamic admin dropdown
- ALWAYS encrypt (AES-256-GCM) stored DNS provider credentials
- ALWAYS resolve FQDN via the documented priority order (reverse proxy headers → DOMAIN env → os.Hostname() → $HOSTNAME → public IPv6 → public IPv4 → localhost)
- ALWAYS strip `:80` and `:443` from displayed URLs
- ALWAYS check certificate lookup order on startup: `/etc/letsencrypt/live/domain/` → `/etc/letsencrypt/live/{fqdn}/` → `{config_dir}/ssl/letsencrypt/{fqdn}/` → `{config_dir}/ssl/local/{fqdn}/` → request new
- ALWAYS auto-renew app-managed Let's Encrypt certs (`{config_dir}/ssl/letsencrypt/{fqdn}/`) 7 days before expiry, checked daily at 03:00
- ALWAYS adapt the startup banner to terminal width (≥80 full, 60-79 compact, 40-59 minimal, <40 micro, NO_COLOR/TERM=dumb plain)

## Key Rules

### Health & Versioning (PART 13)

**Routes:**
| Route | Format |
|---|---|
| `/server/healthz` | HTML/text via content negotiation (PART 14) |
| `/healthz` | Optional root alias, only if `server.healthz.root.enabled: true`, mounts same handler, no redirect |
| `/api/{api_version}/server/healthz` | JSON default, text via API rules |
| `/api/healthz` | Unversioned direct alias, JSON |

**HealthResponse field order (backend struct = frontend display order):**
1. `project` (name/tagline/description from branding, PART 16)
2. `status` (healthy/unhealthy/degraded) + `pending_restart`/`restart_reason`
3. `version`, `go_version`, `build.{commit,date}`
4. `uptime`, `mode`, `timestamp`
5. `cluster.{enabled,status,primary,nodes,node_count,role}`
6. `features` — PUBLIC/non-negotiable only (Tor, GeoIP; optional PARTs 34/35/36 if used by project)
7. `checks.{database,cache,disk,scheduler,cluster,tor}` — ok/error only, no details
8. `stats.{requests_total,requests_24h,active_connections}` — aggregate, public-safe only
9. App-specific fields (Caslink extends via IDEA.md)

**Versioning (SemVer):** MAJOR.MINOR.PATCH starting at `1.0.0` (never `0.x.x`); no `v` prefix in version string (git tags do get `v` prefix); pre-release suffix `-rc1`/`-alpha`; beta format `YYYYMMDDHHMMSS-beta`; daily format `YYYYMMDDHHMMSS`. Version source priority: `release.txt` → git tag → `dev`.

### API Structure (PART 14)

**Route scopes:**
| Scope | Web | API | ID? |
|---|---|---|---|
| Server | `/server/*` | `/api/{v}/server/*` | No |
| Auth | `/server/auth/*` | `/api/{v}/server/auth/*` | No |
| Users | `/users/*` | `/api/{v}/users/*` | No (session) |
| Orgs | `/orgs/*` | `/api/{v}/orgs/*` | Yes (`{slug}`) |
| Server Admin | `/server/{admin_path}/*` | `/api/{v}/server/{admin_path}/*` | No |
| Admin Self | `/server/{admin_path}/{admin_username}/*` | same | Yes |
| Admin Config | `/server/{admin_path}/config/*` | same | No |
| Project (Caslink) | `/*` | `/api/{v}/*` | Varies (see IDEA.md) |

**Content negotiation priority (API `/api/{v}/*`):** `.txt` ext → `Accept: application/json` → `Accept: text/plain` → non-interactive client → default JSON.
**Content negotiation priority (frontend `/**`):** `Accept: text/html` → `Accept: text/plain` → User-Agent browser detection → CLI/curl → default HTML.

**Client types:** our CLI (`{project_name}-cli/` UA) → interactive, JSON. Text browsers (lynx/w3m/links/elinks/browsh/carbonyl/netsurf) → interactive, no-JS HTML. HTTP tools (curl/wget/httpie/no UA) → non-interactive, `HTML2TextConverter()` formatted text (80 col).

**API types required (all 3):** REST, Swagger (`src/swagger/`), GraphQL (`src/graphql/`) — always in sync, JSON-only OpenAPI, generated at build time.

**Root-level endpoints:**
| Endpoint | Auth |
|---|---|
| `/` | None |
| `/server/healthz` | None |
| `/server/docs/swagger`, `/server/docs/graphql` | None |
| `/metrics` | Optional |
| `/server/{admin_path}[/*]` | Session |
| `/api/autodiscover`, `/api/swagger`, `/api/graphql`, `/api/healthz` | None (unversioned aliases) |
| `/api/{api_version}/server/swagger`, `/graphql`, `/healthz` | None |
| `/api/{api_version}/server/{admin_path}/*` | Bearer |

**Unversioned alias criteria:** add when operationally useful before a version is picked and contract is stable/documented (swagger, graphql, healthz, debug); skip when data-shaped/version-specific (resources, business logic).

**Response shapes:** single item = bare object; action = `{ok:true, data:{...}}`; error = `{ok:false, error:"CODE", message:"...", details:{...}}`; list = `{data:[...], pagination:{page,limit,total,pages}}` (default limit 250).

**Port config:** single port → HTTP (except 443 → HTTPS-only); dual ports → first HTTP, second HTTPS; `ssl.enabled` config can override.

### SSL/TLS & Let's Encrypt (PART 15)

**Challenge types:** HTTP-01 (port 80, default), TLS-ALPN-01 (port 443), DNS-01 (wildcard, no port req).

**DNS-01 setup:** admin WebUI `/server/{admin_path}/config/ssl` → select provider (all lego providers) → dynamic credential form → validate via API call → store AES-256-GCM encrypted.

**Cert directory structure (mirrors certbot):**
```
{config_dir}/ssl/letsencrypt/{fqdn}/{fullchain.pem,privkey.pem}   # app-managed, auto-renew
{config_dir}/ssl/local/{fqdn}/{cert.pem,key.pem}                  # user-managed, manual only
```

**Ownership:** `/etc/letsencrypt/live/**` = system/certbot (app never renews); `{config_dir}/ssl/letsencrypt/{fqdn}/` = app auto-renews 7 days before expiry, checked daily 03:00; `{config_dir}/ssl/local/{fqdn}/` = never auto-renewed.

**Overlay networks (Tor/I2P):** default HTTP (encryption at network layer); switch to HTTPS only when clearnet is HTTPS-only (port 443); require self-signed certs (LE doesn't support `.onion`/`.i2p`).

**FQDN resolution priority:** reverse proxy headers (`X-Forwarded-Host` etc.) → `DOMAIN` env (comma-separated, first = primary) → `os.Hostname()` → `$HOSTNAME` → public IPv6 → public IPv4 → `localhost`.

---
For complete details, see AI.md PART 13, 14, 15
