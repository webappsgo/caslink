# Binary Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER build with CGO enabled — `CGO_ENABLED=0`, pure Go only, single static binary with `embed`-ed assets
- NEVER embed security databases (GeoIP, blocklists, CVE, Trivy DB) in the binary — download on first run, update via scheduler
- NEVER hand-roll flag parsing for the server binary (no manual `os.Args`/`switch` loops) — use stdlib `flag` (server is single-command); use `cobra`/`viper` only for the multi-command CLI client
- NEVER use `strconv.ParseBool()` directly anywhere — always `config.ParseBool()` / `config.IsTruthy()`
- NEVER launch TUI when `TERM=dumb` — force CLI mode, no ANSI escapes, no emojis, no spinners/progress bars, ASCII table fallback
- NEVER implement `--tui`, `--cli`, `--gui`, or `--mode tui/cli/gui` flags — display mode is auto-detected; `--mode` is ONLY `production`/`development`
- NEVER attempt GUI over SSH/mosh, even with X11 forwarding available — remote sessions always use TUI/CLI
- NEVER build the CLI GUI with Electron or a web view — native toolkit only (GTK4/Qt6 Linux, Cocoa macOS, Win32/WinUI Windows)
- NEVER require root/escalation for `--help` or `--version` at any subcommand level — must always work as calling user, no `sudo` check
- NEVER bind privileged ports (<1024) after dropping privileges — must bind while still root, then drop
- NEVER resolve `~`/`$HOME` again after privilege drop — service account HOME points at `{data_dir}`, causes nested wrong paths
- NEVER create a PID file inside a container (`isContainer()==true`) — skip entirely, container runtime supervises the process
- NEVER match process identity by substring (e.g. `{project_name}` matching `{project_name}-cli`) — use exact match
- NEVER daemonize on `--service start` under systemd/launchd/runit/s6/container — always foreground, ignoring config
- NEVER let CLI auto-retry after `401 TOKEN_REVOKED`/`TOKEN_EXPIRED` — must be a deliberate user re-login action
- NEVER let CLI add cluster URLs that weren't in the autodiscover response
- NEVER require CLI binary-download authentication by default — public unless operator sets `cli.binary_download.require_auth: true`
- NEVER give the Agent binary an interactive setup wizard — it is configured only via `--connect` connection string from the server admin panel
- NEVER run the Agent inside a container by default — it runs directly on the host system it monitors/manages
- NEVER hardcode `localhost`/`127.0.0.1`/`0.0.0.0`/`[::1]`/any static host/IP or display bare `GET /api/` in output — always resolve `{proto}://{fqdn}:{port}/path`

## CRITICAL - ALWAYS DO

- ALWAYS show the ACTUAL (possibly renamed) binary name in `--help`, `--version`, and error messages; ALWAYS hardcode `{project_name}` internally for User-Agent, default paths, config keys, DB tables, API identifiers
- ALWAYS respect `NO_COLOR` (any non-empty value disables colors AND emojis) in every binary — priority: CLI flag > config file > `NO_COLOR` env > auto-detect (TTY + `TERM`)
- ALWAYS accept both `--flag=value` and `--flag value` syntax on every binary
- ALWAYS give every flag a corresponding config-file default; config-only override via `cli.yml`/`agent.yml`, never a flag, for TUI/GUI/CLI mode forcing
- ALWAYS create directories on demand for every directory flag (`mkdir -p` equivalent) with correct perms (root: 0755 dirs/0644 files; user: 0700 dirs/0600 files)
- ALWAYS detect stale PID files on every startup and verify process identity (handles PID reuse)
- ALWAYS remove the PID file in signal handlers (defer + handler) on graceful shutdown
- ALWAYS drop root privileges as early as possible — but only after creating dirs/setting ownership/binding privileged ports
- ALWAYS handle SIGTERM/SIGINT/SIGQUIT/SIGRTMIN+3 (Docker STOPSIGNAL) as graceful shutdown; SIGHUP ignored (auto config reload); SIGUSR1 reopen logs; SIGUSR2 status dump (Unix only — Windows only gets `os.Interrupt`)
- ALWAYS support `--shell completions [SHELL]` and `--shell init [SHELL]` on every binary (server, agent, client), auto-detecting `$SHELL` when omitted
- ALWAYS create `cli.yml`/`token` files with `0600` perms (or Windows ACL restricted to the running user) and refuse to use them on read if perms are too loose
- ALWAYS verify SHA-256 on any self-update download before atomic binary replace (CLI, agent, server all follow the same pattern)
- ALWAYS honor the full cluster URL list from `/api/autodiscover` for failover (CLI and agent) — never fail over to an unlisted URL
- ALWAYS use `display.DetectDisplayEnv()` from `src/common/display/detect.go` for GUI/TUI/CLI/Headless detection — shared across all three binaries
- ALWAYS make the CLI 100% feature-complete against the server API — GUI must not be feature-incomplete compared to TUI

