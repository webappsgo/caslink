# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

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
