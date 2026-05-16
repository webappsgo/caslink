# Backend Rules (PART 9, 10, 11, 32)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Store passwords in plaintext - use Argon2id
- Store tokens in plaintext - use SHA-256 hash
- Use bcrypt (EVER)
- Use non-parameterized SQL queries (SELECT * FROM users WHERE email = '"+email+"')
- Use SELECT * queries
- Skip defer tx.Rollback() in transactions
- Skip connection pool limits
- Expose internal errors to users (stack traces, DB info, hostnames)

## CRITICAL - ALWAYS DO
- Argon2id for ALL passwords
- SHA-256 for ALL tokens before storage
- Parameterized queries everywhere
- defer tx.Rollback() pattern in all transactions
- All network calls, DB queries, subprocess waits MUST have timeouts
- Constant-time comparison for auth (enumeration mitigation)
- Same error message for "wrong password" vs "no such user"
- Audit log is append-only, never contains raw credentials

## DATABASE
| Driver | Default | Notes |
|--------|---------|-------|
| SQLite | YES | `{data_dir}/db/` (server.db, users.db) |
| PostgreSQL | Optional | pgx/v5 |
| MySQL | Optional | |
| SQL Server | Optional | |

- Schema applied at startup via numbered migration files (001-006)
- No separate migration runner binary
- Migrations are idempotent

## SECURITY
- CSRF on all state-mutating forms
- XSS prevention via template escaping
- SQL injection prevention via parameterized queries
- Path traversal guards on file-serving routes
- Rate limiting: 5 login attempts / 15 min, 3 password reset / 1 hour
- GeoIP as signal only, never sole access gate

## TOR HIDDEN SERVICE (PART 32 - REQUIRED)
- Auto-enabled when Tor binary found on system
- Server binary controls Tor startup (not OS service)
- Provides .onion address when running

---
For complete details, see AI.md PART 9, 10, 11, 32
