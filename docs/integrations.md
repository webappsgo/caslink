# Integrations

This page describes integration surfaces currently exposed by Caslink. The server does not provide native mobile-app association files or outbound webhooks.

## External identity providers

The admin interface supports configuring named OIDC, LDAP, and SAML providers at `/server/{admin_path}/security/auth/{type}`. Provider configuration is stored in the server configuration and sensitive values are encrypted. The provider API is under `{api_base_path}/server/{admin_path}/security/auth/{type}/providers` (default `{api_base_path}`: `/api/v1`); it requires an admin-scoped bearer token (`adm_`).

The browser and client sign-in flows depend on enabled providers. Configure and test a provider in the admin interface before enabling it for users. Use TLS for provider endpoints and keep client secrets, bind credentials, SAML signing keys, and metadata private. The server does not ship Google or GitHub-specific presets.

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

When Tor support is enabled and a Tor executable is available, the server can create a hidden service and report its address through server status and autodiscovery. Tor requires outbound network access. See [Configuration](configuration.md) for the supported settings.

## Standards endpoints

Caslink serves `/.well-known/security.txt`, `/.well-known/change-password`, and active ACME HTTP-01 challenge paths. Other `/.well-known/` paths are not served unless an explicitly enabled feature provides them. Unknown paths return `404`.

## Billing and federation

Billing and federation are described as optional product capabilities in the project definition. They are not exposed by the current server routes and are not enabled integrations in this build.

## I2P

Caslink supports an application-managed I2P eepsite. When `i2pd` is available, the server starts it with app-owned configuration, data, and log directories and connects to its SAM interface. Otherwise, the server can use an already-running SAM bridge at `server.i2p.sam_address`. SAM session control and stream forwarding use separate connections. The eepsite destination key is persisted so its address remains stable across restarts. The dedicated loopback HTTP backend is started before the SAM stream is forwarded.

I2P is enabled by the container-computed default, `server.i2p.enabled`, `I2P_ENABLED`, or `--i2p`, in increasing override precedence. Set `expose: true` to include a running I2P address in public autodiscovery. Admins can inspect status, change parameters, restart the provider, or explicitly regenerate the destination from the I2P network settings page.
