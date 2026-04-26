# Configuration

Caslink uses a YAML configuration file located at:

- Root: `/etc/casapps/caslink/server.yml`
- User: `~/.config/casapps/caslink/server.yml`

## Configuration is auto-created on first run.

## Basic Configuration

```yaml
server:
  port: 64580
  address: "[::]"
  mode: production

database:
  driver: file
  path: "{datadir}/db"

url:
  min_random_length: 6
  max_random_length: 8
  default_expiration: never

analytics:
  enabled: true
  anonymize_ips: true
```

## All Configuration Options

See the auto-generated `server.yml` for all available options.
