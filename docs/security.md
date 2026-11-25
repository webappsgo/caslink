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

Tor hidden-service support starts automatically when enabled by the runtime configuration and a Tor executable is available. Overlay startup is asynchronous and does not prevent the HTTP listener from starting.

## Public endpoints

Health, API discovery, public documentation, public information pages, short-link redirects, and explicitly public link data are reachable without an account. Metrics endpoints are available only when enabled and may require a service token. Review the server configuration and network exposure before publishing operational endpoints.

The `/.well-known/` namespace includes `security.txt`, `change-password`, and active ACME HTTP-01 challenge responses. Other paths are not served unless an enabled feature defines them.

## Security reporting

Use the private vulnerability reporting channel linked from the repository's [Security policy](https://github.com/webappsgo/caslink/blob/main/.github/SECURITY.md). Do not publish vulnerability details in public issues. The server's security-report form uses an expiring submission identifier, spam protection, and encrypted report storage. Do not include real secrets or personal data in a report unless they are necessary to reproduce the issue.

## Audit and monitoring

Security-sensitive administrator and account actions are recorded in the audit system. Audit records must not contain raw credentials, tokens, passwords, or recovery keys. Review server logs and audit records for suspicious login, token, and configuration activity.
