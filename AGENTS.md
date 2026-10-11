# Project SPEC

Project: caslink
Role: Efficient loader for AI.md

⚠️ **THIS FILE IS AUTO-LOADED EVERY CONVERSATION. FOLLOW IT EXACTLY.** ⚠️

Purpose:
- This file is a short loader for the most important rules
- `AI.md` is the full source of truth
- For complete details, read the referenced PARTs in `AI.md`

## FIRST TURN - MANDATORY

On EVERY new conversation or after "context compacted" message:
1. **READ** the relevant `.claude/rules/*.md` for your current task
2. **NEVER** assume or guess - verify against AI.md before implementing

## Asking Questions

- **Default to continuing work** - do not stop just to ask whether you should continue; if the next step is implied by the spec, the current task, or the current findings, continue
- **Never guess** - if the answer cannot be determined from `AI.md`, `IDEA.md`, the codebase, or repo state **and** the missing information materially changes behavior, scope, or safety, ASK the user
- **Do NOT ask for permission to keep going** - continue until the current task is complete, blocked by a real decision, or the user explicitly asks to pause
- **Question mark = question** - when user ends with `?`, answer/clarify, don't execute
- **Use AskUserQuestion wizard** - presents one question at a time with options + "Other" for custom input + Submit/Cancel; layout varies by context (yes/no, multi-select, with descriptions); less overwhelming than plain text questions

**Ask only when at least one of these is true:**
1. A required business/product decision is missing
2. Two or more reasonable implementations would produce materially different behavior
3. The action is destructive, irreversible, or impacts production/user data beyond normal safe development work
4. The spec explicitly says to ask or confirm
5. The user explicitly requested a plan, pause, or checkpoint before execution

**Do NOT ask just to confirm routine continuation:**
- after finishing one obvious sub-step and the next step is clear
- before running normal repo validations/checks already implied by the task
- before making tightly related follow-up edits required to keep the spec internally consistent
- before updating related docs/checklists/examples required by the same change

## Before ANY Code Change

1. Have I read the relevant PART in AI.md? (If no → read it)
2. Does this follow the spec EXACTLY? (If unsure → check spec)
3. Am I guessing or do I KNOW from the spec? (If guessing → read spec)
4. Would this pass the compliance checklist? (AI.md FINAL section)

**WHEN IN DOUBT: READ THE SPEC. DO NOT GUESS.**

## Binary Terminology
- **server** = `caslink` (main binary, runs as service)
- **client** = `caslink-cli` (REQUIRED companion, CLI/TUI/GUI)
- **agent** = `caslink-agent` (optional, runs on remote machines)

## Key Placeholders
- `{project_name}` = caslink
- `{project_org}` = webappsgo
- `{admin_path}` = admin

## Account Types (CRITICAL)
- **Server Admin** = manages the app (NOT a privileged OS user)
- **Primary Admin** = first admin, cannot be deleted
- **Regular User** = end-user (PART 34, optional feature)
- Server Admins ≠ Regular Users (separate DB tables)

## Cluster vs Managed Nodes (CRITICAL)
- **Cluster Node** = another instance of THIS app (horizontal scaling)
- **Managed Node** = EXTERNAL resource app controls/monitors (Docker hosts, etc.)
- Most apps only have cluster nodes

