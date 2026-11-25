# CLI Reference

`caslink-cli` is the companion client for server access from terminals and scripts. Run it without a command in an interactive terminal to open the TUI. With no terminal, it prints help. The client discovers the API version and cluster URLs through `/api/autodiscover` and can fail over to configured cluster members.

## Install

Download the `caslink-cli` release for your operating system and architecture from [GitHub Releases](https://github.com/webappsgo/caslink/releases). The server can also distribute client binaries when the operator provisions the configured binary directory.

## Global options

| Option | Purpose |
|---|---|
| `--server URL` | Select a server; overrides the saved config |
| `--token TOKEN` | Supply a bearer token |
| `--token-file FILE` | Read a bearer token from a file |
| `--output table\|json\|csv` | Select output format |
| `--user NAME` | Select user or organization context (`@NAME` for user, `+NAME` for org) |
| `--prompt` | Prompt for a fresh API token during login, without reusing cached credentials |
| `--debug` | Print diagnostic request information |
| `--color auto\|yes\|no` | Select terminal colors; `NO_COLOR` disables automatic color |
| `--lang CODE` | Select client language |
| `--shell completions [SHELL]` | Print completions for bash, zsh, fish, sh, dash, ksh, PowerShell (`powershell` or `pwsh`); auto-detects from `$SHELL` when omitted |
| `--shell init [SHELL]` | Print a shell-specific command that loads the generated completions |
| `--update check\|yes\|branch CHANNEL` | Check or apply a client update, or select stable/beta/daily channel |
| `-h`, `--help` | Print help |
| `-v`, `--version` | Print version information |

The default server and display preferences are read from `cli.yml`. Tokens are selected in this order: `--token`, `--token-file`, `CASLINK_TOKEN`, the `token` field in `cli.yml`, then the standalone token file.

## Commands

| Command | Purpose |
|---|---|
| `login` | Validate a supplied or prompted API token, save it, and refresh discovery data |
| `logout` | Clear the saved token |
| `list` | List links visible to the token |
| `create <url> [--code CODE]` | Create a short link, optionally with a custom code |
| `get <code>` | Read link details |
| `delete <code>` | Delete a link |
| `qr <code>` | Display the link's QR code |
| `stats <code>` | Read link analytics |
| `version` | Print client version information |

Commands that manage a user's links require an appropriate bearer token. The server applies its normal ownership, organization-role, and link-policy checks.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Success |
| 1 | General error |
| 2 | Invalid command usage or unknown subcommand |
| 4 | A cached token was revoked or expired; login again |
| 65 | Server rejected input as invalid |
| 66 | Resource not found |
| 69 | Server connection failed |
| 77 | Authentication or permission denied |
| 78 | Invalid or missing client configuration |

## Configuration and credentials

The client stores `cli.yml` and the optional standalone `token` file under the user config directory:

- Unix: `$XDG_CONFIG_HOME/webappsgo/caslink/`, or `~/.config/webappsgo/caslink/` when `XDG_CONFIG_HOME` is unset.
- Windows: `%APPDATA%\webappsgo\caslink\`.

Example `cli.yml`:

```yaml
server: https://caslink.casapps.us
api_version: v1
token: usr_…
lang: en
color: auto
display:
  mode: auto
update:
  auto: false
  channel: stable
```

The client refuses to use a config or standalone token file accessible to group or other users (or a Windows ACL that grants access to `Everyone`, `Users`, or `Authenticated Users`). Keep both credential files private; the CLI creates its config directory and file with restrictive permissions.

## Examples

```sh
caslink-cli --server https://caslink.casapps.us login
caslink-cli --output json list
caslink-cli create https://caslink.casapps.us/learn-more --code launch
caslink-cli stats example
caslink-cli --shell completions bash > caslink-cli-completion.bash
eval "$(caslink-cli --shell init bash)"
```
