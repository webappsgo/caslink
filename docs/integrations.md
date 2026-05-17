# Integrations

This page documents external integrations, identity providers, native app links, autodiscovery, and protocol-level integrations supported by Caslink.

## Social Login (OAuth2 / OIDC)

Caslink supports social login via any OAuth2/OIDC-compatible provider. Built-in presets:

| Provider | Type | Configuration |
|----------|------|---------------|
| Google | OIDC | `settings.auth.oauth2.google` |
| GitHub | OAuth2 | `settings.auth.oauth2.github` |
| Custom OIDC | OIDC | `settings.auth.oauth2.custom[]` |

### Configuring a Provider

In the admin panel: **Settings → Authentication → OAuth2**.

Minimum required fields:

```yaml
auth:
  oauth2:
    github:
      enabled: true
      client_id: "your-client-id"
      client_secret: "your-client-secret"
      # redirect URI is auto-set to {baseurl}/auth/oauth2/github/callback
```

## Native App Links

Caslink serves the standard well-known files required for native mobile app deep links.

### iOS — Universal Links

```
GET /.well-known/apple-app-site-association
```

Enables iOS apps to open short links directly in-app instead of the browser. Configure your app team ID and bundle ID in the admin panel under **Settings → Integrations → iOS**.

### Android — App Links

```
GET /.well-known/assetlinks.json
```

Enables Android apps to open short links directly. Configure your package name and SHA-256 signing certificate fingerprint in the admin panel under **Settings → Integrations → Android**.

## Webhooks

Caslink can deliver event notifications to any HTTP endpoint.

### Supported Events

| Event | Description |
|-------|-------------|
| `link.created` | A new short link was created |
| `link.clicked` | A short link was visited |
| `link.deleted` | A short link was deleted |
| `link.expired` | A short link passed its expiration date |
| `user.registered` | A new user registered |

### Configuring Webhooks

In the admin panel: **Settings → Integrations → Webhooks → Add Webhook**.

Payload format (JSON):

```json
{
  "event": "link.clicked",
  "timestamp": "2025-12-04T13:05:13Z",
  "data": {
    "slug": "my-link",
    "url": "https://example.com",
    "country": "US",
    "referrer": "https://twitter.com"
  }
}
```

Webhook deliveries include a `X-Caslink-Signature` header (HMAC-SHA256 of the payload body, keyed with the webhook secret).

## API Integrations

Caslink exposes a versioned REST API and a GraphQL endpoint:

| Surface | URL |
|---------|-----|
| REST API | `/api/v1/` |
| GraphQL | `/graphql` |
| Swagger UI | `/swagger` |
| GraphiQL | `/graphiql` |
| OpenAPI spec | `/server/docs/swagger` |

See [API Reference](api.md) for complete endpoint documentation.

## Prometheus Metrics

Caslink exposes internal metrics in Prometheus text format:

```
GET /metrics
Authorization: Bearer <metrics-token>  # if configured
```

The `/metrics` endpoint is **internal-only** and never publicly routed. Configure access in the admin panel under **Settings → Monitoring**.

## Email / SMTP

Caslink sends transactional email for registration confirmation, password reset, 2FA changes, and link expiration notices.

Supported providers:

| Provider | Configuration |
|----------|---------------|
| SMTP (any) | `email.smtp.*` |
| SendGrid | `email.sendgrid.api_key` |
| Amazon SES | `email.ses.*` |

Configure in the admin panel under **Settings → Email** or in `server.yml`:

```yaml
email:
  from: "noreply@yourdomain.com"
  smtp:
    host: smtp.example.com
    port: 587
    username: user
    password: pass
    tls: starttls
```

## GeoIP Database

Caslink uses a GeoIP2 database for country/city lookup on link clicks. The database is updated automatically by the built-in scheduler.

To use MaxMind GeoLite2 (free, requires account key):

```yaml
geoip:
  provider: maxmind
  license_key: "your-license-key"
  edition: GeoLite2-City
```

## Let's Encrypt (ACME)

Caslink manages TLS certificates automatically via Let's Encrypt:

- **HTTP-01** challenge: used when port 80 is accessible
- **DNS-01** challenge: used for wildcard certificates or when port 80 is not accessible

Configure DNS-01 in the admin panel under **Settings → SSL → DNS Provider**.

## Tor Hidden Service

When `tor` is installed on the host, Caslink automatically starts a Tor process and creates a `.onion` address. No manual torrc configuration required. The `.onion` address is shown in the admin panel under **Settings → Network → Tor**.
