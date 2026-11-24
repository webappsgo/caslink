# Caslink

Caslink is a self-hosted URL shortener written in Go. The server provides link creation and redirection, click analytics, QR codes, bulk link operations, user and organization accounts, custom domains, and a server-admin interface. Web pages are server-rendered with Go templates; JavaScript enhances selected interactions.

## Capabilities

- Short links with custom codes and configurable password, visibility, expiry, and targeting rules
- Click statistics with referrer, device, and GeoIP enrichment
- QR generation and CSV/JSON link import and export
- User accounts, API tokens, TOTP, passkeys, and organizations
- Custom domain verification and certificate management
- Server configuration, scheduled maintenance, backup, and moderation tools
- REST and GraphQL APIs, OpenAPI documentation, and API autodiscovery
- English, Spanish, Chinese, French, Arabic, German, Italian, and Japanese translations

Optional integrations such as billing and federation are documented in [Integrations](integrations.md) with their current availability.

## Start here

- [Install Caslink](installation.md)
- [Configure the server](configuration.md)
- [Use the API](api.md)
- [Manage the server](admin.md)
- [Review the security model](security.md)
- [Configure integrations](integrations.md)
- [Build and contribute](development.md)
- [Translate the interface](translating.md)
