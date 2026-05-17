## Summary

<!-- What does this PR change? One paragraph max. -->

## Why

<!-- Why is this change needed? Link to the issue it closes if applicable. Closes #___ -->

## Test Evidence

<!-- How was this tested? Paste `go test` output, screenshot, or curl response. -->

## Documentation / Config Updates

- [ ] `AI.md` / `IDEA.md` updated (if behavior or spec changed)
- [ ] `README.md` updated (if user-visible behavior changed)
- [ ] Config keys documented (if new `server.yml` keys added)
- [ ] API docs updated (if new/changed endpoints)

## Breaking Changes

<!-- Does this break existing behavior, API contracts, config keys, or CLI flags? -->

- [ ] No breaking changes
- [ ] Breaking change — describe migration path below:

<!-- migration path here -->

## Security / Privacy Impact

<!-- Does this change authentication, authorization, token handling, data storage, or user PII? -->

- [ ] No security or privacy impact
- [ ] Security-relevant change — described below:

<!-- detail here -->

## Checklist

- [ ] `go test ./...` passes
- [ ] `golangci-lint run ./...` passes with no new warnings
- [ ] No `TODO`, `FIXME`, `HACK`, or stub comments introduced
- [ ] No commented-out code introduced
- [ ] No plaintext credentials, tokens, or secrets introduced
- [ ] All new user-visible strings use `i18n.T()`/`i18n.Tf()` (not hardcoded English)
- [ ] `CGO_ENABLED=0` — no CGO introduced
- [ ] All new CLI flags follow the required flag convention (long form only, except `-h`/`-v`)
