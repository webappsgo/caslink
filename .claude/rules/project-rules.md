# Project Rules (PART 2, 3, 4)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use GPL/AGPL/LGPL dependencies (copyleft)
- Create root-level `config/`, `data/`, `logs/`, `vendor/` dirs
- Create `Dockerfile` in project root (use `docker/Dockerfile`)
- Create `docker-compose.yml` in project root (use `docker/docker-compose.yml`)
- Create `CHANGELOG.md`, `SUMMARY.md`, `COMPLIANCE.md`, `NOTES.md`
- Create `SECURITY.md` in root (belongs in `.github/`)
- Hardcode dev machine paths, IPs, or hostnames
- Use plural directory names (`handlers/`, `models/`) - use singular

## CRITICAL - ALWAYS DO
- MIT license in `LICENSE.md` with third-party attributions
- All paths use `{internal_name}` (frozen), not `{project_name}` (mutable)
- `docker/rootfs/` is build-time overlay (committed to repo)
- `volumes/` is runtime data (gitignored)
- Detect project vars from git remote or directory path - never hardcode
- Use singular directory names: `handler/`, `model/`, `service/`

## REQUIRED ROOT FILES
| File | Status |
|------|--------|
| `AI.md` | Source of truth |
| `IDEA.md` | Project plan |
| `CLAUDE.md` | Loader |
| `README.md` | Documentation |
| `LICENSE.md` | MIT + embedded |
| `Makefile` | Local dev only |
| `go.mod` / `go.sum` | Go module |
| `release.txt` | Version |
| `.gitignore` | Git ignores |
| `.dockerignore` | Docker ignores |
| `.gitattributes` | Git attributes |
| `Jenkinsfile` | Jenkins CI |
| `mkdocs.yml` | Docs config |
| `.readthedocs.yaml` | RTD config |
| `renovate.json` | Dep updates |

## OS-SPECIFIC PATHS (Linux privileged)
- Config: `/etc/casapps/caslink/`
- Data: `/var/lib/casapps/caslink/`
- Logs: `/var/log/casapps/caslink/`
- SQLite: `/var/lib/casapps/caslink/db/`
- Service: `/etc/systemd/system/caslink.service`

## DOCKER PATHS (container only)
- Config: `/config/caslink/`
- Data: `/data/caslink/`
- SQLite: `/data/db/sqlite/`

---
For complete details, see AI.md PART 2, 3, 4
