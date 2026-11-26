# Integrations

This page describes integration surfaces currently exposed by Caslink. The server does not provide native mobile-app association files.

## External identity providers

The admin interface supports configuring named OIDC, LDAP, and SAML providers at `/server/{admin_path}/security/auth/{type}`. Provider configuration is stored in the server configuration and sensitive values are encrypted. The provider API is under `{api_base_path}/server/{admin_path}/security/auth/{type}/providers` (default `{api_base_path}`: `/api/v1`); it requires an admin-scoped bearer token (`adm_`).

The browser and client sign-in flows depend on enabled providers. Configure and test a provider in the admin interface before enabling it for users. Use TLS for provider endpoints and keep client secrets, bind credentials, SAML signing keys, and metadata private. The server does not ship Google or GitHub-specific presets.

## Outbound webhooks

Webhook URLs are configured per contact role under `server.contact.{role}.webhooks`. Delivery is queued in `server.db`, uses HTTPS POST, and retries failed requests after 1 minute, 5 minutes, 15 minutes, 1 hour, 6 hours, and 24 hours before dropping them. Each webhook has a random 32-byte HMAC-SHA256 signing secret stored in `secrets.yml`; the secret is never returned by APIs. Requests include `X-Webhook-Signature`, `X-Webhook-Timestamp`, `X-Webhook-ID` (UUID v7), and `X-Webhook-Event` headers.

Built-in adapters support generic JSON, Discord, Slack, Mattermost, Telegram, Pushover, and Gotify. Current event producers queue summary-only admin notifications for scheduled task failures, repeated high-severity IP offenders, and security-report receipts. Security webhooks contain report metadata and a tracking ID, never the report body or contact details. Delivery failures are rate-limited in logs.

## API discovery and documentation

Clients can request `GET /api/autodiscover` before selecting an API version. It returns server connection information and enabled capabilities. OpenAPI JSON is served at `/api/swagger` and at `{api_base_path}/server/swagger` (default `{api_base_path}`: `/api/v1`). The browser documentation is at `/server/docs/swagger`. The GraphQL console and endpoint are at `/server/docs/graphql`; GraphQL queries are also accepted at `/api/graphql` and `{api_base_path}/server/graphql`.

The API base path is configurable. See [API Reference](api.md) for authentication and route families.

## Custom domains and certificates

Users and organization members can add custom domains from their account pages or API. Caslink provides a DNS TXT verification value and reports DNS/SSL status. Verification and certificate issuance require outbound DNS/network access. The server serves ACME HTTP-01 challenges at `/.well-known/acme-challenge/{token}` when a challenge is active. Certificate configuration is managed in the admin panel; availability depends on the configured TLS mode and reachable DNS/HTTP endpoints.

## GeoIP and blocklists

The built-in scheduler updates the configured GeoIP database and security blocklists. GeoIP enriches click analytics and may inform security decisions; it must not be used as the sole access-control decision. The default GeoIP source is the project-configured `sapics/ip-location-db` dataset and does not require a MaxMind account.

## Metrics

Prometheus metrics are available only when enabled in server configuration. The configured endpoint defaults to `/metrics`; an optional bearer token can protect it. Restrict this endpoint at the network boundary as well. The admin metrics page reports server status but is not the Prometheus scrape endpoint.

## Tor

Tor support starts automatically when the `tor` executable is available; there is no enable/disable setting. The server owns a dedicated Tor process and reports its hidden-service address through authorized status and autodiscovery fields. Missing Tor or Tor startup failure does not prevent the HTTP server from starting. See [Configuration](configuration.md) for Tor settings.

## Standards endpoints

Caslink serves `/.well-known/security.txt`, `/.well-known/change-password`, and active ACME HTTP-01 challenge paths. Other `/.well-known/` paths are not served unless an explicitly enabled feature provides them. Unknown paths return `404`.

## Federation

Federation is disabled by default. Enable `server.features.federation.enabled` in the admin panel or configuration before publishing links or accepting federation traffic. The server creates an Ed25519 identity at `{data_dir}/federation/ed25519.key`; keep the data directory private and backed up. `GET /.well-known/caslink` publishes the protocol version, node/key IDs, public key, and feed URL only while enabled.

Add peers from the server admin Federation page by entering their HTTPS origin and the identity fields obtained from `/.well-known/caslink`. Verify the key out of band before saving it. Origins must resolve only to public IP addresses. Peer requests use Ed25519 signatures, five-minute timestamp bounds, and replay-protected nonces. The built-in scheduler pulls signed announcement pages every five minutes; failures are isolated per peer and reflected in peer status.

Users explicitly publish and unpublish eligible public, non-password-protected links from the link-management page or federation API. Imported public announcements are browsable at `/server/federation` and are stored separately from local links, so they cannot change or replace them. Disabling federation stops feed and sync operations while preserving imported records. Admins can purge imported announcements from the Federation page or `DELETE {api_base_path}/server/{admin_path}/federation/imported`; the purge leaves local links, publications, and peer records intact.

See [API Reference](api.md) for discovery, feed, user publication, and admin peer routes. Billing is an optional product capability; see [Billing](billing.md) for setup and the current API surface.

## I2P

Caslink supports an application-managed I2P eepsite. When `i2pd` is available, the server starts it with app-owned configuration, data, and log directories and connects to its SAM interface. Otherwise, the server can use an already-running SAM bridge at `server.i2p.sam_address`. SAM session control and stream forwarding use separate connections. The eepsite destination key is persisted so its address remains stable across restarts. The dedicated loopback HTTP backend is started before the SAM stream is forwarded.

I2P is enabled by the container-computed default, `server.i2p.enabled`, `I2P_ENABLED`, or `--i2p`, in increasing override precedence. Set `expose: true` to include a running I2P address in public autodiscovery. Admins can inspect status, change parameters, restart the provider, or explicitly regenerate the destination from the I2P network settings page.
