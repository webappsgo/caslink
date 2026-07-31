# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never place `Dockerfile` or `docker-compose*.yml` in the project root — always under `docker/`
- Never bake `LABEL` blocks into the Dockerfile — all OCI metadata is applied at build time by CI (`labels:`/`annotations:`)
- Never modify `ENTRYPOINT` or `CMD` — all customization goes through `entrypoint.sh`
- Never let the entrypoint script create directories, set permissions, create the user/group, or manage Tor — the `caslink` binary does all of that
- Never set `MODE`/`DEBUG` in `docker-compose.yml` (production) — binary defaults to production; only `docker-compose.dev.yml`/`docker-compose.test.yml` set them
- Never use `.env` files, `.env.example`, or list-style (`- KEY=value`) environment blocks — always hardcoded map-style (`KEY: value`) with sane defaults
- Never include `build:` or `version:` keys in any compose file
- Never run `docker compose` from the project directory or with `--project-directory` pointing at project root — always a temp dir with `./volumes/`
- Never commit runtime `./volumes/` content from local runs
- Never use `docker-compose.dev.yml` as an AI/agent — it is HUMAN USE ONLY
- Never push `:dev` or `:test` image tags to the production registry
- Never expose PostgreSQL (5432) or Valkey (6379) ports externally in AIO containers — only port 80
- Never use Alpine for the AIO runtime stage — PostgreSQL/Valkey/Tor need glibc; use `debian:latest`

## CRITICAL - ALWAYS DO

- Always use a multi-stage Dockerfile: `casjaysdev/go:latest` builder → `alpine:latest` runtime (standard) or `debian:latest` (AIO)
- Always build with context `.` (project root) and `-f docker/Dockerfile`
- Always copy `docker/rootfs/` into the image (`COPY docker/rootfs/ /`) for the build-time overlay
- Always expose internal port 80
- Always use `tini` as init: `ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]`
- Always set `STOPSIGNAL SIGRTMIN+3`
- Always configure `HEALTHCHECK` with start-period 90s, interval 10s, timeout 5s, retries 3, running `{binary} --status`
- Always mount exactly two volumes: `./volumes/config:/config:z` and `./volumes/data:/data:z` (add `:z` only in production/test, omit in temp-dir dev runs)
- Always run compose from an isolated temp dir: `${TMPDIR:-/tmp}/webappsgo/caslink-XXXXXX/`
- Always use the `x-logging` anchor (`json-file`, max-size 5m, max-file 1) on every service
- Always name services/containers per convention: main `caslink` / `caslink-app`; db `caslink-db`; cache `caslink-cache`
- Always prefer `tests/run_tests.sh` or `tests/docker.sh` over invoking `docker-compose.test.yml` directly

## Key Rules

### Directory layout

```
docker/
├── Dockerfile              # production, alpine
├── Dockerfile.aio          # all-in-one, debian
├── Dockerfile.dev          # devel, tagged :devel
├── docker-compose.yml      # production — HUMAN USE ONLY
├── docker-compose.dev.yml  # dev — HUMAN USE ONLY
├── docker-compose.test.yml # test — AI/AUTOMATED TESTING ONLY
├── all-in-one.yml          # AIO compose
└── rootfs/usr/local/bin/entrypoint.sh
```

### Container paths

| Path | Contents |
|---|---|
| `/config/caslink/` | server.yml, ssl/, tor/ |
| `/data/caslink/security/{geoip,blocklists,cve}/` | downloaded security DBs |
| `/data/db/sqlite/{server,users}.db` | SQLite DBs |
| `/data/db/postgres/`, `/data/db/valkey/` | external services (if used) |
| `/data/log/caslink/` | access.log, error.log, tor.log |
| `/data/backups/caslink/` | backups |

### Compose variants

| File | Mode | AI use? | Bind |
|---|---|---|---|
| `docker-compose.yml` | production, no MODE/DEBUG | No — human only | `172.17.0.1:{port}:80` |
| `docker-compose.dev.yml` | `MODE: dev`, `DEBUG: 1` | No — human only | plain port, all interfaces |
| `docker-compose.test.yml` | `MODE: dev`, `DEBUG: 1`, ephemeral cache | Yes — AI/CI | `172.17.0.1:{port}:80` |

### Image tags

| Tag | Use |
|---|---|
| `ghcr.io/webappsgo/caslink:latest` | latest stable |
| `ghcr.io/webappsgo/caslink:{version}` | pinned release |
| `ghcr.io/webappsgo/caslink:{YYMM}` | year/month |
| `ghcr.io/webappsgo/caslink:{commit7}` | git commit |
| `caslink:dev`, `caslink:test` | local only, never pushed |

### Required OCI labels (applied by CI, not Dockerfile)

`maintainer`, `org.opencontainers.image.{vendor,authors,title,base.name,description,licenses,created,version,schema-version,revision,url,source,documentation,vcs-type}`, `com.github.containers.toolbox`.

### Standard vs AIO

| | Standard | AIO |
|---|---|---|
| Dockerfile | `docker/Dockerfile` | `docker/Dockerfile.aio` |
| Runtime base | alpine | debian |
| DB | external service | embedded postgres |
| Cache | external service | embedded valkey |
| Tag | `:latest` | `:latest-aio` |
| Use when | scaling needed | simple self-contained deploy |

For complete details, see AI.md PART 27
