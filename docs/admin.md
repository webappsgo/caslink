# Admin Panel

## Accessing the Admin Panel

The admin panel is at:

```
http://your-server/server/admin/
```

The `admin` path segment is configurable in `server.yml`:

```yaml
server:
  admin:
    path: admin   # change to any slug to obscure the URL
```

The admin login page is served at `/server/{admin_path}/` (GET). Submit credentials via `POST /server/{admin_path}/login`. The panel is intentionally not linked from any public page.

Admin sessions use a separate cookie (`caslink_admin_session`) from regular user sessions and expire after 24 hours idle.

## First-Run Setup

Before any admin account exists, every request redirects to `/setup`. The setup wizard:

1. Accepts an admin username and password (Argon2id hashed).
2. Creates the primary admin account.
3. Redirects to the admin login page at `/server/admin/`.

After the initial setup, `/setup` returns 404.

## Dashboard

`/server/admin/dashboard` — overview of server status, version, mode, uptime, and active connections.

## Sidebar Navigation

All admin pages live under `/server/{admin_path}/config/`. The sidebar contains:

### General

| Page | Path | Description |
|------|------|-------------|
| Dashboard | `/dashboard` | Server status overview |
| Settings | `/config/settings` | Core server settings (port, address, FQDN, mode) |
| Branding | `/config/branding` | Site title, tagline, logo, favicon, theme, primary colour |
| SSL/TLS | `/config/ssl` | Certificate paths, Let's Encrypt (HTTP-01 / DNS-01), staging mode |
| Scheduler | `/config/scheduler` | View scheduled task list and per-task enable/disable/cron |
| Email | `/config/email` | SMTP provider, from address, SMTP credentials |
| Logs | `/config/logs` | Structured application log viewer |
| Audit Log | `/config/logs/audit` | Append-only security audit trail |
| Backup | `/config/backup` | Trigger manual backup; view backup history |
| Maintenance | `/config/maintenance` | Mode switching, offline maintenance actions |
| Updates | `/config/updates` | Check for updates; switch branch (stable/beta/daily) |
| Server Info | `/config/info` | Version, commit, build date, runtime stats |

### Security

| Page | Path | Description |
|------|------|-------------|
| Auth | `/config/security/auth` | Password policy, session lifetimes, 2FA enforcement, OAuth2 |
| API Tokens | `/config/security/tokens` | View and revoke admin API tokens |
| Rate Limiting | `/config/security/ratelimit` | Per-IP request limits, login attempt thresholds |
| Firewall | `/config/security/firewall` | IP/CIDR block rules |
| Allowlist | `/config/security/allowlist` | IP/CIDR allowlist (bypasses rate limiting) |

### Network

| Page | Path | Description |
|------|------|-------------|
| Tor | `/config/network/tor` | Tor hidden service status, `.onion` address |
| GeoIP | `/config/network/geoip` | GeoIP database status, country allow/deny lists |
| Blocklists | `/config/network/blocklists` | IP and domain blocklist feed management |

### Users

| Page | Path | Description |
|------|------|-------------|
| Users | `/config/users` | List all users; search and filter |
| User Detail | `/config/users/{id}` | View user profile, activity, tokens |
| Suspend | `POST /config/users/{id}/suspend` | Suspend a user account |
| Activate | `POST /config/users/{id}/activate` | Reactivate a suspended account |
| Recovery Keys | `POST /config/users/{id}/recovery-keys` | Force-regenerate user recovery keys |
| Invites | `/config/users/invites` | Manage invitation links |
| Moderation | `/config/moderation/users` | Review registration approval queue |

### Cluster

| Page | Path | Description |
|------|------|-------------|
| Cluster Nodes | `/config/cluster/nodes` | View connected cluster nodes |
| Add Node | `/config/cluster/add` | Register a new cluster node |

### Help

`/server/{admin_path}/help` — admin panel help and quick reference.

## Admin API

The admin REST API mirrors the panel. Base path: `/api/v1/server/{admin_path}/`. Requires `Authorization: Bearer adm_...`.

See [API Reference](api.md#admin-api) for the complete endpoint list.

## Security Notes

- The admin path is kept out of `robots.txt` (`Disallow: /server/admin`).
- Admin sessions are separate from user sessions and use stricter cookie settings.
- All admin actions are written to the audit log at `/config/logs/audit`.
- The admin panel URL is never linked from public pages or the sitemap.
