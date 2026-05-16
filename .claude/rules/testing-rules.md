# Testing Rules (PART 29, 30, 31)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## CRITICAL - NEVER DO
- Use host Go installation for tests (always Docker or Incus)
- Leave test containers running after tests complete
- Write tests that depend on host-specific state

## TESTING HIERARCHY
| Container | Best For |
|-----------|----------|
| Incus (PREFERRED) | Full integration, systemd, persistent |
| Docker (fallback) | Quick checks, ephemeral |

## REQUIRED TEST SCRIPTS (in `tests/`)
| Script | Purpose |
|--------|---------|
| `run_tests.sh` | Auto-detect and run all tests (REQUIRED) |
| `docker.sh` | Docker-based integration tests (REQUIRED) |
| `incus.sh` | Incus/systemd integration tests (REQUIRED) |

## DOCUMENTATION (PART 30)
- MkDocs for ReadTheDocs at `docs/`
- mkdocs.yml at project root
- `.readthedocs.yaml` at project root
- Required docs: index, installation, configuration, api, cli, admin, security, integrations, development

## I18N (PART 31)
- 7 locales embedded in ALL binaries: en, es, fr, de, zh, ar, ja
- English is base language (always complete)
- Translation files at `src/common/i18n/locales/`
- --lang flag supported on all binaries
- Translation parity: all binaries support same languages

---
For complete details, see AI.md PART 29, 30, 31
