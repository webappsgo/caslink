# Installation

## Docker (Recommended)

The Docker image listens on port 80 internally. Run it from an isolated temporary directory so its runtime data stays outside the repository. The production port is bound to Docker's bridge gateway; set `CASLINK_HTTP_PORT` if 64580 is already in use:

```bash
mkdir -p "${TMPDIR:-/tmp}/webappsgo"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/webappsgo/caslink-XXXXXX")
mkdir -p "$TEMP_DIR/volumes/config" "$TEMP_DIR/volumes/data"

docker run -d \
  --name caslink \
  -p "172.17.0.1:${CASLINK_HTTP_PORT:-64580}:80" \
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

# Or set in config
# server:
#   port: 8080
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

The unit file is written to `/etc/systemd/system/caslink.service` and runs under a dedicated `caslink` system user.

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

On the very first start (before any admin account exists), requests to `/` and `/server/{admin_path}/` redirect to `/server/{admin_path}/setup`. Startup prints the one-time setup token once; enter it in the setup wizard. The wizard collects:

1. **Setup token** — authorizes creation of the first server-admin account.
2. **Admin username, email, and password** — the primary admin account. The password is hashed with Argon2id.
3. The server writes the admin record to `users.db`, consumes the one-time setup token, and displays a link to the shared login page at `/server/auth/login`.

After setup completes, the setup form is unavailable until the server has no admin accounts again.

## Installable web app

Caslink serves its install manifest at `/manifest.json` and its root-scope service worker at `/sw.js`. Browsers require HTTPS for service-worker installation, except on loopback development hosts such as `localhost`. The service worker caches versioned static assets and cookie-free public information pages for offline reading; it does not cache authenticated pages or queue form submissions. When a new worker is ready, use the update prompt to activate it. On iOS, use Safari's Share menu and choose **Add to Home Screen**.
