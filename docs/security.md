# Security

This page describes Caslink's authentication boundaries, data protection, and vulnerability reporting process.

## Authentication and sessions

- Account passwords are hashed with Argon2id. API tokens are stored as SHA-256 hashes and displayed only when created or rotated.
- Server administrators and regular users are separate account types. The primary server administrator cannot be deleted.
- Browser sessions are opaque, server-side session IDs in `HttpOnly` cookies. Admin and user sessions are stored and validated separately; API tokens are not browser session tokens.
- Admin sessions normally last 30 days and user sessions 7 days. Cookies use `Secure` on HTTPS and `SameSite` protections.
- API credentials use distinct scopes: `adm_` for server administration, `usr_` for user access, and `org_` for organization access. A user or organization token cannot access server-admin routes.
- Login, registration, password reset, and other exposed authentication flows are rate-limited. Password reset tokens are single-use and stored as hashes.

## Multi-factor authentication

User accounts support TOTP and WebAuthn passkeys. Recovery keys are generated for account recovery, stored as hashes, and accepted once. A login requiring a second factor does not receive a full user session until that factor is verified.

## External identity providers

Administrators can configure identity providers through the admin security settings. Supported provider types are OIDC, LDAP, and SAML. Provider credentials are sensitive configuration and should be kept private. Provider sign-in is available only when the provider is enabled and its server-side configuration is complete.

## Authorization

- Server administrators manage Caslink settings and operations; they are not privileged operating-system users.
- Regular users own their links and account settings.
- Organization permissions are scoped to membership and role. Organization credentials do not grant server-administration access.
- Requests that change browser state require CSRF protection. API requests use bearer credentials.

## Transport and stored data

Serve public traffic over HTTPS. Caslink can manage certificates for configured hostnames, and renewals run through the built-in scheduler. Sensitive values and security reports are encrypted at rest where specified by server configuration; credentials and raw tokens must not be included in logs.

Tor hidden-service support starts automatically when a Tor executable is available; there is no enable/disable setting. Overlay startup is best-effort and does not prevent the HTTP listener from starting.

## Public endpoints

Health, API discovery, public documentation, public information pages, short-link redirects, and explicitly public link data are reachable without an account. Metrics endpoints are available only when enabled and may require a service token. Review the server configuration and network exposure before publishing operational endpoints.

The `/.well-known/` namespace includes `security.txt`, `change-password`, and active ACME HTTP-01 challenge responses. Other paths are not served unless an enabled feature defines them.

## Privacy and terms pages

`/server/privacy` and `/server/terms` use built-in project text. The privacy page covers data collection, use, security, retention, storage, cookies, rights, and third-party services. It also reports deployment settings from `server.privacy`: whether data is sold or stored on the server, cookie descriptions, retention, sharing conditions, and third-party services. The sold state selects the matching data-use and consent text and exposes opt-out controls only when enabled. The page reports when the current request is not protected by TLS and uses the configured DPO email when present; otherwise it links to the security page. The built-in terms cover acceptance, account responsibilities, acceptable use, submitted content, termination, liability, changes, and governing law.

The JSON endpoints `{api_base_path}/server/privacy` and `{api_base_path}/server/terms` return the corresponding page content in the standard `{ok, data}` envelope. Privacy includes summary, cookie categories, data handling, tracking, retention, third parties, conditional CCPA data, and built-in policy text. Terms returns Markdown content and its format.

## Security reporting

Use the human-readable [security overview](https://caslink.casapps.us/server/security) to see the active reporting channels and instructions. Researchers can read the server's `security.txt` at `https://caslink.casapps.us/.well-known/security.txt` and use the security-report form linked from the overview. The form's `security_id` is short-lived; open the overview to get the current form link rather than reusing an old one. Read the [disclosure policy](https://caslink.casapps.us/server/security/policy) before testing. Do not publish vulnerability details in public issues.

Security reports are encrypted before they are persisted. When the project OpenPGP key exists, report bodies use that key; otherwise the server uses AES-256-GCM. Researcher contact details are encrypted separately. A researcher may provide a public key to receive encrypted status updates, but a key is not required. Acknowledgments contain only the tracking ID and status URL, not report content.

## OpenPGP key management

Administrators manage the project security-report keypair in `/server/{admin_path}/security/keypair`. The private key is encrypted with a key derived from the installation secret and stored with owner-only permissions. The public key is available at `https://caslink.casapps.us/.well-known/pgp-key.asc` when publishing is enabled and a keypair exists; the `/server/security` overview reports its availability and fingerprint. Generation and rotation publish the public key to configured keyservers, with durable retries for temporary failures. Exporting the private key, rotating, or deleting it requires the administrator's password and a reason and is recorded in the audit log. Backups include the keypair and keyserver publication state so encrypted reports remain decryptable after restore.

## Public security endpoints

- `https://caslink.casapps.us/.well-known/security.txt` — active security contacts, policy and optional encryption key.
- `https://caslink.casapps.us/.well-known/pgp-key.asc` — public key when publishing is enabled and a keypair exists; otherwise this returns `404`.
- `https://caslink.casapps.us/.well-known/change-password` — browser password-change discovery route.
- `https://caslink.casapps.us/.well-known/acme-challenge/{token}` — active HTTP-01 certificate validation only.
- `https://caslink.casapps.us/server/security` — current reporting channels and PGP status.
- `https://caslink.casapps.us/server/security/policy` — coordinated disclosure policy.
- `https://caslink.casapps.us/server/healthz` and `https://caslink.casapps.us/api/v1/server/healthz` — health endpoints.

Unknown `/.well-known/*` entries return `404`. WebFinger, OpenID Provider Metadata, Android App Links, Apple association and MTA-STS are not enabled by this project.

## Audit and monitoring

Security-sensitive administrator and account actions are recorded in the audit system. Audit records must not contain raw credentials, tokens, passwords, or recovery keys. Review server logs and audit records for suspicious login, token, and configuration activity.
