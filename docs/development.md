# Development Guide

## Prerequisites

- Docker (for building - Go runs in containers)
- Make

## Building

```bash
make build          # Build for current platform
make release        # Build for all platforms
make dev            # Quick dev build
make docker         # Build Docker image
make test           # Run tests
```

## Project Structure

```
src/
├── main.go              # Entry point
├── config/              # Configuration
├── server/              # HTTP server
│   ├── handler/         # HTTP handlers
│   ├── service/         # Business logic
│   ├── model/           # Data models
│   └── store/           # Database access
├── swagger/             # OpenAPI/Swagger
└── graphql/             # GraphQL API
```

## SPEC Compliance

This project follows a strict specification in `AI.md`. All changes must comply with the spec.

## Testing

All builds and tests use Docker containers. Never run binaries directly on the host.

```bash
make test  # Run tests in Docker
```
