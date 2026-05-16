# CI/CD Rules (PART 28)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use Makefile in CI/CD (explicit commands with env vars only)
- Float third-party actions on tags or branches (must pin to full SHA)
- Use `pull_request_target` for untrusted code execution
- Expose secrets to fork PRs
- Use `gitleaks` (requires commercial license) - use truffleHog instead
- Grant workflow-wide write permissions (grant per-job, minimum needed)

## CRITICAL - ALWAYS DO
- Pin all third-party actions to full commit SHA with comment showing version
- `permissions: contents: read` at workflow level (read-only baseline)
- Write permissions ONLY on release job that needs them
- Secret scan (truffleHog) on every push/PR - findings are blockers
- govulncheck when go.sum present
- Trivy image scan when Dockerfile present
- VERSION from release.txt when present, else fallback

## REQUIRED WORKFLOWS
| Provider | Location | Files |
|----------|----------|-------|
| GitHub | `.github/workflows/` | build.yml, release.yml, security.yml |
| Gitea | `.gitea/workflows/` | build.yml, release.yml, security.yml |
| Jenkins | `Jenkinsfile` | Declarative Pipeline |

Additional: beta.yml, daily.yml, docker.yml in each provider

## DOCKER TAGS
| Trigger | Tags |
|---------|------|
| Any push | `devel`, `{commit}` |
| Beta branch | adds `beta` |
| Tag push | `{version}`, `latest`, `YYMM`, `{commit}` |

## RELEASE INTEGRITY
- SHA-256 checksums for all release artifacts
- Release notes with actual change set
- SBOM (CycloneDX or SPDX JSON)
- Build provenance/attestation when platform supports it

## PINNED ACTION EXAMPLE
```yaml
- uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd  # v6.0.2
```

---
For complete details, see AI.md PART 28