## Key Rules

### Binary identity (PART 7 / PART 8)

| Binary | Default name | User-Agent | Config file |
|---|---|---|---|
| Server | `caslink` | `caslink/{version}` | `server.yml` |
| Agent | `caslink-agent` | `caslink-agent/{version}` | `agent.yml` |
| Client | `caslink-cli` | `caslink-cli/{version}` | `cli.yml` |

- Shared flags on ALL binaries: `--help`/`-h`, `--version`/`-v`, `--shell`, `--debug`, `--color {auto|yes|no}`, `--lang CODE`
- Only `-h`/`-v` get short forms; everything else long-form only
- go.mod module: `github.com/webappsgo/caslink`

### External security data (never embedded)

| Data | Dir | Source | Frequency |
|---|---|---|---|
| GeoIP | `{data_dir}/security/geoip/` | ip-location-db (GitHub Releases, no key) | daily/monthly/twice-weekly by tier |
| Blocklists | `{data_dir}/security/blocklists/` | configurable | daily |
| CVE | `{data_dir}/security/cve/` | NVD/NIST | daily |
| Trivy DB | `{data_dir}/security/trivy/` | Aqua | daily |

Download fails → log WARN, continue (graceful degradation); scheduler keeps updated.

### Display mode hierarchy (PART 7 / PART 33)

| Mode | Trigger |
|---|---|
| GUI | native display, no SSH/mosh (CLI only) |
| TUI | interactive terminal |
| CLI | command given or piped output |
| Headless | no display, no TTY |

| Binary | GUI | TUI | CLI | Headless |
|---|---|---|---|---|
| Server | status window | status banner | commands | default (daemon) |
| CLI | full app | full app (default) | commands | error |
| Agent | status window | status banner | commands | default (service) |

`TERM=dumb` forces CLI mode unconditionally, disables all ANSI/emoji/spinners.

### Server binary commands (complete, fixed set — PART 8)

```
--help  --version  --shell {completions,init,help} [SHELL]
--mode {production|development}  --config DIR  --data DIR  --cache DIR
--log DIR  --backup DIR  --pid FILE  --address ADDR  --port PORT
--baseurl PATH  --status  --daemon  --debug  --color {auto|yes|no}
--lang CODE
--service {start,restart,stop,reload,--install,--uninstall,--disable,--help}
--maintenance {backup,restore,update,mode,setup,--help} [file|setting]
--update [check|yes|branch {stable|beta|daily}|--help]
```

`--status` exit codes: 0 = healthy, 1 = unhealthy (used for Docker healthcheck).

### Directory flags & defaults

| Flag | Root default | User default |
|---|---|---|
| `--config` | `/etc/webappsgo/caslink/` | `~/.config/webappsgo/caslink/` |
| `--data` | `/var/lib/webappsgo/caslink/` | `~/.local/share/webappsgo/caslink/` |
| `--cache` | `/var/cache/webappsgo/caslink/` | `~/.cache/webappsgo/caslink/` |
| `--log` | `/var/log/webappsgo/caslink/` | `~/.local/log/webappsgo/caslink/` |
| `--backup` | `/mnt/Backups/webappsgo/caslink/` (fallback `{data_dir}/backup/`) | `~/.local/share/Backups/webappsgo/caslink/` |
| `--pid` | `/var/run/webappsgo/caslink.pid` | `{data_dir}/caslink.pid` |

Mode (system vs user) is locked once from EUID at process start — never re-derived.

### Server startup sequence (21 steps, PART 8)

