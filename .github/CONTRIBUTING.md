# Contributing to Caslink

Thank you for your interest in contributing to Caslink!

## Local Setup

```bash
# Clone the repository
git clone https://github.com/casapps/caslink.git
cd caslink

# Build development binary (uses Docker internally - no Go required on host)
make dev

# Run unit tests
make test

# Run integration tests (Incus preferred, Docker fallback)
./tests/run_tests.sh

# Build documentation locally
pip install mkdocs mkdocs-material
mkdocs serve
```

## Branch and PR Workflow

1. Fork the repository
2. Create a feature branch from `main`: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `make test && ./tests/run_tests.sh`
5. Update documentation if applicable
6. Submit a pull request against `main`

## Code Requirements

- All code changes must include tests
- Documentation must be updated when behavior changes
- All 8 build platforms must continue to compile
- CI must pass (build, test, lint, security scan)
- `CGO_ENABLED=0` always - no CGO dependencies
- Argon2id for all password hashing - never bcrypt

## Documentation Updates

When changing user-facing behavior, API endpoints, admin settings, or security features, update the relevant file in `docs/`:

- `docs/api.md` - REST API changes
- `docs/configuration.md` - Config option changes
- `docs/admin.md` - Admin panel changes
- `docs/security.md` - Security-related changes
- `docs/integrations.md` - External integration changes

## Reporting Security Vulnerabilities

**Do not open public issues for security vulnerabilities.**

See [SECURITY.md](.github/SECURITY.md) for the responsible disclosure process.

## Code of Conduct

See [CODE_OF_CONDUCT.md](.github/CODE_OF_CONDUCT.md).
