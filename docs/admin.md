# Admin Panel

The server-admin panel manages the Caslink instance. Server admins are separate from regular user accounts. The first admin is the primary admin and cannot be deleted.

## Sign in

The login page is `/server/auth/login`; submit credentials there. Successful server-admin authentication redirects to `/server/{admin_path}`. The `server.admin_path` setting defaults to `administration` and is configurable. When the application uses a URL path prefix, public and admin page links, forms, and assets include that prefix; trusted reverse-proxy prefix headers take precedence over the configured `server.baseurl`. Authenticated admin pages require an admin session; first-run setup uses its one-time setup token and limited setup session. State-changing browser forms require CSRF protection. The admin path is not disclosed by API autodiscovery.

## First-run setup

On a new installation, follow the one-time setup instructions shown by the server. Setup creates the primary admin account. Protect the setup token and credentials, and complete setup before exposing the service to the public internet. The setup API provides status, verify, account, token, config, security, services, and complete actions under `/api/v1/server/{admin_path}/setup/`. The status response includes `is_complete` and `current_step`; the latter reports the active session's next step. See [Installation](installation.md) for the setup sequence.

## Pages

The admin root `/server/{admin_path}` is the dashboard. Server-management sections are direct children of that root, as required by PART 17; there is no `/config` grouping segment. The admin's own profile and preferences use `/server/{admin_path}/{admin_username}/*`.

| Path | Purpose |
|---|---|
| `/` | Server status and overview |
| `/settings` | Core server configuration |
| `/branding` | Site identity and visual branding |
| `/pages` | Public page-content editors for the header, About, Privacy, Terms, and footer |
| `/pages/{header,about,privacy,terms,footer}` | Edit sanitized operator content; About, Privacy, and Terms extend or override their built-in pages, and reset removes the branding entry |
| `/pages/contact` | Configure the public report form, its recipient, CAPTCHA provider, and success message; send a test email |
| `/privacy` | Configure data-sharing disclosures, consent banner text and position, cookie descriptions, retention, and third-party services; open authenticated full-page previews with either the sold or unsold policy state |
| `/privacy/preview?state={sold,unsold}` | Render the public privacy page using the selected policy state without changing saved configuration |
| `/pages/help` | Help and documentation content |
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
| `/security/blocked-ips` | Active IP blocks, manual blocks, expiration cleanup, and repeat-offender escalation |
| `/security/allowlist` | Allowlist settings |
| `/security/locked-accounts` | Locked user accounts and admin unlock actions |
| `/security/keypair` | Security-report OpenPGP key generation, rotation, publication, and retirement |
| `/security/reports`, `/security/reports/{tracking_id}` | Security report triage and status |
| `/network/tor` | Tor availability and hidden-service status |
| `/network/i2p` | I2P eepsite status and configuration |
| `/network/geoip` | GeoIP configuration and status |
| `/network/blocklists` | Blocklist configuration and status |
| `/domains`, `/domains/{domain}` | Custom-domain moderation |
| `/users`, `/users/{id}` | User list and account details |
| `/{admin_username}/profile` | The signed-in admin's own profile and username settings |
| `/{admin_username}/profile/security` | The signed-in admin's TOTP and passkey enrollment and recovery keys |
| `/{admin_username}/profile/security/totp/recovery-keys` | Regenerate TOTP recovery keys after current-password and second-factor verification; the new keys are shown once |
| `/{admin_username}/profile/security/passkey/confirm-recovery` | Activate a first passkey after acknowledging that the one-time recovery keys were saved |
| `/{admin_username}/preferences` | The signed-in admin's own display preferences |
| `/{admin_username}/notifications` | The signed-in admin's notification preferences |
| `/users/invites` | User invitations |
| `/orgs/invites`, `/orgs/create` | Organization invitations and creation controls |
| `/moderation/users` | User moderation queue |
| `/nodes`, `/nodes/add` | Cluster node status and registration |

Cluster join tokens are single-use and expire after 15 minutes. A joining node receives the remote database/cache connection, shared secrets, connection TLS files, all `yaml.d/` area files (including `app.yml`), and all `assets/` files in the authenticated encrypted bootstrap response. Its local listener, SSL, and path settings remain local. The imported files are written with owner-only permissions before the node restarts into the shared cluster.
| `/help` | Admin help |

Actions such as suspending users, changing settings, enrolling or disabling MFA, or restoring backups use POST forms protected by CSRF. Security settings include configurable account lockout thresholds and IP block escalation durations. Allowlisted addresses bypass IP block enforcement; they still must authenticate normally.

## Admin API

The admin API is rooted at `/api/v1/server/{admin_path}/` (or the configured API base path). First-run `/setup/*` endpoints use the one-time setup token and setup session; after setup, administrative endpoints require an `adm_` bearer token. Implemented endpoints cover user moderation and recovery-key regeneration, server settings and branding, page-content settings (`GET /pages`, `GET`/`PATCH /pages/contact`, plus `GET`/`PATCH /pages/{header,about,privacy,terms,footer}`), help content, server information, scheduler tasks, maintenance, Tor and I2P status/configuration, custom-domain moderation, OIDC/LDAP/SAML provider management, locked-account and blocked-IP management, lockout and IP-block policy, OpenPGP security-report keys and reports, and the signed-in admin's profile, TOTP/passkeys, preferences, and notification preferences. A `usr_` or `org_` token cannot access these routes. The OpenAPI page lists exact request and response schemas.

Admin self-service pages live below `/server/{admin_path}/{admin_username}/`. The profile page changes the signed-in admin's username and password, and shows the masked API-token list. Password changes require the current password and, when enabled, a current TOTP code or recovery key; other admin sessions are revoked and a security email is attempted. Posting to `/profile/token` rotates the admin's API token, revokes previous tokens, and displays the new plaintext once. TOTP recovery keys can be regenerated after verifying the current password and a TOTP code or recovery key; the old keys stop working and the replacement set is shown once. Admin passkey setup requires the current password; a first passkey stays inactive until the admin confirms saving its one-time recovery keys. Security notifications cannot be disabled. Email notification choices are unavailable until SMTP is configured; saving WebUI choices without SMTP preserves the existing email preferences.

## Operational notes

- Configure backups and verify that the backup destination is writable and has sufficient space.
- Restrict metrics and admin access at the network boundary.
- Keep provider credentials and generated recovery material private.
- Review the audit log for security-relevant administrative actions.

## I2P network settings

The **Network → I2P** page reports provider state, provider type, SAM address, destination, and uptime. Authorized admins can enable or disable the eepsite, configure the provider and tunnel parameters, request a restart, or regenerate the destination after explicit confirmation. Regeneration replaces the persisted destination keys and therefore changes the eepsite address. The settings API is available at `/api/v1/server/{admin_path}/network/i2p` with GET and PATCH; validation, restart, and confirmed regeneration use the corresponding `/validate`, `/restart`, and `/regenerate` actions.
