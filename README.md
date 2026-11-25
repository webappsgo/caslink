# Caslink

[![License](https://img.shields.io/github/license/webappsgo/caslink)](LICENSE.md)
[![Go](https://img.shields.io/github/go-mod/go-version/webappsgo/caslink)](go.mod)
[![Build](https://github.com/webappsgo/caslink/actions/workflows/ci.yml/badge.svg)](https://github.com/webappsgo/caslink/actions/workflows/ci.yml)
[![Documentation](https://readthedocs.org/projects/caslink/badge/?version=latest)](https://caslink.casapps.us/)

Caslink is a self-hosted URL shortener written in Go. It provides link creation and redirects, click analytics, QR codes, bulk import/export, user accounts, organizations, custom domains, and an administrator interface. The server renders its web pages in Go templates and embeds its frontend assets in the binary.

## Features

- Short links with custom codes, passwords, expiration, visibility, and targeting options
- Click analytics with referrer, device, and GeoIP information
- QR code generation and CSV/JSON import and export
- User accounts, API tokens, TOTP, passkeys, and organization membership
- Custom domains with DNS verification and certificate management
- Configurable server administration, scheduler, backup, and security controls
- REST and GraphQL APIs, OpenAPI documentation, and API autodiscovery
- Eight embedded locales: English, Spanish, Chinese, French, Arabic, German, Italian, and Japanese

## Quick start

Use the published container with persistent configuration and data directories:

```sh
docker run --detach \
  --name caslink \
  --publish 64580:80 \
  --volume caslink-config:/config/caslink \
  --volume caslink-data:/data/caslink \
  webappsgo/caslink:latest
```

Open `http://localhost:64580` and follow the first-run setup instructions. For other installation methods, see the [installation guide](docs/installation.md).

For terminal access, use `caslink-cli --server https://caslink.casapps.us login`; reauthenticate without reusing a saved token with `caslink-cli --server https://caslink.casapps.us --prompt login`.

## Documentation

- [Installation](docs/installation.md)
- [Configuration](docs/configuration.md)
- [API reference](docs/api.md)
- [CLI reference, including shell completions](docs/cli.md)
- [Administrator guide](docs/admin.md)
- [Security](docs/security.md)
- [Integrations](docs/integrations.md)
- [Development](docs/development.md)
- [Translation](docs/translating.md)

The documentation is built with MkDocs Material and published on [Read the Docs](https://caslink.casapps.us/).

## Build and development

Builds run in Docker; a local Go toolchain is not required. See [Development](docs/development.md) for the supported build and validation commands. The project supports Linux, macOS, Windows, and FreeBSD on amd64 and arm64.

Build Docker images with `scripts/build-docker.sh`. By default it builds the production image for `linux/amd64` and `linux/arm64`; multi-platform images stay in the Buildx cache. Use `--single-arch` to load the current host architecture into the local Docker image store, or `--target development` for the development image. The helper does not push images; see `scripts/build-docker.sh --help` for the available options and environment variables.

## License

Caslink is distributed under the MIT license. Third-party license notices are included in [LICENSE.md](LICENSE.md).
