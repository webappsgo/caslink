# Project SPEC Overrides — caslink

This file records project-specific rule overrides and optional-PART activation
for caslink. Per `.claude/rules/ai-rules.md`, hierarchy is `SPEC.md` > `AI.md` >
global `CLAUDE.md`. An empty section means no override.

## Optional PART Activation (34-36) — FLIPPED TO REQUIRED

Per the `OPTIONAL → REQUIRED FLIP MECHANISM (PARTS 34-36)` in `AI.md`
(around line 54781), the following optional PARTs have been formally flipped to
`REQUIRED - NON-NEGOTIABLE` for caslink. The flip is recorded consistently across
all three authoritative locations, as the mechanism requires:

| PART | Feature | AI.md heading | IDEA.md variable |
|------|---------|---------------|------------------|
| 34 | Multi-User | `(REQUIRED - NON-NEGOTIABLE)` | `multi_user: true` |
| 35 | Organizations | `(REQUIRED - NON-NEGOTIABLE)` | `organizations: true` |
| 36 | Custom Domains | `(REQUIRED - NON-NEGOTIABLE)` | `custom_domains: true` |

Rationale: caslink's `IDEA.md ## Business logic` section ships end-user accounts,
organizations (up to 5 per user; roles owner/admin/member), and custom domains
(up to 5 per user, 20 per org; DNS TXT verification; Let's Encrypt SSL) as core
product features. Under the flip rules these features are therefore mandatory.

Consequences (per the flip mechanism's own rules):

- Once flipped to REQUIRED, each PART is mandatory in full — every requirement,
  table, and rule in AI.md PART 34/35/36 applies with no partial implementation.
- The flip is one-way: caslink cannot un-flip without a data migration.
- `TEMPLATE.md` (the master) is never changed; only this project's AI.md, IDEA.md,
  and this SPEC.md record the flip.

## Other Rule Overrides

None. Where this file is silent, `AI.md` governs unchanged.
