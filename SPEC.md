## Optional parts
- PART 34: REQUIRED
- PART 35: REQUIRED
- PART 36: REQUIRED

## Billing technical contract

Billing is an optional operator monetization module from IDEA.md. It never gates, removes, or changes access to Caslink features: plans describe prices and usage allowances, but every user receives the same application features. If no external provider is configured, the module operates in invoice-only/manual-payment mode.

### Data and money

- Link each billing account to exactly one existing regular-user or organization owner. Server Admins manage the instance billing configuration; they are not customer billing accounts.
- Use SQLite in single-node mode and the selected cluster database in cluster mode, following the existing store transaction and migration conventions. Tables are `billing_accounts`, `billing_plans`, `billing_subscriptions`, `billing_events`, `billing_providers`, `billing_payment_methods`, `billing_payment_attempts`, `billing_invoices`, `billing_invoice_items`, `billing_credit_notes`, `billing_usage_meters`, `billing_usage_records`, `billing_tax_rates`, `billing_tax_records`, `billing_webhook_events`, and append-only `billing_audit_log`.
- Persist all amounts as signed 64-bit integers in the currency's smallest unit. Persist ISO 4217 currency codes uppercase and timestamps as UTC Unix seconds. Never use floating-point for money.
- Provider credentials and payment tokens are encrypted with the installation's at-rest encryption key and stored only in the database. Never store card or bank account numbers, security codes, credentials in YAML, environment variables, logs, templates, or source. Credential changes and access are audited.
- Invoice numbering is sequential per installation and invoice prefix, allocated atomically. Issued invoices are immutable. Corrections, refunds, and cancellations use linked credit notes or explicit state transitions; they never rewrite issued line items or totals.
- Billing state changes and financial actions append a UTC audit row containing actor type/id, action, target, provider, result code, request IP when available, and correlation/idempotency key. The audit service exposes append/read operations only.

### Product and lifecycle rules

- Plans are operator-created and can define monthly/yearly prices, trials, grace periods, usage meters, and public/hidden availability. No plan may make an existing feature unavailable or create a feature paywall. Meter limits only apply to limits explicitly configured by the operator and disclosed to the customer.
- Subscription states are `PENDING_ACTIVATION`, `TRIALING`, `ACTIVE`, `PAST_DUE`, `PAUSED`, `CANCELLED`, and `EXPIRED`. Only the Billing Lifecycle transitions in the referenced BILLING template are accepted. Invalid transitions return `VALIDATION_FAILED` and create no side effects.
- A payment failure generates an invoice, enters a configurable seven-day grace period by default, schedules idempotent retries, and sends notices. The service remains available during grace. Cancellation is always available; it defaults to the end of the paid period and requires explicit confirmation for immediate cancellation.
- Renewal, dunning, trial notices, usage period aggregation, provider health, and daily reconciliation run as internal scheduler tasks. Each action is idempotent across retries and cluster execution. Provider failure never suppresses invoice creation or immediately suspends the application account.
- Usage meters support `Counter`, `Gauge`, and `Histogram`, `HARD_RESET`, `ROLLING_WINDOW`, `CARRY_FORWARD`, and `ACCUMULATING` reset policies, and `BLOCK`, `ALLOW_WITH_CHARGE`, `ALLOW_WITH_WARNING`, `AUTO_UPGRADE`, and `THROTTLE` policies. Meter mutations are atomic; billing-period summaries are immutable snapshots.

### Providers and tax

- The provider registry contains `stripe`, `paddle`, `paypal`, `lemonsqueezy`, and the built-in `manual` option because these are the providers named by IDEA.md. External processors are disabled on first run. Manual invoice tracking remains available with zero processors configured.
- Provider settings are configured only in the authenticated admin UI. Each provider implements credential validation, capability reporting, payment-method tokenization, authorization/capture/refund, subscription lifecycle operations when supported, transaction lookup/listing, webhook signature validation, and webhook event mapping. Unsupported capabilities return a typed unsupported-capability error; they never silently succeed.
- Provider states are `UNCONFIGURED`, `TESTING`, `ACTIVE`, `DEGRADED`, `MAINTENANCE`, `FAILED`, `DEPRECATED`, and `DISABLED`. Only explicitly enabled, validated providers receive requests or health checks. Failover follows configured priority and uses a unique idempotency key per logical payment.
- Provider webhooks require provider-specific signature validation, bounded request bodies, timestamp/replay checks, and unique event IDs. Duplicate events are acknowledged without repeating state transitions. Failed processing is retried by the internal scheduler and eventually appears in the admin review queue.
- Tax is calculated server-side from a billing address, service classification, configured tax jurisdiction/rate, exemption, and verified business tax ID. The service supports VAT, GST, sales tax, reverse-charge records, and tax reports. Tax-rate/provider outages use the last valid cached rate or a configured default, flag the invoice for review, and still issue the invoice. Tax records store the rate and rule inputs used at calculation time.

### HTTP contract

All routes are rooted below `/api/{api_version}` and use the standard JSON envelope, validation, rate limiting, CSRF protection for cookie-authenticated writes, and existing account/organization authorization rules.

- Customer routes under `/billing`: `GET/POST /accounts`, `GET/PATCH /accounts/{account_id}`, `GET/POST /accounts/{account_id}/subscriptions`, `GET/PATCH/DELETE /subscriptions/{subscription_id}`, `POST /subscriptions/{subscription_id}/cancel`, `POST /subscriptions/{subscription_id}/resume`, `GET /accounts/{account_id}/usage`, `GET /accounts/{account_id}/invoices`, `GET /invoices/{invoice_id}`, `GET /invoices/{invoice_id}/export?format=pdf|csv`, `POST /accounts/{account_id}/payment-methods`, and `DELETE /payment-methods/{method_id}`.
- Admin routes under `/server/{admin_path}/billing`: CRUD for plans and settings; list/update provider configuration and priority; provider credential test/enable/disable; tax rules; invoices, credit notes, reconciliation, webhook review, reports, and append-only audit reads. All require a Server Admin session or admin-scoped bearer token and sensitive changes require the established reauthentication flow.
- Provider callback routes are `/billing/webhooks/{provider}`. They accept only that provider's signature contract, do not use browser sessions, and return success only after durable event acceptance.
- Exported billing data is portable JSON or CSV and includes invoices, payment attempts, subscription history, credit notes, and associated currency/tax snapshots. It excludes provider credentials and raw payment data.

### Notifications and administration

- Emit localized notifications for payment success/failure, invoice issue, trial ending, upcoming renewal, grace start, suspension, and cancellation through the existing notification service. Marketing messages remain opt-in.
- Provider UI lists all five providers grouped by category and disabled status, includes a setup guide, a help tooltip for every field, sandbox/test mode, connection diagnostics, priority, and health. Credentials are write-only after save.
- The admin dashboard reports MRR/ARR, churn, ARPU, LTV, subscription states, payment outcomes, provider health, and daily reconciliation. Disabled providers are never probed.
- The UI includes customer subscription and payment-method management, invoice PDF/CSV downloads, usage and overage views, cancellation without dark patterns, and a portable data export request.