## NEVER Do (Top 19) - VIOLATIONS ARE BUGS
1. Use bcrypt → Use Argon2id
2. Put Dockerfile in root → `docker/Dockerfile`
3. Use CGO → CGO_ENABLED=0 always
4. Hardcode dev values → Detect at runtime
5. Use external cron → Internal scheduler (PART 19)
6. Store passwords plaintext → Argon2id (tokens use SHA-256)
7. Create premium tiers → All features free, no paywalls
8. Use Makefile in CI/CD → Explicit commands only
9. Guess or assume values that a command can produce → Run the command (`date`, `basename "$PWD"`, `git config user.email`, `git rev-parse --short HEAD`, `uname -m`, etc.) — when no command applies, read spec or ask user
10. Skip platforms → Build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
11. Client-side rendering (React/Vue) → Server-side Go templates
12. Require JavaScript for core features → Progressive enhancement only
13. Let long strings break mobile → Use word-break CSS
14. Skip validation → Server validates EVERYTHING
15. Implement without reading spec → Read relevant PART first
16. Modify TEMPLATE.md or AI.md content → READ-ONLY. Project changes go in IDEA.md. To activate optional PARTS 34-36 for this project, declare them in SPEC.md (see "SPEC.md — Optional Part Activation" below)
17. Edit `## Project variables` in IDEA.md without confirming with the user → Variables drive placeholder resolution used by AI.md; wrong values silently corrupt every reference
18. Read an image larger than 1000×1000 directly into context → Resize to ≤1000×1000 first via the fallback chain (`magick` → `convert` → `gm convert` → `vipsthumbnail` → `sips` → `ffmpeg`). If none are available, do NOT read the image — tell the user which tool to install. See "Large Image Handling" in AI.md PART 0.
19. Use a non-conforming IDEA.md without migration → If IDEA.md exists but lacks the three required sections (`## Project description`, `## Project variables`, `## Business logic`), STOP and migrate it before doing anything else. See "IDEA.md Migration" in AI.md PART 0.

## ALWAYS Do - NON-NEGOTIABLE
1. Read AI.md before implementing ANY feature
2. Server-side processing (server does the work, client displays)
3. Mobile-first responsive CSS
4. All features work without JavaScript
5. Tor hidden service support (auto-enabled if Tor found)
6. Built-in scheduler, GeoIP, metrics, email, backup, update
7. Full admin panel with ALL settings
8. Client binary for ALL projects
9. Commit often — small, focused commits. Do NOT hoard unrelated changes into one big commit. **Findings-based work (audits, reviews, numbered fix-lists) defaults to one commit per finding** — never batch distinct findings into one commit just because they share a file or session. **Feature work is the opposite — one commit for the whole feature, never split per part. Unrelated bugs found mid-feature go to `TODO.AI.md`, except app-breaking bugs, which must be fixed immediately.** **Subagents do not commit** — complete edits and report back to the parent instance; the parent reviews the diff and owns the commit.

## File Locations
- Config: `{config_dir}/server.yml`
- Data: `{data_dir}/`
- Logs: `{log_dir}/`
- Source: `src/`
- Docker: `docker/`

## Where to Find Details
- AI behavior: `.claude/rules/ai-rules.md` (PART 0, 1)
- Project structure: `.claude/rules/project-rules.md` (PART 2, 3, 4)
- Config/modes: `.claude/rules/config-rules.md` (PART 5, 6, 12)
- Binary/CLI: `.claude/rules/binary-rules.md` (PART 7, 8, 33)
- Backend: `.claude/rules/backend-rules.md` (PART 9, 10, 11, 32)
- API/SSL: `.claude/rules/api-rules.md` (PART 13, 14, 15)
- Frontend/WebUI: `.claude/rules/frontend-rules.md` (PART 16, 17)
- Features: `.claude/rules/features-rules.md` (PART 18-23)
- Service/privilege: `.claude/rules/service-rules.md` (PART 24, 25)
- Makefile: `.claude/rules/makefile-rules.md` (PART 26)
- Docker: `.claude/rules/docker-rules.md` (PART 27)
- CI/CD: `.claude/rules/cicd-rules.md` (PART 28)
- Testing/docs/i18n: `.claude/rules/testing-rules.md` (PART 29, 30, 31)
- Optional features: `.claude/rules/optional-rules.md` (PART 34-36)
- Full spec: `AI.md` (~62k lines) ← **SOURCE OF TRUTH**

## Current Project State
[AI updates this section as work progresses]
- Last read AI.md: 2026-07-30
- Current task: bootstrap — created `.claude/rules/*.md` and this loader per AI.md PART 0
- Relevant PARTs: 0-6 (bootstrap scope)
