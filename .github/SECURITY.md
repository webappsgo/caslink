# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest stable (`main`) | ✅ |
| Beta (`beta`) | ✅ security fixes only |
| Daily (`daily`) | ❌ development builds only |

We support the latest stable release. Security fixes are backported to the current beta channel when feasible.

## Reporting a Vulnerability

**Do NOT file a public GitHub issue for security vulnerabilities.** Public disclosure before a fix is available puts all caslink users at risk.

### Preferred Reporting Path

1. **Email:** casjay@yahoo.com  
   Subject line: `[caslink][SECURITY] <brief description>`  
   Encrypt if possible (ask for a PGP key by email).

2. **Well-known endpoint:** `/.well-known/security.txt` on any running caslink instance lists the current security contact and policy URL.

3. **In-app contact:** `/server/contact?security_id=vuln` on any running instance routes directly to the security team.

### What to Include

- caslink version / commit ID (output of `caslink --version`)
- Operating system and deployment method (binary, Docker, systemd service)
- Description of the vulnerability
- Steps to reproduce (proof-of-concept code is welcome)
- Potential impact assessment
- Whether you believe this is being actively exploited

### What Happens Next

| Step | Timeframe |
|------|-----------|
| Acknowledgement | Within 48 hours |
| Initial assessment | Within 5 business days |
| Fix development | Varies by severity (critical: ≤7 days) |
| Coordinated disclosure | Agreed with reporter before public release |

We follow responsible disclosure: reporters who follow this policy will be credited in the release notes (unless they prefer anonymity).

## Security Features

caslink ships with these security controls enabled by default:

- Argon2id password hashing (never bcrypt)
- CSRF protection on all state-mutating forms
- Rate limiting on all authentication endpoints
- SHA-256 token storage (never plaintext)
- Parameterized SQL queries (no SQL injection)
- XSS prevention via server-side template escaping
- Path traversal guards on file-serving routes
- `/.well-known/security.txt` (RFC 9116) on all instances
- Audit log for all security-relevant events

## Scope

In scope: the caslink server binary, CLI binary, Docker image, and any CI/CD workflows in this repository.

Out of scope: third-party dependencies (report to their maintainers), issues requiring physical access, social engineering attacks on users.
