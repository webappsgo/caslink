# AI Assistant Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Guess or assume a requirement, file location, default value, or user intent — STOP and ASK instead
- Claim a task is "done" without reading, searching, testing, and verifying first
- Edit `AI.md` (PARTS 0-36 + 37 reference) — it is READ-ONLY; project-specific overrides go in `SPEC.md`, business logic in `IDEA.md`
- Create report/analysis files (`AUDIT.md`, `COMPLIANCE.md`, `SUMMARY.md`, etc.) — fix issues directly instead. Temporary `AUDIT.AI.md` is allowed only during an explicit audit finding >5 issues, and must be deleted when resolved
- "Improve", "optimize", refactor, or rename things the user did not ask about ("this pattern is better" / "this format is cleaner" — always follow spec instead)
- Add unrequested features, invented flags, invented defaults, or invented directory structures
- Run `go` commands directly on the local machine — the local machine has no Go installed; ALL builds/tests use Docker (`casjaysdev/go:latest`) or Incus
- Use plain `git commit` / `git push` — use `gitcommit` only, and only after writing AND re-reading `.git/COMMIT_MESS`
- Let a subagent write `.git/COMMIT_MESS` or call `gitcommit` — only the parent instance commits
- Read an image larger than 1000×1000 directly — resize to `${TMPDIR:-/tmp}` first and read the copy
- Treat a non-conforming `IDEA.md` (missing the 3 required sections) as authoritative without running the migration procedure
- Add attribution trailers, footers, or comments that reference an AI tool anywhere in code, commits, PRs, or docs
- Use `SELECT *` in application code — always name columns explicitly
- Leave inline comments (same line as code) — comments always go ABOVE the code
- Use bare `/path` in embedded code (Go/JS/templates/emails) — always use `{fqdn}/path` via `BuildURL(r, ...)`
- Jump between half-finished features — complete one thing fully before starting the next
- Skip a mandatory PART section because the project "seems simple" — every project implements the full spec

## CRITICAL - ALWAYS DO

- Read the relevant AI.md PART(s) before implementing each task — not the whole file speculatively
- Read a file before editing it — no exceptions
- Search for existing patterns before creating something new
- Test changes and verify output before claiming completion
- Ask numbered/lettered clarifying questions when the spec is silent, ambiguous, or contradictory
- Check `.claude/rules/` exists at session start; create/update rule files if missing or if AI.md is newer
- Check `IDEA.md`'s three-section format (`## Project description`, `## Project variables`, `## Business logic`) before treating it as authoritative; migrate (with backup + user approval) if not conforming
- Translate every new piece of user-facing text (errors, admin pages, CLI help, notifications) — never hardcode English
- Verify self-work with a real tool appropriate to the change type (curl, `make test`, browser, DB rollback, etc.) — "looks right" is not verification
- Remove completed items from `TODO.AI.md` individually as each is resolved and committed; never truncate the whole file at once
- Use `curl -q -LSsf {url}` as the standard curl invocation everywhere (docs, scripts, CI, Makefiles)
- Keep README.md, Swagger, GraphQL, docs/, and CLI `--help` in sync with actual code after every change
- Stop and ask before any destructive or architecturally significant action

## Key Rules

**AI.md vs IDEA.md**

| File | Purpose | Modify? |
|------|---------|---------|
| AI.md (PARTS 0-37) | HOW — implementation patterns, standards | NEVER |
| IDEA.md | WHAT — business logic, features | YES |
| SPEC.md | Project-specific rule overrides / optional PART activation (34-36) | YES |

Hierarchy: `SPEC.md` > `AI.md` > global `CLAUDE.md`.

**Mandatory workflow per task:** identify relevant PART(s) → read completely → implement exactly → follow "See PART X" references and return → re-verify against spec every 3-5 changes.

**Session Start (first read on a project with AI.md):**
1. Read existing `CLAUDE.md` / `.claude/CLAUDE.md`
2. Migrate project content into `IDEA.md` if missing
3. Check `.claude/rules/` exists; create/update if missing or stale
4. Create `CLAUDE.md` loader if missing
5. Read `TODO.AI.md` and `TODO.md` if present
6. Commit COMMIT/NEVER/MUST rules to memory

**Rule files required (14 total):** `.claude/rules/{ai,project,config,binary,backend,api,frontend,features,service,makefile,docker,cicd,testing,optional}-rules.md` — see AI.md PART 0 table for the PART→file mapping. Each file needs: header with PART numbers, NON-NEGOTIABLE warning, `## CRITICAL - NEVER DO`, `## CRITICAL - ALWAYS DO`, key rules summary, footer reference.

**Verification checklist before saying "done":** read files → searched patterns → tested → verified output → certain it's correct → did not guess → did not rush → asked if unsure. Any "no" = not done.

**Full Web Application Architecture (PART 1):** every feature ships through 4 clients — Browser (HTML), PWA (HTML), API/automation (JSON), CLI (`{project_name}-cli`, text/JSON). Every web route has a matching `/api/{api_version}/...` route.

**Container-only development:**

| Task | Container |
|------|-----------|
| Build | Docker `casjaysdev/go:latest` |
| Unit tests | Docker `casjaysdev/go:latest` |
| Full OS/systemd testing | Incus `debian:latest` (preferred) |

**Security-first design (PART 1):** never trust input, defense in depth, least privilege, fail secure, secure by default, internet-facing baseline assumed. Security is suggested (MFA prompts), never forced/blocking. Never weaken authn/authz, TLS, CSRF/CSP/CORS, rate limiting, or input validation to "improve usability."

**Rate limiting defaults:** Read 120/min, Write 10/min, Health 120/min, global burst 240/min. Login 5/15min, password reset 3/hour, registration 5/hour, upload 10/hour.

**Naming conventions:** files `lowercase_snake.go`, packages `lowercase`, public `PascalCase`, private `camelCase`, interfaces `PascalCase`+`-er`. Names must be intent-revealing — never generic `Mode`, `Type`, `Status`, `Config`, `Get()`, `Init()` without a qualifying prefix (e.g. `AppMode`, `IsAppModeDev()`).

**Formatting:** Go = tabs; HTML/JSON/YAML/CSS/JS = 2 spaces (120 col); Makefile = tabs (180 col); shell = 2 spaces (180 col). Every file ends with exactly one trailing newline (exceptions: raw secret files, verbatim-interpolated files, binary artifacts).

**URL standards:** documentation uses `{official_site}/path`; embedded runtime code (Go/JS/templates/email) uses `{fqdn}/path` via `BuildURL(r, ...)` (proxy-aware) — never a hardcoded `https://` string, never a bare `/path` outside internal router registration.

**README.md section order (mandatory):** Title/Badges → About → Official Site → Features → Production → Client → Configuration → API → Other → Development (always last) → Disclaimer → License. Badges must always be linked (`[![alt](img)](link)`), never bare images.

**Sensitive data:** never expose credentials, connection strings, internal paths/IPs, or config internals in healthz, API responses, error messages, logs, or HTML. Tokens/passwords shown only once at generation time.

**Resolved project values:** `project_name=caslink`, `project_org=webappsgo`, `internal_name=caslink`, `app_name=Caslink`, `official_site=https://caslink.casapps.us`.

For complete details, see AI.md PART 0, 1
