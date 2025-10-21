# Caslink API Documentation

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
- [Rate Limiting](#rate-limiting)
- [Error Handling](#error-handling)
- [API Endpoints](#api-endpoints)
  - [URLs](#urls)
  - [Analytics](#analytics)
  - [QR Codes](#qr-codes)
  - [Bulk Operations](#bulk-operations)
  - [Users](#users)
  - [Domains](#domains)
  - [Webhooks](#webhooks)
  - [Federation](#federation)
- [WebSocket API](#websocket-api)
- [GraphQL API](#graphql-api)
- [SDK Libraries](#sdk-libraries)

## Overview

The Caslink REST API provides programmatic access to all URL shortening, analytics, and management features. The API follows REST principles and returns JSON responses.

**Base URL:** `https://your-domain.com/api/v1`

**Content Type:** All requests and responses use `application/json`

**API Version:** v1 (current)

## Authentication

### API Tokens

API tokens are required for all authenticated endpoints. Create tokens in the dashboard or via CLI.

```bash
# Create API token via CLI
caslink user tokens create --name "My App Token"
```

**Authentication Header:**
```
Authorization: Bearer YOUR_API_TOKEN
```

**Example Request:**
```bash
curl -H "Authorization: Bearer your_token_here" \
     https://your-domain.com/api/v1/urls
```

### Session-Based Authentication

Web UI uses cookie-based sessions. Session cookies are httpOnly and secure in production.

## Rate Limiting

Rate limits are enforced per API token and IP address.

**Default Limits:**
- Authenticated: 1000 requests/hour
- Anonymous: 60 requests/hour

**Rate Limit Headers:**
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1609459200
```

**429 Response:**
```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded. Try again in 3600 seconds",
  "retry_after": 3600
}
```

## Error Handling

### Standard Error Response

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": {
    "field": "Additional context"
  }
}
```

### HTTP Status Codes

- `200 OK` - Request succeeded
- `201 Created` - Resource created
- `204 No Content` - Success with no response body
- `400 Bad Request` - Invalid request parameters
- `401 Unauthorized` - Missing or invalid authentication
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `409 Conflict` - Resource conflict (e.g., duplicate short code)
- `422 Unprocessable Entity` - Validation error
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Server error

### Common Error Codes

- `invalid_request` - Request validation failed
- `authentication_required` - Authentication token missing
- `invalid_token` - Authentication token invalid or expired
- `permission_denied` - Insufficient permissions
- `resource_not_found` - Requested resource not found
- `duplicate_resource` - Resource already exists
- `rate_limit_exceeded` - Rate limit exceeded
- `validation_error` - Input validation failed
- `server_error` - Internal server error

## API Endpoints

### URLs

#### Create Short URL

**POST** `/urls`

Create a new short URL.

**Request Body:**
```json
{
  "url": "https://example.com/very/long/url",
  "custom_code": "mycode",
  "title": "Example Page",
  "description": "Optional description",
  "expires_at": "2024-12-31T23:59:59Z",
  "password": "secret123",
  "tags": ["marketing", "campaign"],
  "utm_source": "newsletter",
  "utm_medium": "email",
  "utm_campaign": "spring2024"
}
```

**Response (201 Created):**
```json
{
  "id": "mycode",
  "original_url": "https://example.com/very/long/url",
  "short_url": "https://your-domain.com/mycode",
  "is_custom": true,
  "title": "Example Page",
  "description": "Optional description",
  "created_at": "2024-01-15T10:30:00Z",
  "expires_at": "2024-12-31T23:59:59Z",
  "clicks": 0,
  "unique_clicks": 0,
  "tags": ["marketing", "campaign"],
  "qr_code_url": "https://your-domain.com/api/v1/urls/mycode/qr"
}
```

#### List URLs

**GET** `/urls`

List all URLs for the authenticated user.

**Query Parameters:**
- `page` (int) - Page number (default: 1)
- `limit` (int) - Items per page (default: 20, max: 100)
- `sort` (string) - Sort field: `created_at`, `clicks`, `title` (default: `created_at`)
- `order` (string) - Sort order: `asc`, `desc` (default: `desc`)
- `search` (string) - Search in URL, title, description
- `tags` (string) - Filter by tags (comma-separated)
- `active` (bool) - Filter by active status

**Response (200 OK):**
```json
{
  "urls": [
    {
      "id": "abc123",
      "original_url": "https://example.com",
      "short_url": "https://your-domain.com/abc123",
      "title": "Example",
      "clicks": 42,
      "unique_clicks": 28,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

#### Get URL Details

**GET** `/urls/{id}`

Get detailed information about a specific URL.

**Response (200 OK):**
```json
{
  "id": "abc123",
  "original_url": "https://example.com/page",
  "short_url": "https://your-domain.com/abc123",
  "is_custom": false,
  "title": "Example Page",
  "description": "Page description",
  "favicon_url": "https://example.com/favicon.ico",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "expires_at": null,
  "clicks": 42,
  "unique_clicks": 28,
  "active": true,
  "password_protected": false,
  "tags": ["marketing"],
  "utm_params": {
    "source": "newsletter",
    "medium": "email"
  }
}
```

#### Update URL

**PATCH** `/urls/{id}`

Update URL properties.

**Request Body:**
```json
{
  "title": "Updated Title",
  "description": "Updated description",
  "active": true,
  "expires_at": "2024-12-31T23:59:59Z",
  "tags": ["updated", "tags"]
}
```

**Response (200 OK):**
```json
{
  "id": "abc123",
  "original_url": "https://example.com/page",
  "title": "Updated Title",
  "updated_at": "2024-01-16T14:20:00Z"
}
```

#### Delete URL

**DELETE** `/urls/{id}`

Delete a URL permanently.

**Response (204 No Content)**

#### Get URL Analytics Summary

**GET** `/urls/{id}/analytics`

Get analytics summary for a specific URL.

**Query Parameters:**
- `period` (string) - Time period: `24h`, `7d`, `30d`, `90d`, `all` (default: `30d`)
- `timezone` (string) - Timezone for data aggregation (default: UTC)

**Response (200 OK):**
```json
{
  "summary": {
    "total_clicks": 1250,
    "unique_clicks": 847,
    "click_through_rate": 0.68,
    "average_daily_clicks": 41.67
  },
  "timeline": [
    {
      "date": "2024-01-15",
      "clicks": 45,
      "unique_clicks": 32
    }
  ],
  "top_countries": [
    {
      "country_code": "US",
      "country_name": "United States",
      "clicks": 450,
      "percentage": 36.0
    }
  ],
  "top_referrers": [
    {
      "referrer": "google.com",
      "clicks": 320,
      "percentage": 25.6
    }
  ],
  "browsers": [
    {
      "browser": "Chrome",
      "clicks": 670,
      "percentage": 53.6
    }
  ],
  "devices": [
    {
      "device": "mobile",
      "clicks": 625,
      "percentage": 50.0
    }
  ]
}
```

### Analytics

#### Get Global Analytics

**GET** `/analytics`

Get aggregated analytics across all URLs.

**Query Parameters:**
- `period` (string) - Time period: `24h`, `7d`, `30d`, `90d`, `all`
- `timezone` (string) - Timezone for aggregation

**Response (200 OK):**
```json
{
  "summary": {
    "total_urls": 1523,
    "total_clicks": 45890,
    "total_unique_clicks": 31204,
    "active_urls": 1498
  },
  "timeline": [],
  "top_urls": [
    {
      "id": "popular",
      "clicks": 2340,
      "title": "Popular Link"
    }
  ]
}
```

#### Export Analytics Data

**GET** `/analytics/export`

Export analytics data in various formats.

**Query Parameters:**
- `format` (string) - Export format: `csv`, `json`, `pdf` (default: `csv`)
- `period` (string) - Time period
- `url_id` (string) - Specific URL ID (optional)

**Response:** File download with appropriate Content-Type

### QR Codes

#### Generate QR Code

**GET** `/urls/{id}/qr`

Generate a QR code for a URL.

**Query Parameters:**
- `size` (int) - Size in pixels (default: 200, max: 1000)
- `format` (string) - Format: `png`, `svg`, `pdf` (default: `png`)
- `style` (string) - Style: `square`, `circle`, `rounded` (default: `square`)
- `fg_color` (string) - Foreground color (hex, default: `000000`)
- `bg_color` (string) - Background color (hex, default: `FFFFFF`)
- `logo_url` (string) - Logo URL to embed (optional)

**Response:** Image file with appropriate Content-Type

**Example:**
```bash
# Download PNG QR code
curl "https://your-domain.com/api/v1/urls/abc123/qr?size=400&format=png" \
     -o qrcode.png

# Get SVG QR code
curl "https://your-domain.com/api/v1/urls/abc123/qr?format=svg&style=rounded"
```

### Bulk Operations

#### Import URLs

**POST** `/bulk/import`

Import multiple URLs from CSV or JSON.

**Request (multipart/form-data):**
```
file: urls.csv
format: csv
```

**CSV Format:**
```csv
url,custom_code,title,description,tags
https://example.com/1,code1,Title 1,Description 1,tag1;tag2
https://example.com/2,,Title 2,Description 2,tag3
```

**Response (202 Accepted):**
```json
{
  "job_id": "job_abc123",
  "status": "processing",
  "total_rows": 150,
  "status_url": "/api/v1/bulk/status/job_abc123"
}
```

#### Export URLs

**POST** `/bulk/export`

Export URLs to CSV or JSON.

**Request Body:**
```json
{
  "format": "csv",
  "filters": {
    "tags": ["marketing"],
    "active": true
  }
}
```

**Response (202 Accepted):**
```json
{
  "job_id": "job_xyz789",
  "status": "processing",
  "download_url": "/api/v1/bulk/download/job_xyz789"
}
```

#### Check Bulk Operation Status

**GET** `/bulk/status/{job_id}`

Check the status of a bulk operation.

**Response (200 OK):**
```json
{
  "job_id": "job_abc123",
  "status": "completed",
  "progress": {
    "processed": 150,
    "total": 150,
    "success": 148,
    "failed": 2
  },
  "errors": [
    {
      "row": 5,
      "error": "Invalid URL format"
    }
  ],
  "completed_at": "2024-01-15T10:35:00Z"
}
```

### Users

#### Get Current User

**GET** `/users/me`

Get authenticated user information.

**Response (200 OK):**
```json
{
  "id": "user_123",
  "username": "johndoe",
  "email": "john@example.com",
  "is_admin": false,
  "is_premium": true,
  "created_at": "2023-12-01T10:00:00Z",
  "timezone": "America/New_York",
  "theme": "dark",
  "stats": {
    "total_urls": 45,
    "total_clicks": 2340
  }
}
```

#### Update User Profile

**PATCH** `/users/me`

Update user profile settings.

**Request Body:**
```json
{
  "email": "newemail@example.com",
  "timezone": "Europe/London",
  "theme": "light"
}
```

#### List API Tokens

**GET** `/users/me/tokens`

List API tokens for the current user.

**Response (200 OK):**
```json
{
  "tokens": [
    {
      "id": "token_123",
      "name": "My App Token",
      "created_at": "2024-01-01T00:00:00Z",
      "last_used": "2024-01-15T10:30:00Z",
      "expires_at": null,
      "permissions": ["urls:read", "urls:write"]
    }
  ]
}
```

#### Create API Token

**POST** `/users/me/tokens`

Create a new API token.

**Request Body:**
```json
{
  "name": "New Token",
  "permissions": ["urls:read", "urls:write"],
  "expires_at": "2025-01-01T00:00:00Z"
}
```

**Response (201 Created):**
```json
{
  "id": "token_456",
  "name": "New Token",
  "token": "cl_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
  "created_at": "2024-01-15T10:30:00Z",
  "expires_at": "2025-01-01T00:00:00Z"
}
```

**Important:** The token value is only shown once. Store it securely.

#### Revoke API Token

**DELETE** `/users/me/tokens/{id}`

Revoke an API token.

**Response (204 No Content)**

### Domains

#### List Custom Domains

**GET** `/domains`

List custom domains for the current user.

**Response (200 OK):**
```json
{
  "domains": [
    {
      "id": "domain_123",
      "domain": "short.example.com",
      "verified": true,
      "ssl_enabled": true,
      "is_default": true,
      "created_at": "2024-01-01T00:00:00Z",
      "verified_at": "2024-01-01T01:00:00Z"
    }
  ]
}
```

#### Add Custom Domain

**POST** `/domains`

Add a new custom domain.

**Request Body:**
```json
{
  "domain": "short.example.com",
  "verification_method": "dns"
}
```

**Response (201 Created):**
```json
{
  "id": "domain_456",
  "domain": "short.example.com",
  "verified": false,
  "verification_method": "dns",
  "verification_token": "caslink-verify=abc123def456",
  "dns_records": [
    {
      "type": "TXT",
      "name": "_caslink-verify",
      "value": "abc123def456"
    },
    {
      "type": "CNAME",
      "name": "short.example.com",
      "value": "your-caslink-instance.com"
    }
  ]
}
```

#### Verify Domain

**POST** `/domains/{id}/verify`

Verify domain ownership.

**Response (200 OK):**
```json
{
  "id": "domain_456",
  "domain": "short.example.com",
  "verified": true,
  "verified_at": "2024-01-15T10:30:00Z"
}
```

### Webhooks

#### List Webhooks

**GET** `/webhooks`

List configured webhooks.

**Response (200 OK):**
```json
{
  "webhooks": [
    {
      "id": "webhook_123",
      "url": "https://example.com/webhook",
      "events": ["url.created", "url.clicked"],
      "active": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Create Webhook

**POST** `/webhooks`

Create a new webhook.

**Request Body:**
```json
{
  "url": "https://example.com/webhook",
  "events": ["url.created", "url.clicked", "url.deleted"],
  "secret": "your_webhook_secret"
}
```

**Response (201 Created):**
```json
{
  "id": "webhook_456",
  "url": "https://example.com/webhook",
  "events": ["url.created", "url.clicked", "url.deleted"],
  "active": true,
  "created_at": "2024-01-15T10:30:00Z"
}
```

#### Webhook Events

Available webhook events:

- `url.created` - URL created
- `url.updated` - URL updated
- `url.deleted` - URL deleted
- `url.clicked` - URL clicked
- `url.expired` - URL expired
- `user.created` - User registered
- `domain.verified` - Domain verified

**Webhook Payload Example:**
```json
{
  "event": "url.clicked",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "url_id": "abc123",
    "original_url": "https://example.com",
    "short_url": "https://your-domain.com/abc123",
    "click": {
      "ip_address": "192.168.1.1",
      "user_agent": "Mozilla/5.0...",
      "country_code": "US",
      "referrer": "https://google.com"
    }
  }
}
```

### Federation

#### List Federated Instances

**GET** `/federation/instances`

List federated instances.

**Response (200 OK):**
```json
{
  "instances": [
    {
      "domain": "caslink.example.com",
      "discovered_at": "2024-01-01T00:00:00Z",
      "last_sync": "2024-01-15T10:00:00Z",
      "active": true
    }
  ]
}
```

#### Search Federated URLs

**GET** `/federation/urls/search`

Search for URLs across federated instances.

**Query Parameters:**
- `q` (string) - Search query
- `instance` (string) - Filter by instance domain

**Response (200 OK):**
```json
{
  "urls": [
    {
      "id": "abc123",
      "original_url": "https://example.com",
      "short_url": "https://caslink.example.com/abc123",
      "instance": "caslink.example.com",
      "title": "Example Page"
    }
  ]
}
```

## WebSocket API

Real-time updates via WebSocket connection.

**WebSocket URL:** `wss://your-domain.com/api/v1/ws`

**Authentication:** Send token in first message or query parameter

**Connect with Token:**
```javascript
const ws = new WebSocket('wss://your-domain.com/api/v1/ws?token=YOUR_TOKEN');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

**Message Types:**

Subscribe to URL clicks:
```json
{
  "type": "subscribe",
  "channel": "url.clicks",
  "url_id": "abc123"
}
```

Real-time click event:
```json
{
  "type": "event",
  "channel": "url.clicks",
  "data": {
    "url_id": "abc123",
    "clicks": 43,
    "country_code": "US"
  }
}
```

## GraphQL API

GraphQL endpoint for advanced queries.

**Endpoint:** `POST /api/v1/graphql`

**Example Query:**
```graphql
query {
  urls(limit: 10, sort: CLICKS_DESC) {
    id
    originalUrl
    shortUrl
    title
    clicks
    uniqueClicks
    createdAt
    analytics(period: LAST_30_DAYS) {
      totalClicks
      topCountries {
        countryCode
        clicks
      }
    }
  }
}
```

**Example Mutation:**
```graphql
mutation {
  createUrl(input: {
    url: "https://example.com"
    customCode: "mycode"
    title: "Example"
  }) {
    id
    shortUrl
  }
}
```

## SDK Libraries

Official SDK libraries for popular languages:

**JavaScript/TypeScript:**
```bash
npm install @caslink/client
```

```javascript
import { CaslinkClient } from '@caslink/client';

const client = new CaslinkClient({
  baseUrl: 'https://your-domain.com',
  token: 'YOUR_API_TOKEN'
});

const url = await client.urls.create({
  url: 'https://example.com',
  customCode: 'mycode'
});
```

**Python:**
```bash
pip install caslink
```

```python
from caslink import CaslinkClient

client = CaslinkClient(
    base_url='https://your-domain.com',
    token='YOUR_API_TOKEN'
)

url = client.urls.create(
    url='https://example.com',
    custom_code='mycode'
)
```

**Go:**
```bash
go get github.com/casjaysdevdocker/caslink-go
```

```go
import "github.com/casjaysdevdocker/caslink-go"

client := caslink.NewClient(
    "https://your-domain.com",
    "YOUR_API_TOKEN",
)

url, err := client.URLs.Create(context.Background(), &caslink.CreateURLInput{
    URL: "https://example.com",
    CustomCode: "mycode",
})
```

**PHP:**
```bash
composer require caslink/client
```

```php
use Caslink\Client;

$client = new Client([
    'base_url' => 'https://your-domain.com',
    'token' => 'YOUR_API_TOKEN',
]);

$url = $client->urls->create([
    'url' => 'https://example.com',
    'custom_code' => 'mycode',
]);
```

## Best Practices

1. **Use HTTPS:** Always use HTTPS for API requests in production
2. **Store Tokens Securely:** Never commit API tokens to version control
3. **Handle Rate Limits:** Implement exponential backoff for rate limit errors
4. **Validate Input:** Always validate URLs and parameters before sending
5. **Use Webhooks:** For real-time updates, use webhooks instead of polling
6. **Cache Responses:** Cache URL data when appropriate to reduce API calls
7. **Handle Errors:** Implement proper error handling for all requests
8. **Use Pagination:** Always paginate large result sets
9. **Monitor Usage:** Track API usage to stay within rate limits
10. **Version Pinning:** Pin to specific API version for production apps

## Support

For API support and questions:

- Documentation: https://docs.caslink.example.com
- GitHub Issues: https://github.com/casjaysdevdocker/caslink/issues
- Email: support@caslink.example.com
