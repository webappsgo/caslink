# Caslink

[![CI](https://github.com/webappsgo/caslink/actions/workflows/ci.yml/badge.svg)](https://github.com/webappsgo/caslink/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/webappsgo/caslink)](https://github.com/webappsgo/caslink/releases)
[![License](https://img.shields.io/github/license/webappsgo/caslink)](LICENSE.md)
[![Documentation](https://readthedocs.org/projects/webappsgo-caslink/badge/?version=latest)](https://webappsgo-caslink.readthedocs.io/)

## About

Caslink is a self-hosted URL shortener written in Go. It provides short-link creation and redirects, click analytics, QR codes, bulk operations, user and organization management, custom domains, and a server administration interface. The server renders pages with Go templates and embeds its frontend assets in the binary.

## Official Site

[https://caslink.casapps.us](https://caslink.casapps.us)

## Features

- Short links with custom codes, passwords, expiration, visibility, and targeting options
- Click analytics with referrer, device, and GeoIP information
- QR code generation and CSV/JSON import and export
- User accounts, API tokens, TOTP, passkeys, and organization membership
- Custom domains with DNS verification and certificate management
- Server administration for configuration, scheduling, backups, updates, and security
- REST and GraphQL APIs, OpenAPI documentation, and API autodiscovery
- Eight embedded locales: English, Spanish, Chinese, French, Arabic, German, Italian, and Japanese

## Production

### Docker

```bash
docker run --detach \
  --name caslink \
  --publish 172.17.0.1:64580:80 \
  --volume caslink-config:/config \
  --volume caslink-data:/data \
  ghcr.io/webappsgo/caslink:latest
```

Open `http://172.17.0.1:64580`. On first start, use the one-time token printed in the server console to complete setup. See the [installation guide](docs/installation.md) for Compose, paths, service installation, and other deployment details.

### Binary

Download a release for Linux, macOS, Windows, or FreeBSD on amd64 or arm64 from [GitHub Releases](https://github.com/webappsgo/caslink/releases). For example:

```bash
curl -q -LSsf -o caslink-linux-amd64 https://github.com/webappsgo/caslink/releases/latest/download/caslink-linux-amd64
chmod +x caslink-linux-amd64
./caslink-linux-amd64
```

The binary creates its configuration and data directories on first run and prints the setup URL and one-time token. See [Installation](docs/installation.md) for OS paths and system service commands.

## Client

The companion `caslink-cli` binary supports server login and API operations. Download the matching CLI release and install it alongside the server binary, then connect with:

```bash
caslink-cli --server https://caslink.casapps.us login
```

Use `caslink-cli --help` and `caslink-cli <command> --help` to list commands and options. See the [CLI reference](docs/cli.md) for configuration, authentication, and shell completion details.

## Configuration

The server writes `server.yml` under its config directory on first run. Configuration can also be managed from the administrator interface. Key settings include the listen port, run mode, branding, database, TLS, email, security, and scheduled tasks. See the [configuration reference](docs/configuration.md) for paths, environment variables, and examples.

## API

The versioned API base path is `/api/v1`; the exact configured base path is available through API autodiscovery. The public health endpoint is `/server/healthz`.

```bash
curl -q -LSsf https://caslink.casapps.us/server/healthz
curl -q -LSsf -H 'Authorization: Bearer TOKEN' https://caslink.casapps.us/api/v1/urls
```

The API also provides GraphQL and an OpenAPI document. See the [API reference](docs/api.md) for route and authentication details.

## Other

- [Administrator guide](docs/admin.md)
- [Security and disclosure policy](docs/security.md)
- [Integrations](docs/integrations.md)
- [Troubleshooting and installation](docs/installation.md)
- [Translations](docs/translating.md)
- Documentation is built with MkDocs Material and published on [Read the Docs](https://webappsgo-caslink.readthedocs.io/).

## Development

Development instructions are for contributors. Builds and tests run in Docker; a local Go toolchain is not required.

```bash
git clone https://github.com/webappsgo/caslink.git
cd caslink
make dev
make test
```

Use `make build` to build the server and companion client for all eight supported OS/architecture combinations. See [Development](docs/development.md) for test phases, linting, release builds, and CI details.

## Disclaimer

This software is provided "as is" without warranty of any kind. Use it at your own risk. The authors are not responsible for damages, data loss, or issues arising from its use. This software does not provide legal, financial, medical, or other professional advice. Third-party services are governed by their own terms. Review and test the software for your environment before production use; no software can be guaranteed free of vulnerabilities. By using this software, you acknowledge that you have read and understood this disclaimer.

## License

Caslink is distributed under the MIT license. Third-party license notices are included in [LICENSE.md](LICENSE.md).
