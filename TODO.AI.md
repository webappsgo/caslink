# TODO.AI.md — Caslink Outstanding Work

Tracks remaining spec gaps. Items removed once fully implemented and committed.

---

## PART 15 — DNS-01 Challenge (optional per spec)

DNS-01 Let's Encrypt challenge is optional per AI.md PART 15.
HTTP-01 and TLS-ALPN-01 are both implemented.
DNS-01 not implemented — deferred only because spec marks it optional.

---

## PART 12 — Config validation uses fmt.Printf instead of structured logger

`config.Validate()` emits warnings via `fmt.Printf`. The spec says
"warn and replace with default" — but doesn't prescribe the logging
mechanism. Using the app logger (from `logger.New()`) would be better
but requires restructuring the config load sequence since the logger is
initialized after config. Low priority.

---

## PART 28 — CI/CD Workflows

User said "No — leave empty for now" on 2026-06-04. All 6 workflow files
are now present:
- `.github/workflows/build-toolchain.yml` ✓
- `.github/workflows/ci.yml` ✓
- `.github/workflows/release.yml` ✓
- `.github/workflows/beta.yml` ✓
- `.github/workflows/daily.yml` ✓
- `.github/workflows/docker.yml` ✓

Build image `docker/Dockerfile.build` exists. Next step: trigger
`build-toolchain.yml` via `workflow_dispatch` to push the `:build`
image to ghcr.io, then CI will work.

---

## Federation (Out-of-scope for v1)

`FederationConfig` struct present; no service, no `/.well-known/caslink`,
no discovery or sync. Deferred by design — spec marks federation optional.

---

Last refreshed: 2026-06-17
