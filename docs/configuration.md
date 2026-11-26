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
| `--port PORT` | auto (64xxx) | Listen on one port or `HTTP,HTTPS` (for example `--port 8090,8443`) |
| `--baseurl PATH` | `/` | URL path prefix for deployments mounted below a domain root (for example `/myproject`) |
| `--color MODE` | `auto` | Colour output: `auto`, `yes`, `no` (also reads `NO_COLOR`) |
| `--lang CODE` | `en` | Default language code (`en`, `es`, `fr`, `de`, `it`, `zh`, `ar`, `ja`) |
| `--shell CMD` | — | Shell integration: `completions` or `init` for bash, zsh, fish, sh, dash, ksh, or PowerShell |
| `--status` | — | Query the running server's `/healthz` and print status |
| `--service CMD` | — | Service management: `start`, `stop`, `restart`, `reload`, `--install`, `--uninstall`, `--disable`, `--help` |
| `--daemon` | — | Detach from the terminal (daemonize) |
| `--debug` | — | Enable debug mode (verbose logging, pprof endpoints) |
| `--maintenance CMD` | — | Offline maintenance: `backup [file]`, `restore <file>`, `update [cmd]`, `mode <mode>`, `setup` |
| `--update CMD` | — | Update operations: `check`, `yes`, `branch stable\|beta\|daily` |

## Environment Variables

For each setting below, the loader checks `CASLINK_{NAME}` first and then `{NAME}`. Environment values override `server.yml`. CLI flags override configuration where a matching flag exists. This is the full set read by the current server config loader; similarly named variables are not necessarily supported.

### Server

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_MODE` | `production` | Application mode: `production` or `development` |
| `CASLINK_PORT` | `8080` or `80,443` | Listen on one port or HTTP,HTTPS ports |
| `CASLINK_LISTEN` | `0.0.0.0` | Listen address; `CASLINK_ADDRESS` is applied afterward if both are set |
| `CASLINK_ADDRESS` | `[::]` | Listen address |
| `CASLINK_DOMAIN` | Runtime hostname | Public FQDN / base URL |
| `CASLINK_FQDN` | Runtime hostname | Public hostname used in generated links |
| `CASLINK_APP_NAME` | `Caslink` | Branding title |
| `CASLINK_APP_DESCRIPTION` | `URL shortener` | Branding description |

### Admin

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_ADMIN_EMAIL` | `admin@{runtime-hostname}` | Admin contact email |
| `CASLINK_ADMIN_PATH` | `administration` | Admin panel URL segment (`/server/{path}/`) |

The YAML keys are `server.contact.admin.email` for the contact address and `server.admin_path` for the admin UI segment. The latter defaults to `administration`; reserved route names and invalid values fall back to that default.

### URL path prefix

Set `server.baseurl` or pass `--baseurl /myproject` when the application is
mounted below a domain root. Both `/myproject` and `/myproject/` are accepted.
For reverse-proxy deployments, a trusted proxy's `X-Forwarded-Prefix`,
`X-Forwarded-Path`, or `X-Script-Name` takes precedence. Forwarded prefix
headers from untrusted peers are ignored. Configure the proxy address under
`server.trusted_proxies.additional` before relying on forwarded headers.

### Database

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_DATABASE_DRIVER` | `sqlite` | Database driver |
| `CASLINK_DATABASE_URL` | `postgres://...` | Database DSN |
| `CASLINK_DATABASE_HOST` | `db.internal` | Database host |
| `CASLINK_DATABASE_NAME` | `caslink` | Database name |
| `CASLINK_DATABASE_USERNAME` | `caslink` | Database username |
| `CASLINK_DATABASE_PASSWORD` | `secret` | Database password |

