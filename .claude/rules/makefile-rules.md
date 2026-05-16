# Makefile Rules (PART 26)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use Makefile in CI/CD workflows (explicit commands only in CI)
- Run `go` directly on host machine
- Hardcode PROJECTNAME or PROJECTORG (always infer from git remote or path)

## CRITICAL - ALWAYS DO
- All builds run in Docker (golang:alpine) internally
- Use GODIR/GOCACHE for persistent module cache
- Infer PROJECTNAME from git remote or basename("$PWD")
- Infer PROJECTORG from git remote or basename(dirname("$PWD"))
- VERSION: env var > release.txt > "0.1.0"

## REQUIRED TARGETS
| Target | Purpose | Output |
|--------|---------|--------|
| `make dev` | Quick dev build | `$TMPDIR/casapps/caslink-XXXXXX/` |
| `make local` | Production test build | `binaries/` with version |
| `make build` | Full cross-platform | `binaries/` all 8 platforms |
| `make test` | Unit tests | Coverage report |
| `make clean` | Remove build artifacts | |
| `make release` | Create release | `releases/` |
| `make docker` | Build Docker image | local image |

## BUILD PLATFORMS (all 8 required)
- linux/amd64, linux/arm64
- darwin/amd64, darwin/arm64
- windows/amd64, windows/arm64
- freebsd/amd64, freebsd/arm64

## LDFLAGS
```
-s -w -X 'main.Version=...' -X 'main.CommitID=...' -X 'main.BuildDate=...' -X 'main.OfficialSite=...'
```

---
For complete details, see AI.md PART 26
