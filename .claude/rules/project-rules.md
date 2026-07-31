# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use any license other than MIT for this project (`LICENSE.md` required in project root)
- Add or keep a GPL/AGPL/LGPL-licensed dependency — copyleft licenses are forbidden, no exceptions
- Ship a static/badge-only license claim without a matching `LICENSE.md` — GitHub license detection requires the exact unmodified MIT template text
- Hardcode `{project_name}` or `{project_org}` in scripts — always infer from `git remote get-url origin` (preferred) or `basename "$PWD"` / `basename "$(dirname "$PWD")"` (fallback)
- Change `{internal_name}` after first-time setup — it is frozen forever even if `{project_name}` is later renamed
- Assume the current working directory is the project root — always resolve via `git rev-parse --show-toplevel` or `cd` to project root first
- Use `github.com/mattn/go-sqlite3`, `github.com/lib/pq`, `github.com/ooni/go-libtor`, `github.com/dgrijalva/jwt-go`, `github.com/gorilla/mux`, or `github.com/go-redis/redis` — all forbidden (see replacements below)
- Use any Go library that requires CGO — breaks `CGO_ENABLED=0` static builds
- Pin a specific Go version anywhere (docs, Dockerfile, CI) — always latest stable, use `casjaysdev/go:latest` unpinned
- Store plaintext passwords anywhere, including config files (`server.yml`) or logs
- Hash new passwords with anything but Argon2id (bcrypt is verify-only, then rehash to Argon2id)
- Use `.yaml` extension for the config file — it is always `server.yml`
- Commit `binaries/`, `releases/`, or `volumes/` — always gitignored
- Put `docker/rootfs/` in `.dockerignore` — it is a required build-time overlay and must stay out of `.gitignore`/`.dockerignore` exclusions that would break the build
- Mix `{config_dir}` (user-editable) and `{data_dir}` (app-managed) purposes — never put user config in data_dir or vice versa
- Skip a required OS/arch target — Linux, BSD, macOS (Intel+ARM), Windows on AMD64 and ARM64 are all mandatory

## CRITICAL - ALWAYS DO

- Include `LICENSE.md` in project root with the exact MIT template text (only copyright year/name may change)
- Embed all third-party dependency licenses in `LICENSE.md` (compact table for 10+ deps, full text only when the license requires it, e.g. BSD-3-Clause non-endorsement clause, Apache 2.0 NOTICE)
- Automate license scanning in CI with `go-licenses` and fail the build on GPL/AGPL/LGPL detection
- Use `modernc.org/sqlite` for SQLite (pure Go, no CGO) — accept `sqlite`/`sqlite2`/`sqlite3` as config aliases, normalize internally to `sqlite`
- Use the required-library table below for auth, DB, cache, HTTP, and utility needs
- Use Argon2id (OWASP 2023 params: time=3, memory=64MB, threads=4, keyLen=32, saltLen=16) for password hashing; SHA-256 for API/session token hashing
- Reject passwords with leading/trailing whitespace (never silently trim)
- Follow the required directory layout exactly (`src/`, `docker/`, `.claude/rules/`, `docs/`, `tests/`, root files) — no extra/undocumented top-level dirs
- Start every `.gitignore` with the two required header lines (`# gitignore created on MM/DD/YY at HH:MM` then `ignoredirmessage`)
- Route all file paths relative to project root, never to `$PWD`/cwd, in code, docs, and scripts
- Use the correct OS-specific path for every artifact type (binary, config, data, cache, logs, backup, PID, SSL, DB) per the tables below

## Key Rules

**License:** MIT only. Compatibility matrix — MIT project may depend on MIT, Apache 2.0, BSD, ISC, Public Domain; may NEVER depend on GPL/AGPL/LGPL.

**Variable inference (never hardcode):**

| Variable | Source | Frozen? |
|----------|--------|---------|
| `{project_name}` | git remote or `basename "$PWD"` | No — may be renamed |
| `{project_org}` | git remote or `basename "$(dirname "$PWD")"` | No |
| `{internal_name}` | initial value = `{project_name}` | YES — frozen forever after first setup |
| `{plist_name}` | `io.github.{project_org}.{internal_name}` | derived |

Resolved for this project: `project_name=caslink`, `project_org=webappsgo`, `internal_name=caslink`, `app_name=Caslink`, `official_site=https://caslink.casapps.us`.

