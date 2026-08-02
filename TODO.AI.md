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

- src/server/service/url.go (~lines 42-44): `CreateURL` wraps the
  `url.ParseRequestURI` error for a malformed/invalid `long_url` in a plain
  `fmt.Errorf` instead of a `model.Err*` sentinel, so the handler's
  error-mapping switch in `src/server/handler/url.go` (~lines 98-113) falls
  through to `500 SERVER_ERROR` instead of `400 BAD_REQUEST` for this input.
  Not security-relevant, just the wrong status code for a client error.
  Discovered while writing `src/server/handler/url_test.go`
  (`TestCreateURLMalformedTarget` locks in the actual current behavior).
  Needs `CreateURL` to return a `model.ErrValidation`-style sentinel for
  this case instead of a bare wrapped error — left unfixed since it's a
  behavior change to production code, out of scope for a test-only pass.

- src/server/service/admin_users.go `ForceRegenerateRecoveryKeys()`
  (~lines 179-230): never verifies the target user actually exists before
  deleting/inserting `recovery_keys` rows for the given `user_id` — calling
  it for a nonexistent user ID returns `200 OK` with a freshly generated,
  orphaned set of recovery keys instead of a not-found error. Discovered
  while writing `src/server/handler/admin_test.go`
  (`TestAPIRegenerateRecoveryKeysMissingUserStillSucceeds` locks in the
  actual current behavior). Needs a user-existence check added (e.g. a
  `GetUser` lookup before the transaction) — left unfixed since it's a
  behavior change to production code, out of scope for a test-only pass.

- src/server/handler/org.go (32 call sites): nearly every error response
  calls `respondJSON(w, status, map[string]string{"error": "..."})` instead
  of `respondError`. `respondJSON` (src/server/handler/helpers.go:102-106)
  unconditionally wraps its argument as `{"ok":true,"data":...}`, so these
  error bodies come back as `{"ok":true,"data":{"error":"..."}}` — `ok:true`
  even for 400/403/404/500 responses — instead of the canonical
  `{"ok":false,"error":"CODE","message":"..."}` shape required by
  `.claude/rules/api-rules.md`. Discovered while writing
  `src/server/handler/org_test.go` (`decodeErrorEnvelope` and
  `TestAPICreateOrgInvalidSlug` lock in the actual current shape). Needs
  every error call site in org.go switched from `respondJSON` to
  `respondError` — left unfixed since it's a wide behavior change to
  production code, out of scope for a test-only pass.

  Addendum: two success-path call sites in the same file compound the
  problem in the other direction — `APICreateOrgToken` (line ~556) and
  `APIListOrgTokens` (line ~595) each pass an already-wrapped
  `map[string]interface{}{"ok":true, ...}` into `respondJSON`, which wraps
  it again, directly contradicting `respondJSON`'s own doc comment
  ("callers MUST NOT pre-wrap data themselves" — helpers.go:100-101). The
  plaintext token from `APICreateOrgToken` and the token list from
  `APIListOrgTokens` both land one level deeper than the canonical shape
  (`.data.token` / `.data.data` instead of `.token` / `.data`). Discovered
  while writing `src/server/handler/org_test.go`
  (`TestAPICreateOrgTokenSuccessOwner`, `TestAPIListOrgTokensSuccess` lock
  in the actual current shape) — left unfixed for the same reason as
  above.

- src/server/handler/user_security.go `renderTOTPSetup()` (~line 340-361):
  `QRDataURL` is passed to the template as a plain `string`, but the 2FA
  template (`template/page/users/security/2fa.html:47`) renders it inside an
  `<img src="{{.QRDataURL}}">` attribute. html/template's URL sanitizer
  (`isSafeUrl` in `html/template/url.go`) only allows `http`/`https`/`mailto`
  schemes for a plain string in a URL-attribute context, so the `data:`
  scheme is rejected and the entire attribute value is replaced with the
  `#ZgotmplZ` safety placeholder — the QR code image never actually renders
  for any user setting up 2FA. Discovered while writing
  `src/server/handler/user_security_test.go`
  (`TestTwoFactorEnableCorrectPasswordShowsQR` locks in the actual current
  behavior — asserts `#ZgotmplZ` appears, not the data URI). Needs
  `QRDataURL` typed as `template.URL` (mirroring `KeysJSON template.JS` a
  few lines down in the same file) so html/template treats it as
  pre-vetted-safe — left unfixed since it's a behavior change to production
  code, out of scope for a test-only pass.

- src/server/handler/user_security.go (27 call sites, e.g. lines 492, 511,
  517, 533, 541, 593, 601, 608, 638, 646, 653, 661, 684, 692, 698, 706, 713,
  720): the same `respondJSON`-instead-of-`respondError` misuse already
  logged for `src/server/handler/org.go` above, independently present in
  this file too. Every error path calls
  `respondJSON(w, status, map[string]interface{}{"ok": false, "error": "CODE", ...})`,
  but `respondJSON` (helpers.go:100-106) unconditionally wraps its argument
  again as `{"ok":true,"data":{...}}}`, contradicting its own doc comment
  ("callers MUST NOT pre-wrap data themselves"). Every error response from
  this file therefore reports top-level `ok:true` with the real
  `ok`/`error`/`message` fields nested one level deeper at `.data.*`.
  Discovered while writing `src/server/handler/user_security_test.go`
  (`TestPasskeyActionWebAuthnNotConfigured`,
  `TestPasskeyActionDeleteNoSuchCredentialFails` lock in the actual current
  shape). Needs every error call site in this file switched from
  `respondJSON` to `respondError` — left unfixed since it's a wide behavior
  change to production code, out of scope for a test-only pass.

- src/updater/update.go `DoUpdateFor()` (lines 97-164): the success path
  from a verified download through `replaceBinary` (lines 148-163) is not
  covered by any test. `currentPath` comes from `os.Executable()` with no
  override, so exercising this branch in a unit test would rename over the
  actual running `go test` binary — unsafe and avoided deliberately (see
  `src/updater/update_test.go`
  `TestDoUpdateFor_ChecksumMismatchStopsBeforeReplace`, which verifies the
  pipeline up to but not through `replaceBinary`). `replaceBinary` itself is
  separately unit-tested end-to-end with synthetic temp files in
  `src/updater/replace_unix_test.go`. Would need `DoUpdateFor` to accept an
  injectable "current binary path" (or a seam around `os.Executable`) to
  close this gap safely. Left unfixed — behavior change to production code,
  out of scope for a test-only pass.

- freebsd/arm64 cross-compile is broken, failing the Daily Build workflow's
  8-platform matrix (pre-existing — confirmed failing on commits before this
  test-writing pass, not a regression it introduced):
  - `src/backup/disk_unix.go` (`//go:build linux || darwin`) has no freebsd
    variant, so `src/backup/retention.go:202`'s call to `diskCapacity` is
    `undefined` on freebsd.
  - `src/metrics/disk_unix.go` (`//go:build linux || darwin || freebsd`,
    line 14) does `stat.Bavail * bsize` assuming `Bavail` is `uint64`, but on
    freebsd/arm64 `syscall.Statfs_t.Bavail` is `int64`, so the multiplication
    fails with "mismatched types int64 and uint64".
  Needs `src/backup/disk_unix.go`'s build tag extended to include freebsd (or
  a dedicated `disk_freebsd.go`), and `src/metrics/disk_unix.go` to convert
  `stat.Bavail` to `uint64` before multiplying (guarding against negative
  values from the signed freebsd field). Left unfixed — not app-breaking on
  the primary Linux target, out of scope for a test-only pass.
