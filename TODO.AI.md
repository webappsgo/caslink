# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

## CI/CD workflows (PART 28 — gap)

- `.github/workflows/` is empty. PART 28 requires security workflow(s) plus
  `ci.yml` / `release.yml`. Create security-only workflows first, then
  `ci.yml`/`release.yml` last, per cicd_conventions.md. Third-party Actions
  pinned to full commit SHA, never tags.
