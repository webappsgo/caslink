# TODO.AI.md

Deferred items surfaced by the 2026-07 project health audit. Each is either
feature-sized, carries real regression risk, or needs a design decision — so it
was logged here rather than fixed inline during the audit. All small, safe
findings from that audit were fixed directly and are not listed here.

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
