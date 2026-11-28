# Development Guide

## Prerequisites

- Docker (for building - Go runs in containers)
- Make

## Building

```bash
make dev                    # Quick isolated build in the project temp directory
make test                   # Unit tests, vet, and the 60% coverage gate
./tests/run_tests.sh        # Docker-based binary and integration suite
./tests/incus.sh            # Explicit full service-lifecycle suite
make local                  # Production binaries for the current platform
make build                  # Server and CLI for all eight supported platforms
make docker                 # Host-platform image build, runtime check, and local tags; does not push
```

## Project Structure

```
src/
├── main.go               # Server entry point
├── client/               # caslink-cli, configuration, setup, and TUI
├── common/               # Shared crypto, display, i18n, and utilities
├── config/               # Server configuration and path handling
├── server/               # HTTP server, middleware, handlers, services, and stores
└── ...                   # Scheduler, backup, updater, and supporting packages
```

## SPEC Compliance

This project follows a strict specification in `AI.md`. All changes must comply with the spec.

## Testing

Go builds and tests run in Docker containers. Never run project binaries directly on the host.

```bash
make test                 # Unit tests, vet, and the coverage gate
./tests/run_tests.sh      # Docker-based binary and integration suite
./tests/incus.sh          # Explicit full service-lifecycle suite
make build                # Build the local server binary before browser testing
./tests/e2e.sh            # On-demand browser checks: SSR, no-JS, and full-JS tiers
```

The browser suite is a developer-invoked beta check, not part of `make test` or a
required CI check. It starts an isolated test network with the app and a
headless Chromium sidecar, checks user flows and page conformance, and removes
the containers on exit. On failure, redacted server logs and browser artifacts
are retained in the project-scoped system temporary directory printed by the
script.

The test scripts place runtime data in the project-scoped system temporary
directory and clean up the containers and successful-run artifacts they create.
Failure diagnostics may be retained there for troubleshooting.

`make docker` loads the host-platform image into the local Docker image store,
checks the required runtime commands, then starts the image and waits up to 180
seconds for Tor to become healthy. If the container has no outbound network,
the check reports a skip while Tor is `starting` or `unavailable`. Set
`REQUIRE_OVERLAY=1` to require successful Tor bootstrap. The Docker workflows
run the same check against the published standard, development, and AIO images.
