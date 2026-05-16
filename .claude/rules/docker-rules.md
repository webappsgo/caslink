# Docker Rules (PART 27)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Put Dockerfile in project root (ALWAYS `docker/Dockerfile`)
- Put docker-compose.yml in project root (ALWAYS `docker/docker-compose.yml`)
- Use .env files with docker-compose (hardcode sane defaults)
- Override ENTRYPOINT or CMD directly (customize via entrypoint.sh)
- Use non-alpine base images without justification
- Run docker-compose in project root (use temp directory workflow)

## CRITICAL - ALWAYS DO
- Multi-stage Dockerfile: builder (golang:alpine) + runtime (alpine:latest)
- ENTRYPOINT: `["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]`
- STOPSIGNAL: SIGRTMIN+3
- Required packages: `git`, `curl`, `bash`, `tini`, `tor`
- Default timezone: America/New_York (override with TZ env var)
- Internal port: 80 (app listens on 0.0.0.0:80)
- External port: 64580 mapped to internal 80: `-p 64580:80`
- OCI labels (see below)
- docker-compose.yml works with zero .env (hardcoded sane defaults)

## CONTAINER PORT
- Inside container: app --address 0.0.0.0 --port 80
- Docker mapping: -p 64580:80
- Override: PORT env var (change internal port)

## VOLUME MOUNTS
```yaml
volumes:
  - './volumes/config:/config:z'
  - './volumes/data:/data:z'
```

## REQUIRED OCI LABELS
```dockerfile
LABEL org.opencontainers.image.title="caslink"
LABEL org.opencontainers.image.description="..."
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.source="https://github.com/casapps/caslink"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.vendor="casapps"
LABEL org.opencontainers.image.authors="casapps"
```

---
For complete details, see AI.md PART 27
