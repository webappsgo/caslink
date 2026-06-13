# Configuration

## Config Hierarchy

Settings are resolved in this order (later sources win):

1. **Compiled defaults** — safe values built into the binary
2. **`server.yml`** — the main config file (auto-created on first run)
3. **Environment variables** — `CASLINK_*` prefix (e.g. `CASLINK_PORT=8080`)
4. **CLI flags** — e.g. `--port 8080`
5. **Admin panel** — runtime overrides stored in `server.db`

## CLI Flags

Short flags: only `-h` (help) and `-v` (version). All other flags use long form.

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | — | Show help and exit |
| `-v`, `--version` | — | Show version, commit, and build date |
| `--mode MODE` | `production` | Application mode: `production` or `development` |
| `--config DIR` | auto-detected | Configuration directory (contains `server.yml`) |
| `--data DIR` | auto-detected | Data directory (databases, GeoIP, uploads) |
| `--cache DIR` | auto-detected | Cache directory |
| `--backup DIR` | auto-detected | Backup directory |
| `--log DIR` | auto-detected | Log directory |
| `--pid FILE` | auto-detected | PID file path |
| `--address ADDR` | `[::]` | Listen address |
| `--port PORT` | auto (64xxx) | Listen port; `0` = auto-select from 64xxx range |
| `--baseurl URL` | hostname | Public base URL for generated short links (e.g. `https://short.example.com`) |
| `--color MODE` | `auto` | Colour output: `auto`, `yes`, `no` (also reads `NO_COLOR`) |
| `--lang CODE` | `en` | Default language code (`en`, `es`, `fr`, `de`, `zh`, `ar`, `ja`) |
| `--shell CMD` | — | Shell integration: `completions bash\|zsh\|fish` or `init bash\|zsh\|fish` |
| `--status` | — | Query the running server's `/healthz` and print status |
| `--service CMD` | — | Service management: `start`, `stop`, `restart`, `reload`, `--install`, `--uninstall`, `--disable`, `--help` |
| `--daemon` | — | Detach from the terminal (daemonize) |
| `--debug` | — | Enable debug mode (verbose logging, pprof endpoints) |
| `--maintenance CMD` | — | Offline maintenance: `backup [file]`, `restore <file>`, `update [cmd]`, `mode <mode>`, `setup` |
| `--update CMD` | — | Update operations: `check`, `yes`, `branch stable\|beta\|daily` |

## Environment Variables

All variables use the `CASLINK_` prefix. The bare name (without prefix) is also accepted as a fallback.

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_MODE` | `production` | Application mode |
| `CASLINK_PORT` | `8080` | Listen port |
| `CASLINK_LISTEN` | `0.0.0.0` | Listen address |
| `CASLINK_DOMAIN` | `short.example.com` | Public FQDN / base URL |
| `CASLINK_DATABASE_DRIVER` | `postgres` | Database driver |
| `CASLINK_DATABASE_URL` | `postgres://...` | Full database DSN |

## `server.yml` Reference

The file is created automatically at `{config_dir}/server.yml` on first run. Edit it then restart the server for changes to take effect (or use the admin panel for live changes).

### Server

```yaml
server:
  port: 0              # 0 = auto-select from 64xxx range; persisted after first run
  address: "[::]"      # listen address; use "0.0.0.0" for IPv4-only
  mode: production     # production | development
  fqdn: ""             # public hostname used in generated links (e.g. short.example.com)
  daemonize: false
  pidfile: true
```

### Branding

```yaml
server:
  branding:
    title: caslink
    tagline: ""
    description: ""
    logo_url: ""
    favicon_url: ""
    default_theme: dark   # dark | light | auto
    primary_color: ""     # CSS hex colour, e.g. "#58a6ff"
```

### Admin Panel

```yaml
server:
  admin:
    email: admin@localhost
    path: admin          # URL segment: /server/{path}/ — change to obscure the panel
```

### SSL / TLS

```yaml
server:
  ssl:
    enabled: false
    cert: ""             # path to PEM certificate file
    key: ""              # path to PEM private key file
    min_version: TLS1.2
    letsencrypt:
      enabled: false
      email: ""          # defaults to admin email
      challenge: http-01 # http-01 | dns-01
      staging: false     # true = use LE staging CA (for testing)
      domains: []
```

### Database

```yaml
server:
  database:
    driver: file         # file | sqlite | postgres | mysql | mariadb | mssql
    path: "{datadir}/db" # SQLite: directory that holds server.db and users.db
    # For network databases:
    host: ""
    port: 0
    name: caslink
    username: caslink
    password: ""
    sslmode: ""
```

SQLite is the default (`driver: file`). Two database files are always created:

- `server.db` — server config, admin sessions, audit log, scheduler state
- `users.db` — users, API tokens, sessions, custom domains, organizations

### Rate Limiting

```yaml
server:
  rate_limit:
    enabled: true
    requests: 120              # requests per window
    window: 60                 # window size in seconds
    burst: 10                  # burst allowance above the base rate
    login_max_attempts: 5      # max login failures per 15 minutes
    password_reset_max_attempts: 3
```

### Session

