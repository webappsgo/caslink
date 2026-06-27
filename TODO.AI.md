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

User said "No — leave empty for now" on 2026-06-04.
**All workflow files were deleted in commit `39bc9bf174ee`.**
`.github/workflows/` is intentionally empty — do NOT recreate without
explicit user instruction.

---

## Federation (Out-of-scope for v1)

`FederationConfig` struct present; no service, no `/.well-known/caslink`,
no discovery or sync. Deferred by design — spec marks federation optional.

---

## Bootstrap Status

`.claude/rules/` regenerated from AI.md. All 14 rule files present:
- `ai-rules.md` (PART 0, 1)
- `project-rules.md` (PART 2, 3, 4)
- `config-rules.md` (PART 5, 6, 12)
- `binary-rules.md` (PART 7, 8, 33)
- `backend-rules.md` (PART 9, 10, 11, 32)
- `api-rules.md` (PART 13, 14, 15)
- `frontend-rules.md` (PART 16, 17)
- `features-rules.md` (PART 18-23)
- `service-rules.md` (PART 24, 25)
- `makefile-rules.md` (PART 26)
- `docker-rules.md` (PART 27)
- `cicd-rules.md` (PART 28)
- `optional-rules.md` (PART 34, 35, 36)
- `testing-rules.md` (PART 29, 30, 31)

---

Last refreshed: 2026-06-25
