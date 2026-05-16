# Optional Rules (PART 34-36)

**These PARTs are OPTIONAL but NON-NEGOTIABLE WHEN IMPLEMENTED.**

## STATUS FOR THIS PROJECT
Based on IDEA.md, this project HAS implemented:
- PART 34: Multi-User (users, registration, auth)
- PART 35: Organizations (orgs, RBAC)
- PART 36: Custom Domains (branded domains, SSL)

All three are ACTIVE and must be followed exactly.

## MULTI-USER (PART 34)
- Public registration on by default (operators may disable)
- Login: username or email + password (Argon2id)
- Session tokens: short-lived JWTs
- API tokens: long-lived Bearer, managed at `/user/tokens`
- 2FA: TOTP (RFC 6238), Passkeys/WebAuthn, "remember this device"
- OAuth2/OIDC social login (Google, GitHub, configurable)
- Password reset: time-limited token (24h, SHA-256 hashed, never raw)
- Users own their links; cannot see other users' private links

## ORGANIZATIONS (PART 35)
- Users may create up to 5 orgs (configurable)
- Roles: owner, admin, member
- Org API tokens scoped to org
- Org ownership transfer available
- Audit log tracks all permission changes within org

## CUSTOM DOMAINS (PART 36)
- Users: up to 5 custom domains (configurable)
- Orgs: up to 20 custom domains (configurable)
- DNS TXT record verification
- Auto SSL via Let's Encrypt
- Apex domains and subdomains supported

---
For complete details, see AI.md PART 34, 35, 36
