# CLI Reference

`caslink-cli` is the companion command-line client for the Caslink server. It provides a full TUI (terminal user interface) as well as scriptable subcommands for managing short links, users, and organizations.

## Installation

Download the latest release for your platform from the [releases page](https://github.com/casapps/caslink/releases).

```bash
# Linux amd64
curl -LSsf https://github.com/casapps/caslink/releases/latest/download/caslink-cli-linux-amd64 \
  -o caslink-cli && chmod +x caslink-cli
sudo mv caslink-cli /usr/local/bin/
```

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--help` / `-h` | — | Show help and exit |
| `--version` / `-v` | — | Print version and exit |
| `--server` | `OFFICIALSITE` or required | Caslink server URL (e.g., `https://caslink.example.com`) |
| `--token` | `$CASLINK_TOKEN` | API token for authentication |
| `--mode` | `production` | Application mode (`production`\|`development`) |
| `--config` | OS default | Configuration directory |
| `--data` | OS default | Data directory |
| `--log` | OS default | Log directory |
| `--pid` | OS default | PID file path |
| `--address` | `0.0.0.0` | Listen address |
| `--port` | `64580` | Listen port |
| `--baseurl` | — | Base URL for generated short links |
| `--debug` | `false` | Enable debug mode |
| `--status` | — | Show server status and exit |
| `--lang` | `en` | Language/locale code |
| `--color` | `auto` | Color output (`auto`\|`yes`\|`no`) |
| `--service` | — | Service command (`start`\|`stop`\|`restart`\|`reload`\|`--install`\|`--uninstall`\|`--disable`\|`--help`) |
| `--daemon` | `false` | Daemonize (detach from terminal) |
| `--maintenance` | — | Maintenance command (`backup`\|`restore`\|`update`\|`mode`\|`setup`\|`--help`) |
| `--update` | — | Update command (`check`\|`yes`\|`branch stable`\|`branch beta`\|`branch daily`) |

Respects `NO_COLOR` environment variable per [no-color.org](https://no-color.org/).

## Shell Completions

```bash
# Bash
caslink-cli --shell completions bash > /etc/bash_completion.d/caslink-cli

# Zsh
caslink-cli --shell completions zsh > "${fpath[1]}/_caslink-cli"

# Fish
caslink-cli --shell completions fish > ~/.config/fish/completions/caslink-cli.fish
```

## Subcommands

### link

Manage short links.

```bash
# Create a short link
caslink-cli link create --url https://example.com --slug my-link

# List your links
caslink-cli link list

# Get stats for a link
caslink-cli link stats my-link

# Delete a link
caslink-cli link delete my-link
```

### user

Manage your user account.

```bash
# Show current user info
caslink-cli user info

# Change password
caslink-cli user password

# Manage API tokens
caslink-cli user tokens list
caslink-cli user tokens create --name "my-token"
caslink-cli user tokens revoke <token-id>
```

### org

Manage organizations.

```bash
# List organizations
caslink-cli org list

# Create an organization
caslink-cli org create --name my-org

# Invite a member
caslink-cli org invite --org my-org --user someone@example.com --role member
```

## Configuration File

`caslink-cli` looks for configuration in `~/.config/caslink/cli.yml` (Linux/macOS) or `%APPDATA%\caslink\cli.yml` (Windows).

```yaml
server: https://caslink.example.com
token: your-api-token-here
lang: en
color: auto
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CASLINK_SERVER` | Default server URL |
| `CASLINK_TOKEN` | API token |
| `CASLINK_LANG` | Language/locale |
| `NO_COLOR` | Disable color output |

## Examples

```bash
# Create a short link (JSON output for scripting)
caslink-cli link create --url https://example.com --output json

# List all links, filtered by tag
caslink-cli link list --tag marketing

# Check server health
caslink-cli --status

# Check for updates
caslink-cli --update check

# Apply update
caslink-cli --update yes
```
