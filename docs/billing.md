# Billing

Billing is an optional operator monetization module. It never gates or removes
Caslink features. If no external processor is enabled, administrators can use
manual invoice mode.

## Administrator setup

Open `/server/admin/billing` while signed in as a Server Admin. Create plans,
save provider credentials, test a provider, then enable it and choose its
priority. External providers start disabled. Credentials are encrypted with
the installation encryption key and are write-only after saving. Configure
the key in the server security settings before using billing.

The provider registry contains Stripe, Paddle, PayPal, and manual invoicing.
Stripe and PayPal adapters currently provide hosted one-time checkout and
refunds. Paddle provides hosted checkout and signed webhook verification.
Manual mode supports invoice tracking without an external processor. Hosted
checkout initiation is not yet connected to customer subscription creation.
Provider-specific credentials and setup requirements appear in the admin panel.

Paddle hosted checkout needs a Paddle `price_id` for each plan intended for
Paddle. Save each value in the plan's provider credentials as `price_id_<plan
ID>`. Provider credentials and payment data must never be added to YAML or
source files.

## Customer billing

Signed-in users can manage their billing profile, view plans, manage
subscriptions, and download invoice PDFs or CSV files at `/users/billing`.
Customer APIs are rooted at `{api_base_path}/billing` and require a user bearer
token. Account access is derived from the authenticated user; a supplied
account ID is checked against that user's account.

The API includes billing account profile, subscriptions, invoices, usage
summaries, and payment-method routes. Invoice exports contain invoice totals
and line items. Provider credentials and raw payment data are never included.

## Webhooks and scheduled work

Provider callbacks are posted to
`{api_base_path}/billing/webhooks/{provider}`. They do not use browser
sessions. Requests have a 1 MiB body limit, provider signature and timestamp
validation, and event-ID deduplication before acknowledgement. Accepted event
payloads are encrypted at rest.

The internal scheduler checks enabled provider health every five minutes and
reconciles billing invoices daily at 02:00 in the configured scheduler
timezone. Disabled providers are not probed.

## Current API surface

| Method | Path | Purpose |
|---|---|---|
| GET, POST | `{api_base_path}/billing/accounts` | Read or create the authenticated user's billing account |
| GET, PATCH | `{api_base_path}/billing/accounts/{account_id}` | Read or update the authenticated user's billing account |
| GET, POST | `{api_base_path}/billing/accounts/{account_id}/subscriptions` | List or create subscriptions |
| GET, PATCH, DELETE | `{api_base_path}/billing/subscriptions/{subscription_id}` | Read or cancel an owned subscription; PATCH is currently unsupported |
| GET | `{api_base_path}/billing/accounts/{account_id}/usage` | Read account usage aggregates |
| GET | `{api_base_path}/billing/accounts/{account_id}/invoices` | List invoices |
| GET | `{api_base_path}/billing/invoices/{invoice_id}` | Read an account-owned invoice |
| GET | `{api_base_path}/billing/invoices/{invoice_id}/export?format=pdf\|csv` | Download an account-owned invoice |
| GET, POST | `{api_base_path}/billing/accounts/{account_id}/payment-methods` | List or save a provider token reference |
| DELETE | `{api_base_path}/billing/payment-methods/{method_id}` | Delete an account-owned payment method |
| POST | `{api_base_path}/billing/subscriptions/{subscription_id}/cancel` | Cancel at period end or request confirmed immediate cancellation |
| POST | `{api_base_path}/billing/subscriptions/{subscription_id}/resume` | Resume a period-end cancellation |
| POST | `{api_base_path}/billing/webhooks/{provider}` | Accept a verified provider event |

Admin APIs are rooted at `{api_base_path}/server/admin/billing` and require an
admin-scoped bearer token. They expose plan and provider operations and the
append-only billing audit read API. Browser administration is under
`/server/admin/billing` and uses the normal authenticated admin session and
CSRF protection. The admin API can list or issue invoices and record manual
payments at `/invoices`, `/invoices/{invoice_id}/payments` respectively.
