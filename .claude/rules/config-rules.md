# Config Rules (PART 5, 6, 12)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use .env files (hardcode sane defaults in docker-compose instead)
- Commit runtime config files (`server.yml`) to repo
- Hardcode machine-specific values in config or binary
- Use inline YAML comments - ALL comments go ABOVE the setting
- Use strconv.ParseBool() - use config.ParseBool() instead

## CRITICAL - ALWAYS DO
- Config hierarchy: defaults -> server.yml -> env vars -> CLI flags -> admin panel
- Env prefix: `CASLINK_*`
- Config file: `server.yml` (not .yaml)
- All boolean parsing via config.ParseBool() (handles 40+ variations)
- Detect mode at startup: --mode flag > MODE env > default "production"
- Detect debug at startup: --debug flag > DEBUG env > default false

## CONFIG HIERARCHY
1. Defaults (hardcoded sane values, zero-config startup)
2. Config file (`{config_dir}/server.yml`)
3. Environment variables (`CASLINK_*` prefix)
4. CLI flags (`--port`, `--data`, `--config`, `--mode`, `--debug`)
5. Admin panel (runtime overrides stored in DB)

## APPLICATION MODES
| Mode | Debug | Behavior |
|------|-------|----------|
| production | false | Live deployment, minimal logging |
| production | true | Live debug (temporary) |
| development | false | Local dev, verbose logs |
| development | true | Full debug, all endpoints |

## DEBUG ENDPOINTS (--debug/DEBUG=true ONLY)
- `/debug/pprof/*` - profiling
- `/debug/vars` - expvar
- `/debug/config`, `/debug/routes`, `/debug/cache`, `/debug/db`, `/debug/scheduler`
- Returns 404 in production unless debug enabled

---
For complete details, see AI.md PART 5, 6, 12
