# API Rules (PART 13, 14, 15)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use trailing slash in API routes
- Use non-plural nouns in REST routes
- Return plain text errors from API (use RFC 7807 error body)
- Expose /metrics publicly (internal only)
- Redirect /healthz -> /server/healthz (direct handler only)

## CRITICAL - ALWAYS DO
- REST API at `/api/v1/` - versioned, plural nouns, no trailing slash
- GraphQL at `/graphql`
- OpenAPI/Swagger at `/server/docs/swagger`
- All success: `{"ok":true,"data":{...}}`
- All errors: RFC 7807 error body `{"ok":false,"error":"CODE","message":"..."}`
- X-Request-ID propagated through every request
- Rate limiting on ALL auth endpoints

## HEALTH ENDPOINTS
| Endpoint | Access | Format |
|----------|--------|--------|
| `/server/healthz` | PUBLIC | HTML/JSON/text (content negotiation) |
| `/healthz` | PUBLIC (optional alias) | Same as /server/healthz |
| `/api/v1/server/healthz` | PUBLIC | JSON |
| `/metrics` | INTERNAL ONLY | Prometheus text |

NEVER expose /metrics publicly. NEVER include metrics data in healthz.

## SSL/TLS
- Let's Encrypt via HTTP-01 and DNS-01 challenges
- Certificates stored encrypted in database
- Auto-renewal scheduled via built-in scheduler
- SSL stored at `{config_dir}/ssl/` (letsencrypt/, local/)

---
For complete details, see AI.md PART 13, 14, 15