1. Parse flags, handle immediate-exit flags (`--help`, `--version`, `--status`, `--shell/--service/--maintenance/--update --help`)
2–4. Handle `--service`/`--maintenance`/`--update` subcommands, exit
5. Resolve/cache all paths once
6. If root: create user, create dirs, chown, chmod, determine ports, bind privileged ports, **drop privileges**
7. If user: create user-scope dirs (port must be >1024)
8. Init logging → check/write PID file → load config (generate on first run + one-time setup token) → reconfigure logging → init database → start scheduler → start Tor if available → start HTTP server → register signal handlers → log startup complete → enter main loop

### Daemonization (`--daemon`, Unix only)

- Default: foreground (systemd/launchd prefer this)
- `--service start` auto-detects manager and ignores `daemonize` config: systemd/launchd/runit/s6/container → always foreground; SysV/rc.d → always daemonize
- Manual start priority: `--daemon` flag > `daemonize` config > default false
- Windows: `--daemon` ignored with warning; use `--service --install` + Windows Service instead

### Signals

| Unix | Action | Windows |
|---|---|---|
| SIGTERM/SIGINT/SIGQUIT/SIGRTMIN+3 | graceful shutdown | `os.Interrupt` only |
| SIGHUP | ignored (auto reload) | n/a |
| SIGUSR1 | reopen logs | n/a |
| SIGUSR2 | status dump | n/a |

Shutdown timeouts: in-flight requests 30s, children 10s (SIGKILL after), DB flush 5s, log flush 2s.

### NO_COLOR / emoji priority

1. CLI flag (`--color=yes/no`) 2. Config file 3. `NO_COLOR` env (non-empty disables) 4. Auto-detect (TTY + `TERM`)
NO_COLOR disables colors + emojis only — never bold/underline/box-drawing/structure. `TERM=dumb` disables ALL ANSI escapes.

### CLI client (PART 33)

- Config: `~/.config/webappsgo/caslink/cli.yml` (Unix) / `%APPDATA%\webappsgo\caslink\cli.yml` (Windows), perms `0600`/user-only ACL
- Token resolution priority: `--token` flag > `--token-file` > `CASLINK_TOKEN` env > `cli.yml` `auth.token` > `{config_dir}/token` file
- `401 TOKEN_REVOKED`/`TOKEN_EXPIRED`: interactive → modal + re-login prompt (preserve drafts); non-interactive/watch → stderr message, exit code 4, no auto-retry; delete cached token either way
- `--user NAME` (auto-detect user/org), `--user @NAME` (force user), `--user +NAME` (force org); reserved names: `me`, `self`, `@me`, `admin`, `system`, `api`, `www`
- Flag-to-config save: only overwrite config value if current is empty or invalid; otherwise flag applies session-only
- Mode auto-detection: `-h`/`-v` → CLI, exit; interactive terminal + no command → TUI; interactive + command/args → CLI; piped/non-interactive → plain output
- GUI toolkits: Linux GTK4/Qt6, macOS Cocoa (cgo), Windows Win32/WinUI, BSD GTK4/Qt6 — no Electron/web views
- Setup wizard (GUI/TUI) is CLI-only; server has no CLI/TUI wizard (web-based `/server/{admin_path}/config/setup`); agent has none (connection-string only)
- Exit codes: 0 success, 1 general error, 2 config error, 3 connection error, 4 auth error, 5 not found, 64 usage error
- Cluster failover: cache full cluster list from autodiscover; on connection-level failure (not auth/4xx) try each cluster URL in order; promote to primary after 5 min sustained failure

### Agent binary (optional per-project, PART 33)

- Only needed when the server must reach INTO remote machines to collect data/execute commands
- Config: `/etc/webappsgo/caslink/agent.yml` (root) / `~/.config/webappsgo/caslink/agent.yml` (user)
- Flags mirror server pattern: `--help --version --status --config --data --log --server --token --debug --color --lang`, plus `status`/`test`/`connect`/`--service ...`/`--update ...` subcommands
- First run requires `--connect "https://server?token=agt_xxx&name=..."` — errors if missing, no wizard
- Runs directly on the host, not containerized (unless the project explicitly needs container-based agent)
- Same cluster-failover and self-update pattern as CLI/server

For complete details, see AI.md PART 7, 8, 33
