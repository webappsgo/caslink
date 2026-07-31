# Service & Privilege Escalation Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never prompt for privilege escalation if the user cannot actually escalate (not in sudoers/wheel/admin) — show an informative error instead
- Never skip the "already root/admin" check before prompting for escalation
- Never run the service permanently as root/Administrator unless IDEA.md explicitly approves it (and the service file + docs must say why privilege drop is impossible)
- Never use a reserved/well-known UID/GID (65534, 980-999, 101-110, 170-179, etc.) for the `caslink` system user, even if it looks free on the current host
- Never pick a UID/GID outside the safe range (Linux/BSD: 200-899; macOS: 200-399)
- Never let UID and GID differ — they MUST be the same numeric value
- Never require `--service --install` to redo user/dir/permission setup — that happens on normal server startup, not on install
- Never delete data on `--service --disable` — only `--uninstall` deletes data, and only after an explicit `[y/N]` confirmation
- Never remove the Windows service via a manually-managed user account — Windows services default to Virtual Service Account (VSA); never use Local System, Administrator, or a logged-in user account
- Never accept a `usr_` or `org_` token where an `adm_` (admin) token is required in `caslink-cli --admin ...` commands

## CRITICAL - ALWAYS DO

- Always check EUID==0 / elevated token first; only prompt for escalation if the check fails AND the user can actually escalate
- Always follow the OS-specific escalation order:
  - Linux: root → sudo → su → pkexec → doas
  - macOS: root → sudo → osascript (GUI)
  - BSD: root → doas → sudo → su
  - Windows: Administrator → UAC prompt → runas
- Always detect the platform's init system and install the matching service type: systemd/OpenRC/SysVinit/runit (Linux), launchd (macOS), rc.d (FreeBSD), Windows Service
- Always fall back to a user-level service (systemd --user, launchctl user agent) when the caller cannot install a system service
- Always drop privileges after binding privileged ports (Unix): start as root only long enough to bind <1024, then switch to the `caslink` service user
- Always create the home directory before creating the service user, then `chown` after
- Always search for an available UID/GID starting at the top of the safe range and working down (899→200 Linux/BSD, 399→200 macOS), skipping reserved IDs
- Always use `NT SERVICE\caslink` (Virtual Service Account) for the Windows service — no manual user creation needed
- Always require confirmation (`[y/N]`) before `--service --uninstall` since it deletes config, data, cache, log, backup dirs, the PID file, and the system user/group
- Always leave the binary in place after `--service --uninstall` and print the manual removal path

## Key Rules

### System user (Linux/BSD)

| Field | Value |
|---|---|
| Username / Group | `caslink` |
| UID/GID | Same value, range 200-899 |
| Shell | `/sbin/nologin` or `/usr/sbin/nologin` |
| Home | `/etc/webappsgo/caslink` (config) or `/var/lib/webappsgo/caslink` (data) |
| Gecos | `caslink service account` |

### Service file locations

| Init system | Path |
|---|---|
| systemd | `/etc/systemd/system/caslink.service` |
| OpenRC / SysVinit | `/etc/init.d/caslink` |
| runit | `/etc/sv/caslink/run` |
| FreeBSD rc.d | `/usr/local/etc/rc.d/caslink` |
| macOS launchd | `/Library/LaunchDaemons/{plist_name}.plist` |
| Windows | Service Control Manager, `NT SERVICE\caslink` |

### `--service` subcommands

| Command | Effect |
|---|---|
| `start`/`stop`/`restart`/`reload` | Standard service lifecycle |
| `--install` | Install + enable + start (server does user/dir setup on its own startup) |
| `--disable` | Stop + disable auto-start; data/user/config all remain |
| `--uninstall` | Stop + disable + remove service file + delete ALL data + delete system user (confirm first); binary stays |

### `--maintenance` subcommands

`backup [file]`, `restore <file>`, `update {check|yes|branch <name>}`, `mode {production|development}`, `setup` (interactive wizard, creates primary Server Admin).

### `caslink-cli --admin` requires an admin-scope token

- Env var `CASLINK_TOKEN=adm_...` or `--token adm_...`
- `usr_`/`org_` tokens are rejected for admin subcommands
- Subtrees: `user`, `org`, `token`, `server` (`config`, `admin`, `stats`, `blocklist`)

### macOS privilege drop phases

| Phase | Running as | Action |
|---|---|---|
| Start | root | launchd starts binary as root |
| Bind | root | bind privileged ports |
| Drop | root → caslink | binary drops privileges |
| Run | caslink | serve requests |

For complete details, see AI.md PART 24, 25