```yaml
server:
  session:
    admin:
      cookie_name: caslink_admin_session
      max_age: 86400            # 24 hours
      idle_timeout: 3600        # 1 hour
    user:
      cookie_name: caslink_session
      max_age: 2592000          # 30 days
      idle_timeout: 86400       # 24 hours
    extend_on_activity: true
    secure: auto               # auto | true | false
    http_only: true
    same_site: lax             # strict | lax | none
    timeout: "24h"
    remember_me_timeout: "720h"
```

### Scheduler

All tasks run via the built-in cron scheduler. Override the schedule or disable individual tasks:

```yaml
server:
  scheduler:
    enabled: true
    session_cleanup_cron: "@every 15m"
    session_cleanup_enabled: true
    token_cleanup_cron: "@every 15m"
    token_cleanup_enabled: true
    expire_urls_cron: "30 2 * * *"
    expire_urls_enabled: true
    log_rotation_cron: "0 0 * * *"
    log_rotation_enabled: true
    backup_cron: "0 1 * * *"
    backup_enabled: true
    ssl_renewal_cron: "0 3 * * *"
    ssl_renewal_enabled: true
    geoip_update_cron: "0 3 * * 0"
    geoip_update_enabled: true
    blocklist_update_cron: "0 4 * * *"
    blocklist_update_enabled: true
    cve_update_cron: "0 5 * * 0"
    cve_update_enabled: true
    healthcheck_cron: "@every 5m"
    healthcheck_enabled: true
    tor_health_cron: "@every 10m"
    tor_health_enabled: true
```

### Email / Notifications

```yaml
server:
  notifications:
    email:
      enabled: false
      provider: smtp       # smtp | sendgrid | ses
      from: no-reply@localhost
      from_name: Caslink
      reply_to: ""
      smtp:
        host: ""
        port: 587
        username: ""
        password: ""
        use_tls: false
        use_starttls: true
```

### GeoIP

```yaml
server:
  geoip:
    enabled: true
    dir: ""              # defaults to {data_dir}/security/geoip
    deny_countries: []   # ISO 3166-1 alpha-2 codes to block
    allow_countries: []  # if set, only these countries can access
    databases:
      asn: true
      country: true
      city: true
      whois: true
```

### Security

```yaml
server:
  security:
    password:
      min_length: 8
      require_uppercase: false
      require_lowercase: false
      require_number: false
      require_special: false
```

### Metrics (Prometheus)

```yaml
server:
  metrics:
    enabled: true
    endpoint: /metrics
    include_system: true
    include_runtime: true
    token: ""            # optional bearer token; leave blank for no auth
```

The `/metrics` endpoint is internal-only. Never expose it publicly — restrict it at the reverse proxy or firewall level.

### Tor

```yaml
server:
  tor:
    binary: ""                      # auto-detected from PATH when blank
    use_network: false              # allow outbound clearnet via Tor
    allow_user_preference: true
    max_circuits: 32
    circuit_timeout: "60s"
    bootstrap_timeout: "3m"
    safe_logging: true
    bandwidth_rate: "1 MB"
    bandwidth_burst: "2 MB"
    max_monthly_bandwidth: "100 GB"
    num_intro_points: 3
    virtual_port: 80
```

### Features

```yaml
server:
  features:
    users:
      enabled: true
      registration:
        enabled: true
        require_email_verification: false
        require_approval: false
        allow_disposable_emails: false
    organizations:
      enabled: true
      allow_creation: true
      max_per_user: 5
      roles: [owner, admin, member]
    custom_domains:
      enabled: true
      max_domains_per_user: 5
      max_domains_per_org: 20
      require_ssl: true
      allow_apex: true
      allow_subdomain: true
      allow_wildcard: false
    totp_issuer: Caslink
    webauthn_display: Caslink
```

### URL Shortening

```yaml
caslink:
  url:
    min_random_length: 6
    max_custom_length: 50
    default_expiration: never    # never | 1h | 24h | 7d | 30d
    allow_custom_codes: true
    per_user_limit: 0            # 0 = unlimited
    per_org_limit: 0
    reserved_words:
      - admin
      - api
      - auth
      - user
      - org
      - setup
      - healthz
      - swagger
      - graphql
      - graphiql
```

### Analytics

```yaml
caslink:
  analytics:
    enabled: true
    enable_geolocation: true
    anonymize_ips: true
    retention_days: 365
```

### QR Codes

```yaml
caslink:
  qr:
    default_size: 256
    max_size: 2048
    default_format: png          # png | svg | pdf
    error_correction: medium     # low | medium | quartile | high
```

### Web / CORS

```yaml
web:
  ui:
    theme: dark                  # dark | light | auto
  cors: ""                       # comma-separated origins; blank = same-origin only
```

## Modes

| Mode | Debug flag | Behaviour |
|------|-----------|-----------|
| `production` | off | Compact access logs, no pprof endpoints |
| `production` | on | Same as production plus pprof and verbose logs |
| `development` | off | Verbose chi logger |
| `development` | on | Full debug: pprof, `/debug/*` endpoints, verbose logs |

Debug endpoints (`/debug/pprof/*`, `/debug/vars`, `/debug/config`, `/debug/routes`, `/debug/cache`, `/debug/db`, `/debug/scheduler`) are only available when `--debug` is set or `MODE=development`. They return 404 in production without the debug flag.
