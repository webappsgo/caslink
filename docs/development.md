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
make docker                 # Multi-platform Docker build; does not push
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
```

The test scripts create runtime data under the project-scoped system temporary
directory and remove their own temporary containers and files when they finish.
