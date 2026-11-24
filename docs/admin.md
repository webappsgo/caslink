# Admin Panel

The server-admin panel manages the Caslink instance. Server admins are separate from regular user accounts. The first admin is the primary admin and cannot be deleted.

## Sign in

The login page is `/server/{admin_path}/`; submit credentials to `/server/{admin_path}/login`. The `admin_path` setting is configurable. Admin pages require an admin session; state-changing forms require CSRF protection. The admin path is not disclosed by API autodiscovery.

## First-run setup

On a new installation, follow the one-time setup instructions shown by the server. Setup creates the primary admin account. Protect the setup token and credentials, and complete setup before exposing the service to the public internet. See [Installation](installation.md) for the setup sequence.

## Pages

All paths below are relative to `/server/{admin_path}`.

| Path | Purpose |
|---|---|
| `/dashboard` | Server status and overview |
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
| `/network/tor` | Tor status and configuration |
| `/network/geoip` | GeoIP configuration and status |
| `/network/blocklists` | Blocklist configuration and status |
| `/domains`, `/domains/{domain}` | Custom-domain moderation |
| `/users`, `/users/{id}` | User list and account details |
| `/users/invites` | User invitations |
| `/orgs/invites`, `/orgs/create` | Organization invitations and creation controls |
| `/moderation/users` | User moderation queue |
| `/cluster/nodes`, `/cluster/add` | Cluster node status and registration |
| `/help` | Admin help |

Actions such as suspending users, changing settings, or restoring backups use POST forms protected by CSRF.

## Admin API

The admin API is rooted at `{api_base_path}/server/{admin_path}/` and requires an `adm_` bearer token. It covers user moderation, server settings and branding, scheduler tasks, maintenance, custom-domain moderation, and OIDC/LDAP/SAML provider management. A `usr_` or `org_` token cannot access these routes. The OpenAPI page lists exact request and response schemas.

## Operational notes

- Configure backups and verify that the backup destination is writable and has sufficient space.
- Restrict metrics and admin access at the network boundary.
- Keep provider credentials and generated recovery material private.
- Review the audit log for security-relevant administrative actions.
