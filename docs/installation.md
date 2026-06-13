# Installation

## Docker (Recommended)

The Docker image exposes port 80 internally; map it to 64580 (or any port you choose):

```bash
docker run -d \
  --name caslink \
  -p 64580:80 \
  -v ./volumes/config:/config \
  -v ./volumes/data:/data \
  casapps/caslink:latest
```

Open `http://localhost:64580/setup` to run the first-run setup wizard.

### Docker Compose

```yaml
services:
  caslink:
    image: casapps/caslink:latest
    restart: unless-stopped
    ports:
      - "64580:80"
    volumes:
      - ./volumes/config:/config
      - ./volumes/data:/data
    environment:
      PORT: "80"
      MODE: "production"
```

Inside the container the app always listens on `0.0.0.0:80`. Use the `PORT` environment variable only if you need to change the internal port.

### Data Paths Inside the Container

| Purpose | Container path |
|---------|----------------|
| Config (`server.yml`) | `/config/caslink/` |
| Databases (`server.db`, `users.db`) | `/data/caslink/db/` |
| Backups | `/data/caslink/backups/` |
| GeoIP databases | `/data/caslink/security/geoip/` |

## Binary

Download the release binary for your platform from [GitHub Releases](https://github.com/casapps/caslink/releases/latest).

Available builds: `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64`, `windows-arm64`, `freebsd-amd64`, `freebsd-arm64`.

```bash
wget https://github.com/casapps/caslink/releases/latest/download/caslink-linux-amd64
chmod +x caslink-linux-amd64
./caslink-linux-amd64
```

On first run the binary:

1. Creates the config directory and writes a default `server.yml`.
2. Selects an available port starting from 64580.
3. Creates the two SQLite databases (`server.db` and `users.db`).
4. Prints the listen address and redirects the browser to `/setup`.

### Default Paths (Linux, running as root)

| Purpose | Path |
|---------|------|
| Config | `/etc/casapps/caslink/` |
| Data | `/var/lib/casapps/caslink/` |
| Logs | `/var/log/casapps/caslink/` |
| PID | `/var/run/casapps/caslink/caslink.pid` |

### Default Paths (Linux, running as a regular user)

| Purpose | Path |
|---------|------|
| Config | `~/.config/casapps/caslink/` |
| Data | `~/.local/share/casapps/caslink/` |
| Logs | `~/.local/state/casapps/caslink/logs/` |

Override any path with the corresponding CLI flag (see [Configuration](configuration.md)).

## Port Persistence

The port auto-selection algorithm tries ports starting at 64580 and walks up to 65000. Once a port is chosen it is written to `server.yml` and reused on subsequent restarts. To change the port:

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
git clone https://github.com/casapps/caslink
cd caslink
make build           # all 8 platforms → binaries/
make dev             # quick local build → $TMPDIR/casapps/caslink-*/
make local           # production test build → binaries/ (current platform)
```

## First-Run Setup Wizard

On the very first start (before any admin account exists), every request redirects to `/setup`. The wizard collects:

1. **Admin username and password** — the primary admin account. The password is hashed with Argon2id.
2. The server writes the admin record to `users.db` and redirects to the admin login page at `/server/admin/`.

After setup completes, the `/setup` route returns 404 until the database is wiped.
