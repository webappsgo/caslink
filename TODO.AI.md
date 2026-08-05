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

- `server.config.trusted_proxies.additional` entries that are hostnames (not
  IPs/CIDRs) are not DNS-resolved before matching. `isTrustedPeer()` compares
  the raw TCP peer IP against parsed IP/CIDR entries only, so a hostname entry
  silently never matches. Resolving hostnames on the request path would put a
  blocking DNS lookup in front of every request; doing it correctly needs a
  background resolver with a TTL cache refreshed on the 5-minute proxy-refresh
  cycle (per config-rules "refreshed every 5 min"). Deferred — needs that
  resolver design; IP/CIDR entries (the documented common case) work today.
