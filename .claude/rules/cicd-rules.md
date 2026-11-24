# CI/CD Workflow Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

**This project is hosted on GitHub (`webappsgo/caslink` → github.com), so GitHub Actions is the active CI system. AI.md PART 28 also requires complete equivalent workflows for GitLab, Gitea, Forgejo, and Jenkins in every project.**

## CRITICAL - NEVER DO

- Never use Makefile targets in CI/CD — always explicit `go build`/`go test` commands with visible flags
- Never reference local host paths (`~/.local/share/go`, `~/go/pkg/mod`) in CI caching — use CI-native caching or `/tmp/`
- Never depend on local Docker containers for CI builds — GitHub Actions jobs run natively inside `container: image: casjaysdev/go:latest`
- Never skip workflow concurrency/auto-cancel on branch-push workflows targeting `main`, `master`, `devel`, `dev`, or `beta`
- Never let a newer tag-release run cancel a different tag's run — `release.yml` concurrency must be scoped to the exact tag ref
- Never use `github.event.default_branch` for secret-scan diff range — use `github.event.before`/`github.event.after` (default_branch resolves to HEAD after push and silently skips the scan)
- Never install tools inline in CI jobs — use `container: image: casjaysdev/go:latest` for Go jobs
- Never pin a third-party Action to a floating tag — always pin to a full commit SHA (with a `# vX.Y.Z` comment)
- Never bake OCI `LABEL`s into the Dockerfile in a CI-built image — pass `labels:`/`annotations:` via `docker/metadata-action` and `docker/build-push-action`
- Never skip the coverage gate (60% threshold, enforced in `ci.yml`'s `test` job)

## CRITICAL - ALWAYS DO

- Always set `VERSION`, `COMMIT_ID`, and `BUILD_EPOCH` explicitly in a "Set build info" step, never as static `env:`; derive human-readable `BUILD_DATE` from the epoch where needed
- Always build the full 8-platform matrix for release/beta/devel workflows: linux/darwin/windows/freebsd × amd64/arm64
- Always add `concurrency: { group: <workflow>-${{ github.ref }}, cancel-in-progress: true }` (or ref-conditional cancel for devel/docker) to every branch/tag workflow
- Always run `ci.yml` security jobs (`secret-scan`, `workflow-policy`, `vuln-scan`, `image-scan`) on push, PR, AND weekly cron (`0 6 * * 1`); guard non-security jobs with `if: github.event_name != 'schedule'`
- Always use truffleHog (Apache-2.0) for mandatory secret scanning on every public repo
- Always build CLI/Agent binaries conditionally (`if: hashFiles('src/client/') != ''` / `src/agent/`)
- Always upload artifacts per-platform, then merge in the `release` job via `download-artifact` with `merge-multiple: true`
- Always create `version.txt` and a source tarball (excluding `.git`, `.github`, `.gitea`, `binaries`, `releases`) as part of the release job
- Always build Docker images for `linux/amd64,linux/arm64` via `docker buildx`, pushing to `ghcr.io`
- Always tag Standard images without a suffix and All-in-One images with `-aio`

## Key Rules

### Required workflow files (GitHub)

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | push/PR to default branch + weekly cron (security jobs) | build, test, lint, coverage, secret-scan, image-scan, workflow-policy |
| `release.yml` | tag push `v*` / `*.*.*` | stable release, all 8 platforms |
| `beta.yml` | push to `beta` | beta release, timestamp-beta version |
| `devel.yml` | schedule 3am UTC + push main/master | rolling `devel` release (max 1 kept) |
| `docker.yml` | branch/tag pushes and schedule | standard multi-arch image build+push |
| `docker-aio.yml` | branch/tag pushes and schedule | AIO multi-arch image build+push |

### Concurrency policy

| Workflow type | Group key | Cancel behavior |
|---|---|---|
| Branch push (`ci`, `beta`, `devel`, `docker`) | `<name>-${{ github.ref }}` | cancel older run for same ref (`main`/`master`/`devel`/`dev`/`beta`) |
| Tag release (`release.yml`) | `release-${{ github.ref }}` | cancel only the same exact tag ref, never cross-tag |

### Version/build-info step (every workflow)

```
VERSION: explicit input, release.txt, or provider-specific derived version
COMMIT_ID: git rev-parse --short=7 HEAD
BUILD_EPOCH: UTC Unix epoch embedded in binaries; BUILD_DATE is derived from it for release metadata
OFFICIAL_SITE: site.txt wins, else secrets.OFFICIAL_SITE, else empty (never guessed)
```

### ci.yml jobs (all run in `container: image: casjaysdev/go:latest`)

`lint` (go vet + staticcheck) → `test` (coverage ≥60%, gate) → `build` (needs lint+test) → `vuln-scan` (govulncheck), plus `secret-scan`, `workflow-policy`, and independent `image-scan`; `coverage` needs test and `upload-artifacts` needs build.

### Docker image tag matrix

| Trigger | Standard tags | AIO tags |
|---|---|---|
| Any push | `devel`, `{commit7}` | `devel-aio`, `{commit7}-aio` |
| Push to `beta` | + `beta` | + `beta-aio` |
| Version tag | `{version}`, `latest`, `{YYMM}` | `{version}-aio`, `latest-aio`, `{YYMM}-aio` |

### Cross-provider equivalence (all five providers are required)

| Feature | GitHub Actions | Gitea/Forgejo | GitLab CI | Jenkins |
|---|---|---|---|---|
| Config location | `.github/workflows/*.yml` | `.gitea/` or `.forgejo/workflows/*.yml` | `.gitlab-ci.yml` | `Jenkinsfile` |
| Stable | `release.yml` | `release.yml` | `rules: tag` | `BUILD_TYPE == 'release'` |
| Beta | `beta.yml` | `beta.yml` | `rules: beta` | `BUILD_TYPE == 'beta'` |
| Devel | `devel.yml` | `devel.yml` | `rules: schedule` | `BUILD_TYPE == 'devel'` |
| Docker | `docker.yml` | `docker.yml` | `docker:build` | Docker stage |
| Context vars | `github.*` | `gitea.*` / `forgejo.*` | `$CI_*` | Groovy vars |
| Secrets | `${{ secrets.X }}` | `${{ secrets.X }}` (GITEA_TOKEN/FORGEJO_TOKEN) | `$X` | `credentials('id')` |

For complete details, see AI.md PART 28
