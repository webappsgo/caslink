# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

**Scope note: this file covers LOCAL DEVELOPMENT ONLY. `make build`/`make release`/`make docker` are for manual/local use — automated CI/CD pipelines are a separate concern (see `cicd-rules.md`).**

## CRITICAL - NEVER DO

- Never add a 7th Makefile target — exactly six: `dev`, `local`, `build`, `test`, `release`, `docker`
- Never hardcode `PROJECT_NAME`/`PROJECT_ORG` — always derive from `git remote get-url origin` (fallback to directory basenames)
- Never build on the host — all builds go through Docker (`casjaysdev/go:latest`)
- Never symlink or copy a binary out of `binaries/` (to PATH, `/usr/local/bin`, another dir, etc.) — run it in place
- Never guess or assume `OFFICIAL_SITE` — only from `site.txt`, `OFFICIAL_SITE` env var, or empty
- Never add a `v` prefix to a text/timestamp version (`dev`, `beta`, `daily`, `20251218060432`) — only numeric semver gets `v`
- Never double the `v` prefix (`vv1.2.3`)
- Never let `make docker` push — it builds and tags only; pushing is CI/CD's job
- Never skip `clean` at the start of `build`/`local` (it runs automatically as a dependency)
- Never commit without `make test` passing and coverage ≥ 60%

## CRITICAL - ALWAYS DO

- Always use exactly these six targets, matching the purpose table below
- Always derive `VERSION` with this priority: `VERSION` env var → `release.txt` → `devel` fallback (or create `release.txt` with `0.1.0` on first release)
- Always embed `Version`, `CommitID`, `BuildDate`, `OfficialSite` via `-ldflags` (`-s -w -X main.X=...`)
- Always use `CGO_ENABLED=0` and `GOFLAGS=-buildvcs=false` for Docker Go builds
- Always mount persistent Go caches: `~/go/pkg/mod` → `/usr/local/share/go/pkg/mod`, `~/.cache/go-build/caslink` → `/usr/local/share/go/cache`
- Always build all 8 platforms for `make build`/release binaries: linux, darwin, windows, freebsd × amd64, arm64
- Always name binaries `caslink[-cli|-agent]-{os}-{arch}[.exe]` for distribution; unsuffixed for local (`binaries/caslink`)
- Always strip binaries built with musl before release (no `-musl` in the final name)
- Always run `make dev` output to an isolated temp dir: `${TMPDIR:-/tmp}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/`
- Always apply the correct `v`-prefix rule (see table) when tagging or naming releases

## Key Rules

### Six targets

| Target | Purpose | Output | Ldflags? |
|---|---|---|---|
| `dev` | Quick dev build | `${TMPDIR}/webappsgo/caslink-XXXXXX/` | No |
| `local` | Production test build | `binaries/` | Yes |
| `build` | Full release, 8 platforms | `binaries/` | Yes |
| `test` | Unit tests + coverage | Coverage report (≥60%) | n/a |
| `release` | GitHub release w/ source archive | `releases/` | via build |
| `docker` | Build multi-arch image, no push | buildx cache | via build-args |

### Version tag `v` prefix

| Input | Type | Gets `v`? | Example |
|---|---|---|---|
| `1.2.3` / `v1.2.3` | Semver | Yes | `v1.2.3` |
| `dev`, `beta`, `daily` | Text | No | `dev` |
| `20251218060432` | Timestamp | No | `20251218060432` |

### Release types

| Type | Trigger | version.txt | Release name | Max kept |
|---|---|---|---|---|
| Stable | tag push `v*`/`*.*.*` | `1.2.3` | `v1.2.3` | unlimited |
| Beta | push to `beta` branch | `{YYYYMMDDHHMMSS}-beta` | same | unlimited |
| Daily | schedule 3am UTC + push main | `{YYYYMMDDHHMMSS}` | `daily` | 1 (overwrites) |

### Directory rules

`binaries/` = build output only, gitignored. `releases/` = packaged release artifacts. Every release includes `version.txt`.

### Local workflow order

`make dev` (iterate) → `make test` (unit, pre-commit gate) → `./tests/run_tests.sh` (integration) → `make local` (prod test build) → `./tests/incus.sh` (preferred systemd test) → `make build` (cross-platform) → `make release` or CI/CD tag push.

### Binary naming pattern

`{project_name}[-cli|-agent]-{os}-{arch}[.exe]` — e.g. `caslink-linux-amd64`, `caslink-cli-windows-amd64.exe`.

### Prohibited binary handling

Never symlink/copy `binaries/*` anywhere (PATH, system dirs, test dirs, between `binaries/`/`releases/`). Run with `./binaries/caslink`. Only CI/CD's release process copies/strips/uploads binaries.

For complete details, see AI.md PART 26
