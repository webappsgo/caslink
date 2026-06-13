# Caslink

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../LICENSE.md)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev)

Caslink is a secure, mobile-first, fully self-hosted URL shortener written in Go. It ships as a single static binary with all assets embedded — no external dependencies, no runtime configuration required to start.

## Features

- **Single static binary** — all templates, CSS, JS, and locale files embedded at build time
- **Zero-config startup** — auto-selects a port in the 64xxx range, creates `server.yml` on first run
- **Multi-database** — SQLite (default), PostgreSQL, MySQL/MariaDB, SQL Server
- **Full REST API + GraphQL** — versioned REST at `/api/v1/`, GraphQL at `/graphql`
- **Interactive API docs** — Swagger UI at `/server/docs/swagger`, GraphiQL at `/graphiql`
- **Click analytics** — real-time tracking with GeoIP enrichment, referrer and device parsing
- **QR code generation** — PNG/SVG/PDF output, configurable size and error-correction level
- **Bulk import/export** — CSV/JSON via `/api/v1/users/urls/import` and `/export`
- **Multi-user with orgs** — registration, RBAC (owner/admin/member), org-scoped API tokens
- **Custom domains** — up to 5 per user, 20 per org, DNS TXT verification, auto-SSL
- **2FA** — TOTP (RFC 6238) and WebAuthn/passkeys
- **Built-in scheduler** — SSL renewal, GeoIP updates, session cleanup, backup, blocklist refresh
- **Tor hidden service** — auto-enabled when the `tor` binary is found on PATH
- **7 embedded locales** — English, Spanish, French, German, Chinese, Arabic, Japanese
- **Prometheus metrics** — internal-only endpoint, optional bearer token protection

## Quick Start

**Docker (recommended):**

```bash
docker run -d \
  -p 64580:80 \
  -v ./volumes/config:/config \
  -v ./volumes/data:/data \
  casapps/caslink:latest
```

Open `http://localhost:64580/setup` to create the first admin account.

**Binary:**

```bash
# Download for your platform, e.g. Linux amd64
wget https://github.com/casapps/caslink/releases/latest/download/caslink-linux-amd64
chmod +x caslink-linux-amd64
./caslink-linux-amd64
```

The server prints the auto-selected port on startup and opens the setup wizard at `/setup`.

## Quick Links

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
- [Admin Panel](admin.md)
- [Security](security.md)
- [Integrations](integrations.md)
- [Development](development.md)
- [CLI Reference](cli.md)
