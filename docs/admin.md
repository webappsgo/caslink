# Admin Panel

The server-admin panel manages the Caslink instance. Server admins are separate from regular user accounts. The first admin is the primary admin and cannot be deleted.

## Sign in

The login page is `/server/auth/login`; submit credentials there. Successful server-admin authentication redirects to `/server/{admin_path}`. The `server.admin_path` setting defaults to `administration` and is configurable. When the application uses a URL path prefix, public and admin page links, forms, and assets include that prefix; trusted reverse-proxy prefix headers take precedence over the configured `server.baseurl`. Authenticated admin pages require an admin session; first-run setup uses its one-time setup token and limited setup session. State-changing browser forms require CSRF protection. The admin path is not disclosed by API autodiscovery.

## First-run setup

On a new installation, follow the one-time setup instructions shown by the server. Setup creates the primary admin account. Protect the setup token and credentials, and complete setup before exposing the service to the public internet. The setup API provides status, verify, account, token, config, security, services, and complete actions under `/api/v1/server/{admin_path}/setup/`. See [Installation](installation.md) for the setup sequence.

## Pages

The admin root `/server/{admin_path}` is the dashboard. Server-management sections are direct children of that root, as required by PART 17; there is no `/config` grouping segment. The admin's own profile and preferences use `/server/{admin_path}/{admin_username}/*`.

| Path | Purpose |
|---|---|
| `/` | Server status and overview |
| `/settings` | Core server configuration |
| `/branding` | Site identity and visual branding |
| `/ssl` | TLS configuration and certificate status |
| `/scheduler` | Scheduled task status and actions |
| `/email` | Email delivery configuration and test action |
| `/logs`, `/logs/audit` | Application and audit logs |
| `/backup` | Backup and restore actions |
| `/maintenance` | Maintenance configuration |
| `/updates` | Update status and actions |
| `/info` | Build and runtime information |
| `/metrics` | Metrics status and configuration summary |
| `/security/auth` | Authentication policy and provider configuration |
| `/security/auth/{type}` | OIDC, LDAP, or SAML provider administration |
| `/security/tokens` | Admin API token management |
| `/security/ratelimit` | Rate-limit settings |
| `/security/firewall` | Firewall rules |
| `/security/allowlist` | Allowlist settings |
| `/security/reports`, `/security/reports/{tracking_id}` | Security report triage and status |
| `/network/tor` | Tor availability and hidden-service status |
| `/network/i2p` | I2P eepsite status and configuration |
| `/network/geoip` | GeoIP configuration and status |
| `/network/blocklists` | Blocklist configuration and status |
| `/domains`, `/domains/{domain}` | Custom-domain moderation |
| `/users`, `/users/{id}` | User list and account details |
| `/{admin_username}/profile` | The signed-in admin's own profile and username settings |
| `/{admin_username}/preferences` | The signed-in admin's own display preferences |
| `/{admin_username}/notifications` | The signed-in admin's notification preferences |
| `/users/invites` | User invitations |
| `/orgs/invites`, `/orgs/create` | Organization invitations and creation controls |
| `/moderation/users` | User moderation queue |
| `/cluster/nodes`, `/cluster/add` | Cluster node status and registration |
| `/help` | Admin help |

Actions such as suspending users, changing settings, or restoring backups use POST forms protected by CSRF.

## Admin API

The admin API is rooted at `/api/v1/server/{admin_path}/` (or the configured API base path). First-run `/setup/*` endpoints use the one-time setup token and setup session; after setup, administrative endpoints require an `adm_` bearer token. Implemented endpoints cover user moderation and recovery-key regeneration, server settings and branding, server information, scheduler tasks, maintenance, Tor and I2P status/configuration, custom-domain moderation, OIDC/LDAP/SAML provider management, and the signed-in admin's profile, security status, preferences, and notification preferences. A `usr_` or `org_` token cannot access these routes. The OpenAPI page lists exact request and response schemas.

## Operational notes

- Configure backups and verify that the backup destination is writable and has sufficient space.
- Restrict metrics and admin access at the network boundary.
- Keep provider credentials and generated recovery material private.
- Review the audit log for security-relevant administrative actions.

## I2P network settings

The **Network → I2P** page reports provider state, provider type, SAM address, destination, and uptime. Authorized admins can enable or disable the eepsite, configure the provider and tunnel parameters, request a restart, or regenerate the destination after explicit confirmation. Regeneration replaces the persisted destination keys and therefore changes the eepsite address. The settings API is available at `/api/v1/server/{admin_path}/network/i2p` with GET and PATCH; validation, restart, and confirmed regeneration use the corresponding `/validate`, `/restart`, and `/regenerate` actions.