### TLS / Let's Encrypt

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_SSL_ENABLED` | `true` | Enable TLS |
| `CASLINK_LE_ENABLED` | `true` | Enable Let's Encrypt |
| `CASLINK_LE_EMAIL` | Empty | ACME account email |
| `CASLINK_LE_CHALLENGE` | `http-01` | ACME challenge mode |
| `CASLINK_LE_STAGING` | `false` | Use LE staging CA (for testing) |

### Email / SMTP

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_EMAIL_ENABLED` | `true` | Enable outbound email |
| `CASLINK_EMAIL_FROM` | `no-reply@{runtime-hostname}` | From address |
| `SMTP_HOST` | Empty | SMTP server host (also `CASLINK_SMTP_HOST`) |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USERNAME` | `user` | SMTP auth username |
| `SMTP_PASSWORD` | `pass` | SMTP auth password |

### Features & Limits

| Variable | Example | Description |
|----------|---------|-------------|
| `CASLINK_MIN_RANDOM_LENGTH` | `6` | Minimum random short-code length |
| `CASLINK_RATE_LIMIT_ENABLED` | `true` | Enable rate limiting |
| `CASLINK_RATE_LIMIT_REQUESTS` | `120` | Request count used by the configured rate-limit window |
| `CASLINK_METRICS_ENABLED` | `true` | Enable the `/metrics` endpoint |
| `CASLINK_METRICS_TOKEN` | `secret` | Bearer token protecting `/metrics` |
| `CASLINK_GEOIP_ENABLED` | `true` | Enable GeoIP lookups |
| `CASLINK_ANALYTICS_ENABLED` | `true` | Enable click analytics |
| `CASLINK_ANONYMIZE_IPS` | `true` | Anonymize IPs in analytics |

Federation has no environment-variable override. Its default is disabled. When enabled, peers and publications are stored in the users database and the signing identity is stored at `{data_dir}/federation/ed25519.key` with owner-only permissions.

### Client / Maintenance

`NO_COLOR` is read by the CLI. The API token and backup password are supplied through the client/command mechanisms, not these server configuration environment variables.

| Variable | Example | Description |
|----------|---------|-------------|
| `NO_COLOR` | `1` | Disable coloured output (any non-empty value) |

## `server.yml` Reference

The file is created automatically at `{config_dir}/server.yml` on first run. Edit it then restart the server for changes to take effect. The `server`, `web`, and `caslink` sections map to the application's configuration; the admin panel exposes only the settings supported by its pages and API.

### Server

```yaml
server:
  port: 64580          # Omit to select and persist an available port in 64000-64999
  address: "[::]"      # listen address; use "0.0.0.0" for IPv4-only
  mode: production     # production | development
  fqdn: ""             # detected hostname for operator reference only; set DOMAIN to choose the public hostname
  baseurl: /           # URL path prefix; trusted reverse-proxy prefix headers take precedence
  api_version: v1      # version segment for API routes
  admin_path: administration # admin UI URL segment
  daemonize: false
  pidfile: true
```

### Branding

```yaml
server:
  branding:
    title: Caslink
    tagline: ""
    description: ""
    logo_url: ""
    favicon_url: ""
    default_theme: dark   # dark | light | auto
    primary_color: ""     # CSS hex colour, e.g. "#58a6ff"
```

### SEO

```yaml
server:
  seo:
    keywords: []         # meta keyword list injected into public page <head>
```

### Admin Panel

```yaml
server:
  admin_path: administration
  contact:
    admin:
      email: admin@example.com
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
    backup_cron: "0 2 * * *"
    backup_enabled: true
    ssl_renewal_cron: "0 3 * * *"
    ssl_renewal_enabled: true
    geoip_update_cron: "0 3 * * 0"
    geoip_update_enabled: true
    blocklist_update_cron: "0 4 * * *"
    blocklist_update_enabled: true
    cve_update_cron: "0 5 * * *"
    cve_update_enabled: true
    update_check_cron: "0 6 * * *"
    update_check_enabled: true
    healthcheck_cron: "@every 5m"
    healthcheck_enabled: true
    tor_health_cron: "@every 10m"
    tor_health_enabled: true
    cluster_heartbeat_cron: "@every 30s"
    cluster_heartbeat_enabled: true
    domain_verification_cron: "@every 30m"
    domain_verification_enabled: true
    custom_domain_ssl_renewal_cron: "0 4 * * *"
    custom_domain_ssl_renewal_enabled: true
