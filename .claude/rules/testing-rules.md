# Testing & Documentation Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never run `go build`/`go test`/binaries directly on the host — the local machine has no Go; everything runs inside Docker (`casjaysdev/go:latest`) or Incus (`debian:latest`)
- Never use `docker-compose.yml` or `docker-compose.dev.yml` as an AI/agent — human use only; use `docker-compose.test.yml` via `tests/` scripts
- Never write runtime/test data to the project directory — always `${TMPDIR:-/tmp}/webappsgo/caslink-XXXXXX/`
- Never use bare `mktemp -d` or a generic `/tmp/` path — always the `{project_org}/{internal_name}-XXXXXX` structure
- Never bypass admin authentication in tests, in debug mode, or anywhere — no backdoors, no hardcoded dev creds
- Never use `pkill -f`, `pkill` without `-x`, `killall`, `docker rm $(docker ps -aq)`, `docker system prune`, or any other broad kill/cleanup command — only exact PIDs/names scoped to `caslink`
- Never put non-ReadTheDocs files in `docs/` — it is exclusively MkDocs documentation source
- Never hardcode a user-facing string — every string goes through `t(key)` / `{{t .Lang key}}`
- Never let `?lang=`/`Accept-Language` errors crash the app — unsupported language silently falls back to `en`
- Never convey information by color alone (accessibility)

## CRITICAL - ALWAYS DO

- Always run both test phases: Phase 1 `make test` (Go unit tests, ≥60% coverage, pre-commit gate) and Phase 2 `./tests/run_tests.sh` (binary/integration, manual, developer-initiated)
- Always create/update the matching `*_test.go` in the same work pass when package logic changes — never defer
- Always test every route with all applicable Accept headers (`text/html`+`text/plain` frontend; `application/json`+`text/plain` API) and every `.txt` endpoint
- Always test admin routes for both rejection (unauthenticated/invalid) and success (setup token → login → session)
- Always prefer Incus (full systemd) for Phase 2; fall back to Docker Alpine when Incus unavailable
- Always host `docs/` on ReadTheDocs via MkDocs Material with dark/light/auto toggle, per PART 16 theme rules
- Always keep every language's JSON keys in sync with `en.json` (`make i18n-validate`) and rebuild all binaries after adding a language
- Always set `dir="rtl"` for Arabic and use CSS logical properties (`margin-inline-start`, not `margin-left`)
- Always meet WCAG 2.1 AA: 4.5:1 text contrast, visible focus indicators, 44x44px touch targets, skip links as first focusable elements

## Key Rules

### Two required test phases

| Phase | Files | Run with | Gate |
|---|---|---|---|
| 1 — Toolchain | `*_test.go` | `make test` | ≥60% coverage, pre-commit, CI |
| 2 — Binary Validation | `tests/*.sh` | `./tests/run_tests.sh` | 100% endpoint/route coverage, manual only |

Required scripts: `tests/run_tests.sh` (auto-detect), `tests/docker.sh` (alpine fallback), `tests/incus.sh` (debian/systemd, preferred). All license `WTFPL`.

### Temp directory rule

Only acceptable pattern: `${TMPDIR:-/tmp}/webappsgo/caslink-XXXXXX/` (with `volumes/config`, `volumes/data` subdirs as needed). Never bare `/tmp/`, never missing org/project prefix.

### Content negotiation test matrix

| Route type | Required Accept headers |
|---|---|
| Frontend `/**` | `text/html`, `text/plain` |
| API `/api/v1/*` | `application/json`, `text/plain` |
| `.txt` endpoints | robots.txt, security.txt, `*.txt` API variants |

### Process/container cleanup — allowed vs forbidden

| Allowed | Forbidden |
|---|---|
| `kill {exact-pid}` after verify | `pkill -f {pattern}`, `killall` |
| `docker stop/rm caslink` | `docker rm $(docker ps -aq)` |
| `docker rmi webappsgo/caslink:tag` | `docker system/image/volume/network prune` |
| `rm -rf $BUILD_DIR`/`$TEST_DIR` (from mktemp) | `rm -rf /`, `~`, `.`, `*` |

### ReadTheDocs required files

`mkdocs.yml`, `.readthedocs.yaml`, `docs/{index,installation,configuration,api,admin,security,integrations,development}.md`, `docs/requirements.txt`, `docs/stylesheets/{dark,light}.css`. `cli.md` only if a CLI surface exists.

### i18n — supported languages

| Code | Language | Direction | Plurals |
|---|---|---|---|
| en | English | ltr | one, other |
| es | Spanish | ltr | one, other |
| zh | Chinese | ltr | other |
| fr | French | ltr | one, other |
| ar | Arabic | rtl | zero, one, two, few, many, other |
| de | German | ltr | one, other |
| ja | Japanese | ltr | other |

Language resolution order: `?lang=` (sets cookie) → `lang` cookie → `Accept-Language` header → `en`. Files at `src/common/i18n/locales/{lang}.json`, embedded via `go:embed`, validated by `make i18n-validate`.

### a11y core requirements

WCAG 2.1 AA, full keyboard nav, screen reader support (NVDA/JAWS/VoiceOver), 4.5:1 text contrast (3:1 large text/UI components), visible focus indicators, 44x44px touch targets, skip links, ARIA live regions for dynamic content, focus trap+return on modals.

For complete details, see AI.md PART 29, 30, 31
