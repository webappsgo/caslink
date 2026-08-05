# Project Audit

Started: 2026-08-02

Comprehensive six-pass health audit of caslink. Issues fixed directly; this
file tracks the batch (>5 findings) and is deleted once every item is resolved.

## Pass 1: Security
- [x] service/password.go: Argon2id time=1 below OWASP 2023 minimum (AI.md PART 11 requires time=3) — FIXED (a34b0c73d25f)
- [x] server/middleware.go: X-Forwarded-For / X-Real-IP trusted from any peer, letting a direct public client spoof its IP (rate-limit/GeoIP/audit/blocklist poisoning) — FIXED, now gated on trusted-proxy validation (2a0c96604136)
- [x] handler/url.go RedirectURL: password-protected links redirected without ever checking PasswordHash — advertised protection was decorative — FIXED, unlock gate + prompt page + tests
- [ ] service/url.go: URL validation (url.ParseRequestURI) accepts any scheme, so javascript:/data:/file: destinations can be stored and served as redirect targets — restrict to http/https

## Pass 3: Logic
- [x] service/url.go parseExpiration: returned year-0001 zero time for "never"/default, which GetURLByCode treated as already-expired, killing never-expire links at creation — FIXED, returns *time.Time nil (1db4bc1c2f0d)

## Pass 4: Documentation
- [ ] docs/configuration.md: undocumented runtime env vars (DEBUG, APP_URL, APP_ENV/ENV/ENVIRONMENT) and config sections (seo, compliance, trusted_proxies, compression); SMTP_TLS gap
- [ ] service/email.go: hardcoded http://localhost:64521 APP_URL default and localhost fallbacks in generated email links/vars — violates binary-rules no-hardcode-localhost/port; resolve {proto}://{FQDN}[:port] from config

## Pass 6: Code Flow Trace
- [ ] config.IsTruthy/IsFalsy, mode.GetModeInfo, paths.WritePIDFile/RemovePIDFile, paths.ResolvePath: exported but no callers — decide wire-up vs remove (some are spec-mandated helpers)

## Deferred (tracked in TODO.AI.md, not blocking)
- trusted_proxies.additional hostname entries are not DNS-resolved (blocking DNS on the request path is undesirable) — IP/CIDR only for now