```

### Email / Notifications

```yaml
server:
  notifications:
    email:
      enabled: false
      provider: smtp       # smtp | sendgrid | ses
      from: ""             # defaults to no-reply@{fqdn}; email is disabled when the public FQDN is unresolved
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

### Compliance

```yaml
server:
  compliance:
    enabled: false       # when true, enforces stricter password policy and
                         # requires a backup encryption password (AI.md PART 22)
```

All compliance standards are disabled by default; enabling compliance applies
the strictest-wins baseline described in the security rules.

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
    federation:
      enabled: false
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
  llms:
    include_endpoints: true       # list public and authenticated API operations
    include_schemas: false        # include request schemas with listed operations
    custom_sections: []           # up to 10 {title, content} Markdown sections
```

`/.well-known/llms.txt` and `/llms.txt` are always enabled. Custom section titles
must be 1–80 characters and content is limited to 4096 characters. HTML is
escaped and links are limited to HTTPS and relative paths. An operator-provided
`{data_dir}/web/.well-known/llms.txt` replaces the generated document.

### Compression

```yaml
server:
  compression:
    enabled: true
    level: 6             # gzip level 1–9
    types:               # MIME types eligible for compression
      - text/html
      - text/css
      - text/javascript
      - application/json
      - application/javascript
      - image/svg+xml
```

### Trusted Proxies

```yaml
server:
  trusted_proxies:
    additional: []       # extra public proxy IPs / CIDRs to trust for
                         # X-Forwarded-* headers (private + link-local ranges
                         # are always trusted); hostnames refresh every 5 min
    client_ip_header: auto # auto or one supported client-IP header name
```

Client IP chains are walked from the nearest proxy toward the client. For
production, pin `client_ip_header` to the header your front proxy overwrites
or appends; the default `auto` checks `X-Forwarded-For`, `Forwarded`,
`CF-Connecting-IP`, `True-Client-IP`, `X-Real-IP`, then `X-Client-IP`.

## Modes

| Mode | Debug flag | Behaviour |
|------|-----------|-----------|
| `production` | off | Compact access logs, no pprof endpoints |
| `production` | on | Same as production plus pprof and verbose logs |
| `development` | off | Verbose chi logger |
| `development` | on | Full debug: pprof, `/debug/*` endpoints, verbose logs |

Debug endpoints (`/debug/pprof/*`, `/debug/vars`, `/debug/config`, `/debug/routes`, `/debug/cache`, `/debug/db`, `/debug/scheduler`) are only available when `--debug` is set or `MODE=development`. They return 404 in production without the debug flag.

## I2P eepsite

The server can publish the HTTP application through an I2P eepsite. `server.i2p.enabled` defaults on when the application detects a container and off otherwise. An explicit YAML value overrides that computed default. At runtime, `I2P_ENABLED` overrides YAML, and the `--i2p` command-line flag has highest precedence; `--i2p=false` explicitly disables the feature.

```yaml
server:
  i2p:
    enabled: false
    expose: false
    binary: ""
    sam_address: 127.0.0.1:7656
    virtual_port: 80
    inbound_length: 3
    outbound_length: 3
    inbound_quantity: 5
    outbound_quantity: 5
    signature_type: 7
    bootstrap_timeout: 300
```

The server starts the dedicated loopback backend only after the regular HTTP listener is serving. It prefers an available `i2pd` binary and otherwise connects to the configured external SAM bridge. Destination keys are persisted under the configured data directory. `expose` controls whether the eepsite address appears in public autodiscovery; the authenticated health endpoint and admin page report operational status.
