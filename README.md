# Caslink - Self-Hosted URL Shortener

[![License](https://img.shields.io/github/license/casapps/caslink)](LICENSE.md)
[![Go Version](https://img.shields.io/github/go-mod/go-version/casapps/caslink)](go.mod)
[![Build Status](https://img.shields.io/github/actions/workflow/status/casapps/caslink/release.yml?branch=main)](https://github.com/casapps/caslink/actions)
[![Docker Pulls](https://img.shields.io/docker/pulls/casapps/caslink)](https://hub.docker.com/r/casapps/caslink)

**Caslink** is a secure, mobile-first, feature-rich, fully self-hosted URL shortener application written in Go that compiles into a single static binary with zero external dependencies.

## Features

- **Multi-User Support** - Public registration enabled by default, full user account management
- **Organization Support** - Teams/organizations with role-based access control (owner, admin, member)
- **Custom Domains** - Users and organizations can use branded domains with automatic SSL
- **Universal Feature Parity** - All users get all features, regardless of deployment type
- **Zero-Config Startup** - Works immediately with intelligent defaults
- **Single Static Binary** - No external dependencies, just one executable
- **Database Agnostic** - SQLite, PostgreSQL, MySQL, SQL Server support
- **Advanced Analytics** - Real-time click tracking with geolocation
- **QR Code Generation** - Customizable QR codes in PNG, SVG, PDF formats
- **Bulk Operations** - Import/export URLs in CSV and JSON
- **Proxy Aware** - Automatic detection of reverse proxies
- **Production Ready** - Migrations, audit trails, backups
- **Security** - Argon2id password hashing, TOTP/2FA, Passkeys/WebAuthn, OAuth2/OIDC

## Quick Start

### Using Docker (Recommended)

```bash
docker run -d \
  -p 64580:80 \
  -v caslink-config:/etc/casapps/caslink \
  -v caslink-data:/var/lib/casapps/caslink \
  casapps/caslink:latest
```

Access at: `http://localhost:64580`

### Using Binary

```bash
# Download latest release
wget https://github.com/casapps/caslink/releases/latest/download/caslink-linux-amd64
chmod +x caslink-linux-amd64

# Run server (creates admin on first run)
./caslink-linux-amd64
```

Access at: `http://localhost:64580` (or auto-selected port shown in terminal)

### Using Docker Compose

```bash
git clone https://github.com/casapps/caslink
cd caslink/docker
docker-compose up -d
```

## Configuration

Caslink works with zero configuration but can be customized via:

1. **Environment Variables** - `CASLINK_*` prefix
2. **Config File** - `/etc/casapps/caslink/server.yml`
3. **CLI Flags** - `--config`, `--data`, `--port`, etc.
4. **Admin Panel** - Web UI at `/admin`

### Example Configuration

```yaml
server:
  address: "0.0.0.0:8080"
  mode: production

database:
  type: sqlite
  path: "{datadir}/db/server.db"

url:
  min_random_length: 6
  default_expiration: never

analytics:
  enabled: true
  enable_geolocation: true
  anonymize_ips: true
```

## CLI Commands

```bash
caslink --help                 # Show all commands
caslink --version              # Show version
caslink --config /path         # Set config directory
caslink --data /path           # Set data directory
caslink --port 8080            # Set port
caslink --mode production      # Set mode (production/development)
caslink --debug                # Enable debug mode
caslink --status               # Show server status
caslink --maintenance backup   # Create backup
caslink --update check         # Check for updates
```

## API Usage

### Create Short URL

```bash
curl -X POST http://localhost:64580/api/v1/urls \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "custom_code": "ex"}'
```

### Get Analytics

```bash
curl http://localhost:64580/api/v1/urls/ex/stats
```

### Generate QR Code

```bash
curl http://localhost:64580/api/v1/qr/ex > qr.png
```

See [API Documentation](https://caslink.casapps.us/api) for complete API reference.

## Building from Source

### Prerequisites

- Docker (for building - Go is run in containers)
- Make

### Build Commands

```bash
make build          # Build for current platform
make release        # Build for all platforms
make docker         # Build Docker image
make test           # Run tests
make dev            # Quick dev build
```

All builds use Docker containers - no Go installation required on host.

## Documentation

- **[Installation Guide](https://caslink.casapps.us/installation)** - Detailed setup instructions
- **[Configuration Reference](https://caslink.casapps.us/configuration)** - All config options
- **[API Documentation](https://caslink.casapps.us/api)** - Complete API reference
- **[Admin Guide](https://caslink.casapps.us/admin)** - Managing your instance
- **[Development Guide](https://caslink.casapps.us/development)** - Contributing and development

## Architecture

Caslink uses a clean architecture with separation of concerns:

```
src/
├── main.go              # Entry point
├── config/              # Configuration management
├── server/              # HTTP server and routes
│   ├── handler/         # HTTP handlers
│   ├── service/         # Business logic
│   ├── model/           # Data models
│   └── store/           # Database access
├── swagger/             # OpenAPI/Swagger
├── graphql/             # GraphQL API
└── mode/                # Production/Development modes
```

## User Accounts & Organizations

### Multi-User Mode

Caslink supports full multi-user functionality with public registration enabled by default:

- **User Registration**: Open registration at `/auth/register` (can be disabled via config)
- **Authentication**: Username or email + password login
- **User Profiles**: Customizable display names, avatars, and bios
- **Personal URLs**: Users can create and manage their own shortened URLs
- **Privacy**: Users can only view and manage their own URLs

### Organizations

Users can create and join organizations for collaborative URL management:

- **Create Organizations**: Users can create up to 5 organizations (configurable)
- **Role-Based Access**: Owner, admin, and member roles with different permissions
- **Shared URLs**: Organization members can collaborate on URLs owned by the org
- **Org Domains**: Organizations can use up to 20 custom branded domains

### Custom Domains

Both users and organizations can use custom domains for their short links:

- **Automatic SSL**: Let's Encrypt integration with automatic certificate management
- **Domain Verification**: DNS-based ownership verification
- **User Limits**: 5 custom domains per user (configurable)
- **Org Limits**: 20 custom domains per organization (configurable)
- **Supported**: Apex domains (example.com) and subdomains (go.example.com)

## Security

- Argon2id password hashing (not bcrypt)
- TOTP/2FA support
- Passkeys/WebAuthn support
- OAuth2/OIDC integration
- Rate limiting
- CSRF protection
- XSS prevention
- SQL injection prevention
- Comprehensive audit logging
- Username blocklist to prevent impersonation

## License

MIT License - see [LICENSE.md](LICENSE.md) for details.

Includes embedded licenses for all third-party dependencies.

## Support

- **Documentation**: https://caslink.casapps.us
- **Issues**: https://github.com/casapps/caslink/issues
- **Discussions**: https://github.com/casapps/caslink/discussions

## Contributing

Contributions welcome! Please read [CONTRIBUTING.md](.github/CONTRIBUTING.md) first.

## Roadmap

- [ ] Mobile apps (iOS/Android)
- [ ] Browser extensions
- [ ] Link in bio pages
- [ ] Advanced link rotation
- [ ] A/B testing for links
- [ ] Link expiration notifications
- [ ] Webhook events

## Credits

Built with ❤️ by [casapps](https://github.com/casapps)

Powered by Go and amazing open-source libraries.
