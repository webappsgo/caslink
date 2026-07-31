# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

## GraphQL resolvers (PART 14 — REQUIRED)

- `src/graphql/` has `schema.go` + `graphql.go` but no `resolvers.go`.
  `executeQuery` was changed during the audit to return an honest
  `{"data":null,"errors":[{"message":"GraphQL resolvers are not yet implemented"}]}`
  instead of a fake healthy response. Implement real resolvers backed by the
  URL/user/org services so the REQUIRED GraphQL endpoint actually works.

## CI/CD workflows (PART 28 — gap)

- `.github/workflows/` is empty. PART 28 requires security workflow(s) plus
  `ci.yml` / `release.yml`. Create security-only workflows first, then
  `ci.yml`/`release.yml` last, per cicd_conventions.md. Third-party Actions
  pinned to full commit SHA, never tags.