**Required directory layout (top level):** `.github/` or `.gitea/` workflows, `CLAUDE.md`, `.claude/{settings.json,agents/,rules/}`, `docs/` (MkDocs/ReadTheDocs only), `src/`, `scripts/`, `tests/{run_tests.sh,docker.sh,incus.sh}`, `docker/{Dockerfile,Dockerfile.dev,docker-compose*.yml,rootfs/}` (rootfs is committed), `volumes/` (gitignored), `binaries/` (gitignored), `releases/` (gitignored), plus root files: `README.md`, `LICENSE.md`, `AI.md`, `TODO.AI.md`, `TODO.md`, `PLAN.AI.md`, `PLAN.md`, `Jenkinsfile`, `release.txt`, `site.txt`.

**14 required `.claude/rules/*.md` files** (PART→file mapping) mirrored into `.cursor/rules/*.mdc`, `.windsurf/rules/*.mdc`, `.ai/rules/*.md` if those tools are used — see `ai-rules.md` for the full list.

**Required Go libraries (pure Go / CGO_ENABLED=0 only):**

| Purpose | Library |
|---------|---------|
| SQLite | `modernc.org/sqlite` |
| libSQL/Turso | `github.com/tursodatabase/libsql-client-go` (remote-only) |
| PostgreSQL | `github.com/jackc/pgx/v5/stdlib` |
| MySQL/MariaDB | `github.com/go-sql-driver/mysql` |
| MSSQL | `github.com/microsoft/go-mssqldb` |
| MongoDB | `go.mongodb.org/mongo-driver/mongo` |
| Valkey/Redis | `github.com/redis/go-redis/v9` |
| Router | `github.com/go-chi/chi/v5` |
| TOTP | `github.com/pquerna/otp` |
| Passkeys/WebAuthn | `github.com/go-webauthn/webauthn` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| OIDC | `github.com/coreos/go-oidc/v3` |
| LDAP | `github.com/go-ldap/ldap/v3` |
| SAML | `github.com/crewjam/saml` |
| Sessions | `github.com/gorilla/sessions` |
| Scheduler | `github.com/go-co-op/gocron/v2` |
| Password hash | `golang.org/x/crypto/argon2` (+ `bcrypt` for legacy verify) |

**Forbidden libraries:** `mattn/go-sqlite3`, `lib/pq`, `ooni/go-libtor`, `dgrijalva/jwt-go`, `gorilla/mux`, `go-redis/redis` (old path) — all require CGO or are unmaintained/insecure.

**Password/token hashing:** Argon2id for passwords (slow, memory-hard by design); SHA-256 for API/session tokens (fast lookup, tokens already high-entropy).

**OS-specific paths (privileged / user), pattern `{internal_org}/{internal_name}`:**

| OS | Config (privileged) | Data (privileged) | Service |
|----|---------------------|--------------------|---------| 
| Linux | `/etc/{internal_org}/{internal_name}/` | `/var/lib/{internal_org}/{internal_name}/` | `/etc/systemd/system/{internal_name}.service` |
| macOS | `/Library/Application Support/{internal_org}/{internal_name}/` | same, `/data/` | `/Library/LaunchDaemons/{plist_name}.plist` |
| BSD | `/usr/local/etc/{internal_org}/{internal_name}/` | `/var/db/{internal_org}/{internal_name}/` | `/usr/local/etc/rc.d/{internal_name}` |
| Windows | `%ProgramData%\{internal_org}\{internal_name}\` | same `\data\` | Windows Service Manager |
| Docker | `/config/{project_name}/` | `/data/{project_name}/` | n/a — internal port `80` |

Config file is always `server.yml` (never `.yaml`) on every OS. User (non-privileged) variants use `~/.config`, `~/.local/share`, `~/.cache`, `~/.local/log` on Linux/BSD; `~/Library/...` on macOS; `%AppData%`/`%LocalAppData%` on Windows.

**Directory purpose rule:** user-editable → `{config_dir}`; app-managed → `{data_dir}`; logs → `{log_dir}`; backups → `{backup_dir}`. Never mix.

**Platform support (mandatory):** Linux, BSD, macOS (Intel+Apple Silicon), Windows — each on AMD64 and ARM64.

For complete details, see AI.md PART 2, 3, 4
