# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

- src/metrics/metrics.go `normalizePath()`: the third alternative in `idPattern`
  (`[A-Za-z0-9_-]{3,50}`) matches ANY path segment of 3-50 word chars/hyphens,
  not just short dynamic slugs — so ordinary static route segments like
  `/users`, `/orgs`, `/api` are also collapsed to `/:code`, contradicting the
  function's own doc comment ("Segments that look like UUIDs or pure integers
  become :id; short alphanumeric slugs become :code"). Discovered while
  writing `src/metrics/normalize_test.go`. Needs a product decision on
  intended semantics (e.g. a known-static-segment allowlist, or accepting
  this as deliberate max-caution behavior) before changing the regex —
  left unfixed per test-writing task scope.

- src/config/bool.go (lines 12, 16, 26, 50, 63): `ParseBool`/`IsTruthy`/
  `IsFalsy` implement a much shorter truthy/falsy word list than the one
  documented in `.claude/rules/config-rules.md` and
  `.claude/rules/testing-rules.md`. Implemented truthy: `y t yep yup yeah
  aye si oui`; documented truthy adds `1 yes true on ok enable enabled da
  hai affirmative accept allow grant sure totally`. Implemented falsy: `n f
  nope nah nay nein non`; documented falsy adds `0 no false off disable
  disabled niet iie lie negative reject block revoke deny never noway`.
  Discovered while writing `src/config/bool_test.go` (tests were written
  against actual implemented behavior, not the spec, per test-writing task
  scope). Needs a decision on whether to expand `bool.go` to match the
  documented word list — left unfixed since this is a behavior change to
  production code, out of scope for a test-only pass.

- src/server/middleware.go `URLNormalizeMiddleware` (line ~676): the
  "file-like path" trailing-slash exemption can never actually fire. The
  code takes `last := p[strings.LastIndex(p, "/"):]` to find the final path
  segment and checks it for a `.`, but this only runs when `p` already ends
  in `/` — so the last `/` found IS the trailing slash itself, making `last`
  always exactly `"/"`, which never contains a dot. A request like
  `/static/app.css/` is therefore always 301-redirected to
  `/static/app.css`, contradicting the function's own doc comment
  ("Requests for paths that end with a file extension are exempt"). The
  intended fix is likely trimming the trailing slash before computing
  `last`. Discovered while writing `src/server/middleware_test.go` (test
  `TestURLNormalizeMiddlewareFileLikeTrailingSlashStillRedirects` locks in
  the actual current behavior) — left unfixed since it's a behavior change
  to production code, out of scope for a test-only pass.

- src/client/cli/commands.go `doWithFailover()` (lines ~144-177): the doc
  comment states "Only fail-over on connection errors, not on HTTP 4xx/5xx
  from the server," but the implementation never actually distinguishes
  error types — `c.do()` returns a non-nil `error` both for real connection
  failures AND for a successfully-decoded `{"ok":false,...}` business-error
  response, and `doWithFailover` sends any non-nil error into the
  cluster-failover loop. A 4xx/5xx from the primary therefore still tries
  every configured cluster member, and returns success if any of them
  happens to answer differently for the same path — silently masking a
  legitimate "not found"/"forbidden"/etc. response from the primary.
  Discovered while writing `src/client/cli/commands_test.go` (test
  `TestDoWithFailover_4xxNotRetried` locks in the actual current behavior).
  Needs a design decision on how to classify "connection error" vs.
  "business error" in `do()` (e.g. a typed/sentinel error, or checking the
  HTTP status code before treating a decoded response as a failover
  trigger) — left unfixed since it's a behavior change to production code,
  out of scope for a test-only pass.
