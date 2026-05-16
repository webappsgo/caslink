# Binary Rules (PART 7, 8, 33)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use CGO (CGO_ENABLED=0 always - no exceptions)
- Build for fewer than 8 platforms
- Use `-musl` suffix in binary names
- Build Go directly on host - always use Docker (golang:alpine)
- Skip any required CLI flag

## CRITICAL - ALWAYS DO
- Single static binary, all assets embedded with go:embed
- 8 platforms: linux/darwin/windows/freebsd x amd64/arm64
- Binary naming: `caslink-{os}-{arch}` (windows adds .exe)
- Build from `./src` directory always
- CGO_ENABLED=0 always

## REQUIRED CLI FLAGS (NON-NEGOTIABLE)
```
--help / -h           Show help
--version / -v        Show version
--mode {production|development}
--config {config_dir}
--data {data_dir}
--log {log_dir}
--pid {pid_file}
--address {listen}
--port {port}
--baseurl {path}
--debug
--status
--service {start,restart,stop,reload,--install,--uninstall,--disable,--help}
--daemon
--maintenance {backup,restore,update,mode,setup,--help} [optional-arg]
--update [check|yes|branch {stable|beta|daily}]
```

Short flags: ONLY -h (help) and -v (version). All others: long form only.

## BINARIES
| Binary | Name | Required |
|--------|------|----------|
| server | `caslink` | YES |
| client | `caslink-cli` | YES |
| agent | `caslink-agent` | OPTIONAL |

## BUILD COMMANDS (local dev)
| Command | Purpose | Output |
|---------|---------|--------|
| `make dev` | Quick build | `$TMPDIR/casapps/caslink-XXXXXX/` |
| `make local` | Production test | `binaries/` with version |
| `make build` | Full release | `binaries/` all 8 platforms |
| `make test` | Unit tests | Coverage report |

NEVER run `go build` directly on host - always via Makefile (Docker internally).

---
For complete details, see AI.md PART 7, 8, 33
