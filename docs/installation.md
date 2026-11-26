# Installation

## Docker (Recommended)

The Docker image listens on port 80 internally. Run it from an isolated temporary directory so its runtime data stays outside the repository. The production HTTP port is bound to Docker's bridge gateway; set `CASLINK_HTTP_PORT` if 64580 is already in use. To enable dual HTTP/HTTPS listeners, set `CASLINK_CONTAINER_PORTS=80,443`, configure TLS in `/config/server.yml`, then use `https://<host>:64443` (or set `CASLINK_HTTPS_PORT`):

```bash
mkdir -p "${TMPDIR:-/tmp}/webappsgo"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/webappsgo/caslink-XXXXXX")
mkdir -p "$TEMP_DIR/volumes/config" "$TEMP_DIR/volumes/data"

docker run -d \
  --name caslink \
  -e PORT="${CASLINK_CONTAINER_PORTS:-80}" \
  -p "172.17.0.1:${CASLINK_HTTP_PORT:-64580}:80" \
  -p "172.17.0.1:${CASLINK_HTTPS_PORT:-64443}:443" \
  -v "$TEMP_DIR/volumes/config:/config:z" \
  -v "$TEMP_DIR/volumes/data:/data:z" \
  ghcr.io/webappsgo/caslink:latest
```

Open `http://172.17.0.1:64580/server/administration/setup` to run first-run setup when using the default admin path (`administration`). If `server.admin_path` is customized, replace `administration` with that path.

### Docker Compose

Run the shipped production Compose file from an isolated temporary directory. It includes the Valkey cache service and keeps its persistent data alongside the app's data:

```bash
mkdir -p "${TMPDIR:-/tmp}/webappsgo"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/webappsgo/caslink-XXXXXX")
mkdir -p "$TEMP_DIR/volumes/config" "$TEMP_DIR/volumes/data"
cp docker/docker-compose.yml "$TEMP_DIR/"
cd "$TEMP_DIR" && docker compose up -d
```

Inside the container the app listens on port 80. The production Compose file does not set a development mode; the server defaults to production.

### Data Paths Inside the Container

| Purpose | Container path |
|---------|----------------|
| Config (`server.yml`) | `/config/caslink/` |
| SQLite databases (`server.db`, `users.db`) | `/data/db/sqlite/` |
| Backups | `/data/backups/caslink/` |
| GeoIP databases | `/data/caslink/security/geoip/` |
| Logs | `/data/log/caslink/` |

## Binary

Download the release binary for your platform from [GitHub Releases](https://github.com/webappsgo/caslink/releases/latest).

Available builds: `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64`, `windows-arm64`, `freebsd-amd64`, `freebsd-arm64`.

```bash
wget https://github.com/webappsgo/caslink/releases/latest/download/caslink-linux-amd64
chmod +x caslink-linux-amd64
./caslink-linux-amd64
```

On first run the binary:

1. Creates the config directory and writes a default `server.yml`.
2. Selects and persists an available port from 64000–64999 when running outside an application container. Application containers listen on port 80 internally.
3. Creates the two SQLite databases (`server.db` and `users.db`).
4. Prints the listen address and redirects the browser to `/server/{admin_path}/setup` when first-run setup is required.

### Default Paths (Linux, running as root)

| Purpose | Path |
|---------|------|
| Config | `/etc/webappsgo/caslink/` |
| Data | `/var/lib/webappsgo/caslink/` |
| Logs | `/var/log/webappsgo/caslink/` |
| PID | `/var/run/webappsgo/caslink/caslink.pid` |

### Default Paths (Linux, running as a regular user)

| Purpose | Path |
|---------|------|
| Config | `~/.config/webappsgo/caslink/` |
| Data | `~/.local/share/webappsgo/caslink/` |
| Logs | `~/.local/log/webappsgo/caslink/` |

Override any path with the corresponding CLI flag (see [Configuration](configuration.md)).

## Port Persistence

Outside application containers, the server selects an available port from 64000–64999 when no port is configured. It writes the selected port to `server.yml` and reuses it on subsequent restarts. Inside an application container, the internal port defaults to 80; the production Docker host mapping defaults to 64580. To change the server port:

```bash
# Set permanently in server.yml
caslink --port 8080

# HTTP and HTTPS listeners
caslink --port 8080,8443

# Or set in config
# server:
#   port: 8080
# Or for HTTP plus HTTPS:
#   port: "8080,8443"
```

## systemd Service

Install caslink as a system service using the built-in service manager:

```bash
# Install (requires root or sudo)
sudo caslink --service --install

# Control the service
caslink --service start
caslink --service stop
caslink --service restart
caslink --service reload

# Remove the service
sudo caslink --service --uninstall
```

The unit file is written to `/etc/systemd/system/caslink.service`. It starts with the privileges needed to create and protect the system paths, bind listeners, and initialize the service, then the server drops to the dedicated `caslink` system user. The service account's home is `/var/lib/webappsgo/caslink`; it cannot log in interactively. The unit limits writable paths to Caslink's configuration, data, cache, logs, runtime state, backups, and installed binary.

## Building from Source

Requires Docker; no local Go toolchain needed.

```bash
git clone https://github.com/webappsgo/caslink
cd caslink
make build           # all 8 platforms → binaries/
make dev             # quick local build → $TMPDIR/webappsgo/caslink-*/
make local           # production test build → binaries/ (current platform)
```

## First-Run Setup Wizard

On the very first start (before any admin account exists), requests to `/` and `/server/{admin_path}/` redirect to `/server/{admin_path}/setup`. Startup prints the one-time setup token once. Enter it in the wizard to begin the ordered setup session. The six steps are:

1. **Primary administrator** — choose the username and email. The default username is `steward`; the wizard generates a secure password that you can copy, or you can replace it with a custom password and confirm it. Passwords are stored as Argon2id hashes.
2. **API token** — create and copy the administrator's API token. It is displayed once and cannot be recovered later.
3. **Server configuration** — set the application name, primary domain/FQDN, operating mode, and scheduler timezone.
4. **Security** — optionally set a backup-encryption password and enable TOTP for the primary administrator. TOTP enrollment requires a valid six-digit code and acknowledgment that the one-time recovery keys were saved.
5. **Optional services** — review HTTPS and certificate settings, and choose whether to enable regular-user accounts. Let's Encrypt requires a valid contact email and supported challenge; manual certificates are checked before they are saved. Enabling HTTPS requires a server restart to take effect.
6. **Complete** — save the configuration, consume the setup token, create the administrator's browser session, and open the admin panel.

Every step is a server-rendered form protected by CSRF and works without JavaScript. Browser JavaScript enhances the same ordered setup API flow. Text browsers receive a no-JavaScript page automatically; other browsers can use the **No JavaScript setup** link. Setup remains unavailable after completion unless the server has no administrator accounts and a new setup token is generated through the documented maintenance flow.

## Installable web app

Caslink serves its install manifest at `https://caslink.casapps.us/manifest.json` and its service worker at `https://caslink.casapps.us/sw.js`. Both URLs follow the configured `server.baseurl`; when Caslink is mounted under a path prefix, the manifest, worker, and worker scope use that prefix. Browsers require HTTPS for service-worker installation, except on loopback development hosts such as `localhost`. The service worker caches versioned static assets and cookie-free public information pages for offline reading; it does not cache authenticated pages or queue form submissions. When a new worker is ready, use the update prompt to activate it. On iOS, use Safari's Share menu and choose **Add to Home Screen**.
