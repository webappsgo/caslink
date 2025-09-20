CASLINK PROJECT SPECIFICATION v2.0 - COMPLETE IMPLEMENTATION GUIDE

PROJECT OVERVIEW:
caslink is a secure, mobile-first, feature-rich, fully self-hosted URL shortener application written in Go 1.22+ that compiles into a single static binary with zero external dependencies. It provides universal feature parity across all deployment types (self-hosted, SaaS, enterprise) with optional billing, comprehensive database support, automatic proxy detection, headless-first design, and production-ready migration system. The application follows the principle that everyone gets the same full feature set regardless of hosting method, with billing being purely an optional monetization layer rather than a feature gate.

CORE PHILOSOPHY AND DESIGN PRINCIPLES:
1. UNIVERSAL FEATURE PARITY: All users (self-hosted, SaaS, enterprise) receive identical feature sets with no artificial restrictions or limitations
2. HEADLESS-FIRST: Designed primarily for headless server environments (VPS, containers, cloud) with terminal-based setup and configuration
3. ZERO-CONFIG STARTUP: Works immediately upon first run with intelligent defaults, auto-port selection, and progressive configuration
4. STANDARDS COMPLIANT: Uses standard environment variables, RFC-compliant headers, and industry-standard protocols
5. DATABASE AGNOSTIC: Supports all major databases with intelligent migration system and connection string flexibility
6. OPTIONAL BILLING: Billing is purely for business operations, never for feature access - all features always available
7. PROXY AWARE: Automatically detects and handles all major reverse proxy configurations and load balancers
8. PRODUCTION READY: Built for scale with connection pooling, migration safety, audit trails, and comprehensive error handling

EXACT DIRECTORY STRUCTURE AND FILE LAYOUT:
```
caslink/
├── cmd/
│   ├── main.go                          # Primary application entry point with cobra CLI
│   └── caslink-cli/
│       └── main.go                      # Standalone CLI tool entry point
├── internal/
│   ├── server/
│   │   ├── server.go                    # HTTP server initialization and lifecycle management
│   │   ├── handlers.go                  # Web UI HTTP handlers (home, create, view, etc.)
│   │   ├── api.go                       # REST API endpoints for URL operations
│   │   ├── middleware.go                # HTTP middleware (CORS, security, proxy, rate limiting)
│   │   ├── helpers.go                   # Server utility functions and response helpers
│   │   ├── templates/
│   │   │   ├── base.html               # Base HTML template with navigation and layout
│   │   │   ├── home.html               # Homepage with URL creation form
│   │   │   ├── setup/
│   │   │   │   ├── admin.html          # First-run admin account creation
│   │   │   │   ├── first-url.html      # First URL creation tutorial
│   │   │   │   └── customize.html      # Optional customization step
│   │   │   ├── url.html                # URL details and analytics view
│   │   │   ├── dashboard.html          # User dashboard with URL list and stats
│   │   │   ├── analytics.html          # Detailed analytics and reporting
│   │   │   ├── bulk.html               # Bulk import/export interface
│   │   │   ├── qr.html                 # QR code generation interface
│   │   │   ├── login.html              # User authentication form
│   │   │   ├── register.html           # User registration form
│   │   │   ├── profile.html            # User profile and settings
│   │   │   ├── admin/
│   │   │   │   ├── dashboard.html      # Admin dashboard overview
│   │   │   │   ├── users.html          # User management interface
│   │   │   │   ├── settings.html       # Server configuration interface
│   │   │   │   ├── database.html       # Database configuration and testing
│   │   │   │   ├── billing.html        # Billing configuration (if enabled)
│   │   │   │   └── migrations.html     # Migration status and controls
│   │   │   ├── billing/
│   │   │   │   ├── plans.html          # Billing plan selection
│   │   │   │   ├── subscription.html   # Subscription management
│   │   │   │   ├── usage.html          # Usage monitoring
│   │   │   │   └── invoices.html       # Invoice history
│   │   │   └── error.html              # Error page template
│   │   └── static/
│   │       ├── css/
│   │       │   ├── main.css            # Primary stylesheet with responsive design
│   │       │   ├── dashboard.css       # Dashboard-specific styles
│   │       │   ├── analytics.css       # Analytics page styles
│   │       │   ├── admin.css           # Admin interface styles
│   │       │   └── themes/
│   │       │       ├── dark.css        # Dark theme variables and overrides
│   │       │       ├── light.css       # Light theme variables and overrides
│   │       │       └── professional.css # Professional theme for business use
│   │       ├── js/
│   │       │   ├── main.js             # Core JavaScript functionality
│   │       │   ├── dashboard.js        # Dashboard interactions and real-time updates
│   │       │   ├── analytics.js        # Analytics charts and data visualization
│   │       │   ├── bulk.js             # Bulk operations and file handling
│   │       │   ├── qr.js               # QR code generation and customization
│   │       │   ├── admin.js            # Admin interface functionality
│   │       │   └── setup.js            # First-run setup wizard
│   │       └── fonts/                  # Web fonts for consistent typography
│   ├── config/
│   │   ├── config.go                   # Configuration loading and validation
│   │   ├── defaults.go                 # Default configuration values
│   │   ├── environment.go              # Environment variable parsing
│   │   └── validation.go               # Configuration validation rules
│   ├── db/
│   │   ├── db.go                       # Database connection and initialization
│   │   ├── models.go                   # Database models and structs
│   │   ├── sqlite.go                   # SQLite-specific implementation
│   │   ├── postgresql.go               # PostgreSQL-specific implementation
│   │   ├── mysql.go                    # MySQL/MariaDB-specific implementation
│   │   ├── sqlserver.go                # SQL Server-specific implementation
│   │   └── migrations/
│   │       ├── service.go              # Migration service with validation and rollback
│   │       ├── runner.go               # Migration execution engine
│   │       ├── validator.go            # Migration validation logic
│   │       ├── rollback.go             # Rollback planning and execution
│   │       ├── backup.go               # Database backup functionality
│   │       └── migrations/
│   │           ├── 001_initial_schema.go        # Initial database schema
│   │           ├── 002_add_analytics.go         # Analytics tables
│   │           ├── 003_add_billing.go           # Billing system tables
│   │           ├── 004_add_federation.go        # Federation support
│   │           └── 005_add_audit_logs.go        # Audit logging tables
│   ├── url/
│   │   ├── service.go                  # URL business logic and operations
│   │   ├── models.go                   # URL-related data structures
│   │   ├── validation.go               # URL validation and sanitization
│   │   ├── shortcode.go                # Short code generation and management
│   │   ├── suggestions.go              # Custom code suggestion algorithm
│   │   └── expiration.go               # URL expiration handling
│   ├── analytics/
│   │   ├── service.go                  # Analytics data collection and processing
│   │   ├── collector.go                # Click tracking and data collection
│   │   ├── aggregator.go               # Data aggregation and statistics
│   │   ├── geolocation.go              # IP geolocation and geographic data
│   │   ├── reports.go                  # Report generation and export
│   │   └── realtime.go                 # Real-time analytics updates
│   ├── qr/
│   │   ├── service.go                  # QR code generation service
│   │   ├── generator.go                # QR code creation with customization
│   │   ├── styles.go                   # QR code styling options
│   │   └── formats.go                  # Multiple output format support
│   ├── bulk/
│   │   ├── service.go                  # Bulk operations coordination
│   │   ├── importer.go                 # CSV/JSON import functionality
│   │   ├── exporter.go                 # Data export in multiple formats
│   │   ├── validator.go                # Bulk data validation
│   │   └── processor.go                # Background bulk processing
│   ├── auth/
│   │   ├── service.go                  # Authentication service coordination
│   │   ├── session.go                  # Session management and storage
│   │   ├── password.go                 # Password hashing and validation
│   │   ├── tokens.go                   # API token generation and validation
│   │   ├── oauth.go                    # OAuth provider integration
│   │   ├── webauthn.go                 # WebAuthn/FIDO2 support
│   │   ├── totp.go                     # TOTP/2FA implementation
│   │   └── permissions.go              # Role-based access control
│   ├── billing/
│   │   ├── service.go                  # Billing service coordination
│   │   ├── plans.go                    # Subscription plan management
│   │   ├── subscriptions.go            # Subscription lifecycle management
│   │   ├── usage.go                    # Usage tracking and metering
│   │   ├── invoicing.go                # Invoice generation and management
│   │   ├── payments.go                 # Payment processing coordination
│   │   ├── webhooks.go                 # Payment provider webhook handling
│   │   ├── dunning.go                  # Failed payment recovery
│   │   └── providers/
│   │       ├── stripe.go               # Stripe payment integration
│   │       ├── paypal.go               # PayPal payment integration
│   │       ├── paddle.go               # Paddle payment integration
│   │       ├── lemonsqueezy.go         # LemonSqueezy integration
│   │       └── manual.go               # Manual/enterprise billing
│   ├── federation/
│   │   ├── service.go                  # Federation service coordination
│   │   ├── discovery.go                # Instance discovery via DNS and well-known
│   │   ├── client.go                   # Federation client for remote instances
│   │   ├── server.go                   # Federation server for sharing URLs
│   │   ├── sync.go                     # Synchronization with federated instances
│   │   └── protocol.go                 # Federation protocol implementation
│   ├── webhook/
│   │   ├── service.go                  # Webhook service coordination
│   │   ├── dispatcher.go               # Webhook event dispatching
│   │   ├── queue.go                    # Webhook delivery queue
│   │   ├── retry.go                    # Failed webhook retry logic
│   │   └── validation.go               # Webhook payload validation
│   ├── domains/
│   │   ├── service.go                  # Custom domain management
│   │   ├── verification.go             # Domain ownership verification
│   │   ├── ssl.go                      # SSL certificate management
│   │   └── routing.go                  # Domain-based request routing
│   ├── proxy/
│   │   ├── detector.go                 # Automatic proxy detection
│   │   ├── headers.go                  # Proxy header parsing and validation
│   │   ├── trust.go                    # Trusted proxy network management
│   │   └── client.go                   # Client information extraction
│   ├── scheduler/
│   │   ├── scheduler.go                # Task scheduler using cron
│   │   ├── tasks.go                    # Scheduled task definitions
│   │   ├── cleanup.go                  # Data cleanup tasks
│   │   └── maintenance.go              # Database maintenance tasks
│   ├── notifications/
│   │   ├── service.go                  # Notification service coordination
│   │   ├── email.go                    # Email notification sending
│   │   ├── templates.go                # Email template management
│   │   └── providers/
│   │       ├── smtp.go                 # SMTP email provider
│   │       ├── sendgrid.go             # SendGrid integration
│   │       └── ses.go                  # AWS SES integration
│   └── cli/
│       ├── root.go                     # Root CLI command definition
│       ├── server.go                   # Server management commands
│       ├── config.go                   # Configuration management commands
│       ├── url.go                      # URL management commands
│       ├── user.go                     # User management commands
│       ├── analytics.go                # Analytics and reporting commands
│       ├── bulk.go                     # Bulk operation commands
│       ├── billing.go                  # Billing management commands
│       ├── migrate.go                  # Database migration commands
│       ├── backup.go                   # Backup and restore commands
│       └── completions.go              # Shell completion generation
├── scripts/
│   ├── Makefile                        # Build automation and development tasks
│   ├── build.sh                        # Cross-platform build script
│   ├── install.sh                      # Installation script for various platforms
│   ├── backup.sh                       # Database backup script
│   ├── restore.sh                      # Database restore script
│   ├── deploy.sh                       # Deployment automation script
│   └── docker/
│       ├── Dockerfile                  # Production Docker image
│       ├── Dockerfile.dev              # Development Docker image
│       ├── docker-compose.yml          # Single-instance deployment
│       ├── docker-compose.dev.yml      # Development environment
│       ├── docker-compose.prod.yml     # Production deployment with external DB
│       └── docker-entrypoint.sh        # Docker container initialization
├── docs/
│   ├── API.md                          # Complete API documentation
│   ├── CONFIGURATION.md                # Configuration reference
│   ├── DEPLOYMENT.md                   # Deployment guides for various platforms
│   ├── FEDERATION.md                   # Federation protocol documentation
│   ├── BILLING.md                      # Billing system documentation
│   ├── MIGRATION.md                    # Database migration guide
│   ├── DEVELOPMENT.md                  # Development setup and contribution guide
│   ├── SECURITY.md                     # Security considerations and best practices
│   └── examples/
│       ├── nginx.conf                  # Nginx reverse proxy configuration
│       ├── apache.conf                 # Apache reverse proxy configuration
│       ├── caddy.conf                  # Caddy reverse proxy configuration
│       ├── systemd.service             # systemd service file
│       └── config-examples/
│           ├── basic.toml              # Basic self-hosted configuration
│           ├── production.toml         # Production configuration
│           └── enterprise.toml         # Enterprise configuration
├── tests/
│   ├── unit/                           # Unit tests for all packages
│   ├── integration/                    # Integration tests
│   ├── api/                            # API endpoint tests
│   ├── cli/                            # CLI command tests
│   ├── load/                           # Load testing scripts
│   └── fixtures/                       # Test data and fixtures
├── migrations/                         # Database migration files
├── go.mod                              # Go module definition
├── go.sum                              # Go module checksums
├── README.md                           # Project documentation and quick start
├── LICENSE                             # MIT license file
├── CHANGELOG.md                        # Version history and changes
├── CONTRIBUTING.md                     # Contribution guidelines
├── SECURITY.md                         # Security policy and reporting
├── .env.example                        # Example environment configuration
├── config.toml.example                 # Example configuration file
├── .gitignore                          # Git ignore patterns
├── .dockerignore                       # Docker ignore patterns
└── .github/
    ├── workflows/
    │   ├── build.yml                   # GitHub Actions build pipeline
    │   ├── test.yml                    # GitHub Actions test pipeline
    │   ├── security.yml                # GitHub Actions security scanning
    │   └── release.yml                 # GitHub Actions release automation
    ├── ISSUE_TEMPLATE/                 # Issue templates
    └── PULL_REQUEST_TEMPLATE.md        # Pull request template
```

GO MODULE AND DEPENDENCY CONFIGURATION:
Module name: github.com/casjaysdevdocker/caslink
Go version requirement: 1.22+
Direct dependencies with exact version constraints:
- github.com/gorilla/mux v1.8.1 (HTTP routing and middleware)
- github.com/gorilla/sessions v1.2.2 (HTTP session management)
- github.com/gorilla/websocket v1.5.1 (WebSocket support for real-time features)
- github.com/mattn/go-sqlite3 v1.14.19 (SQLite database driver)
- github.com/lib/pq v1.10.9 (PostgreSQL database driver)
- github.com/go-sql-driver/mysql v1.7.1 (MySQL/MariaDB database driver)
- github.com/denisenkom/go-mssqldb v1.6.0 (SQL Server database driver)
- github.com/jackc/pgx/v5 v5.5.1 (Advanced PostgreSQL driver and toolkit)
- github.com/spf13/cobra v1.8.0 (CLI framework and command parsing)
- github.com/spf13/viper v1.18.2 (Configuration management)
- github.com/spf13/pflag v1.0.5 (POSIX-compatible command-line flags)
- golang.org/x/crypto v0.17.0 (Cryptographic functions and password hashing)
- golang.org/x/oauth2 v0.15.0 (OAuth2 client implementation)
- golang.org/x/net v0.19.0 (Extended networking libraries)
- github.com/robfig/cron/v3 v3.0.1 (Cron-like task scheduling)
- github.com/go-webauthn/webauthn v0.10.2 (WebAuthn/FIDO2 authentication)
- github.com/pelletier/go-toml/v2 v2.1.1 (TOML configuration parsing)
- github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e (QR code generation)
- github.com/oschwald/geoip2-golang v1.9.0 (IP geolocation using MaxMind GeoIP2)
- github.com/ua-parser/uap-go v0.0.0-20211112212520-00c877edfe0f (User agent parsing)
- github.com/google/uuid v1.5.0 (UUID generation for unique identifiers)
- github.com/patrickmn/go-cache v2.1.0+incompatible (In-memory caching)
- github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 (Font rendering for QR codes)
- github.com/rs/cors v1.10.1 (CORS middleware)
- github.com/go-playground/validator/v10 v10.16.0 (Struct validation)
- github.com/dgrijalva/jwt-go v3.2.0+incompatible (JWT token handling)
- github.com/stripe/stripe-go/v76 v76.16.0 (Stripe payment processing)
- github.com/go-redis/redis/v8 v8.11.5 (Redis client for caching and sessions)
- github.com/prometheus/client_golang v1.17.0 (Prometheus metrics)
- github.com/sirupsen/logrus v1.9.3 (Structured logging)
- gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df (Email sending)

BUILD SYSTEM AND COMPILATION:
Build targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
CGO requirement: Enabled for SQLite support
Static linking: All dependencies statically linked for single-binary deployment
Embedding: All static assets (CSS, JS, templates) embedded using embed.FS
Build flags: -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE) -s -w"
Binary name: caslink (with OS-specific extensions)
Minimum binary size optimization through build flags and dependency management

COMPREHENSIVE ENVIRONMENT VARIABLE CONFIGURATION:
All configuration follows the CASLINK_ prefix standard with hierarchical organization.

SERVER CONFIGURATION:
CASLINK_SERVER_HOST=0.0.0.0 (server bind address, default all interfaces)
CASLINK_SERVER_PORT=auto (server port, auto-selects 64000-65535 range and persists)
CASLINK_SERVER_BEHIND_PROXY=auto (auto-detect proxy presence)
CASLINK_SERVER_TRUSTED_PROXIES="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" (CIDR networks)
CASLINK_SERVER_REAL_IP_HEADER=auto (auto-detect best proxy header)
CASLINK_SERVER_READ_TIMEOUT=30s (HTTP read timeout)
CASLINK_SERVER_WRITE_TIMEOUT=30s (HTTP write timeout)
CASLINK_SERVER_IDLE_TIMEOUT=120s (HTTP idle timeout)
CASLINK_SERVER_MAX_HEADER_BYTES=1048576 (1MB max header size)

DATABASE CONFIGURATION (METHOD 1 - CONNECTION STRING):
CASLINK_DATABASE_URL="sqlite:///var/lib/caslink/caslink.db" (complete connection string)

DATABASE CONFIGURATION (METHOD 2 - INDIVIDUAL FIELDS):
CASLINK_DATABASE_TYPE=sqlite (sqlite|postgresql|mysql|mariadb|sqlserver|cockroachdb)
CASLINK_DATABASE_HOST=localhost (database server hostname)
CASLINK_DATABASE_PORT=auto (auto-detected based on database type)
CASLINK_DATABASE_NAME=caslink (database name)
CASLINK_DATABASE_USERNAME=caslink (database username)
CASLINK_DATABASE_PASSWORD="" (database password)
CASLINK_DATABASE_SSL_MODE=auto (ssl mode, auto-detected based on database type)
CASLINK_DATABASE_MAX_OPEN_CONNS=25 (maximum open connections)
CASLINK_DATABASE_MAX_IDLE_CONNS=5 (maximum idle connections)
CASLINK_DATABASE_CONN_MAX_LIFETIME=300s (connection maximum lifetime)
CASLINK_DATABASE_AUTO_MIGRATE=true (automatically run migrations on startup)
CASLINK_DATABASE_MIGRATION_TIMEOUT=300s (migration timeout)
CASLINK_DATABASE_SLOW_QUERY_THRESHOLD=1s (log queries slower than threshold)
CASLINK_DATABASE_LOG_QUERIES=false (log all database queries)

SQLITE SPECIFIC CONFIGURATION:
CASLINK_DATABASE_SQLITE_WAL=true (enable WAL mode for better concurrency)
CASLINK_DATABASE_SQLITE_CACHE_SIZE=64MB (SQLite cache size)
CASLINK_DATABASE_SQLITE_BUSY_TIMEOUT=30s (busy timeout for locked database)

APPLICATION CONFIGURATION:
CASLINK_BRAND_NAME="Casjay URL Shortener" (application branding name)
CASLINK_BASE_URL=auto (auto-detected from Host header, used for absolute URLs)
CASLINK_ENABLE_REGISTRATION=true (allow new user registration)
CASLINK_ENABLE_ANONYMOUS_URLS=true (allow anonymous URL creation)
CASLINK_REQUIRE_LOGIN_FOR_URLS=false (require authentication for URL creation)
CASLINK_DEFAULT_THEME=dark (default UI theme: light|dark|professional)
CASLINK_CUSTOM_CSS_URL="" (optional custom CSS override URL)
CASLINK_FAVICON_URL="" (custom favicon URL)

URL SHORTENING CONFIGURATION:
CASLINK_MIN_RANDOM_LENGTH=6 (minimum length for auto-generated codes)
CASLINK_MAX_RANDOM_LENGTH=8 (maximum length for auto-generated codes)
CASLINK_CUSTOM_CODE_MIN_LENGTH=3 (minimum length for custom codes)
CASLINK_CUSTOM_CODE_MAX_LENGTH=50 (maximum length for custom codes)
CASLINK_ALLOWED_CHARACTERS="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" (character set)
CASLINK_EXCLUDE_SIMILAR_CHARS=true (exclude visually similar characters like 0O, 1lI)
CASLINK_RESERVED_WORDS="api,admin,www,app,help,about,setup,login,register,dashboard" (comma-separated)
CASLINK_MAX_URL_LENGTH=2048 (maximum original URL length)
CASLINK_DEFAULT_EXPIRATION=never (default URL expiration: 1h|24h|7d|30d|never)

ANALYTICS CONFIGURATION:
CASLINK_ENABLE_ANALYTICS=true (enable click tracking and analytics)
CASLINK_ENABLE_GEOLOCATION=true (enable IP geolocation tracking)
CASLINK_GEOIP_DATABASE_PATH="/var/lib/caslink/GeoLite2-City.mmdb" (MaxMind GeoIP database)
CASLINK_ANALYTICS_RETENTION_DAYS=365 (days to retain analytics data, -1 for unlimited)
CASLINK_ANONYMIZE_IPS=true (hash IP addresses for privacy)
CASLINK_TRACK_BOTS=false (include bot traffic in analytics)
CASLINK_REAL_TIME_ANALYTICS=true (enable real-time analytics updates)
CASLINK_ANALYTICS_EXPORT_FORMATS="csv,json,pdf" (supported export formats)

QR CODE CONFIGURATION:
CASLINK_ENABLE_QR_CODES=true (enable QR code generation)
CASLINK_QR_DEFAULT_SIZE=200 (default QR code size in pixels)
CASLINK_QR_MAX_SIZE=1000 (maximum QR code size in pixels)
CASLINK_QR_SUPPORTED_FORMATS="png,svg,pdf" (supported output formats)
CASLINK_QR_DEFAULT_STYLE=square (default style: square|circle|rounded)
CASLINK_QR_ALLOW_CUSTOM_COLORS=true (allow custom foreground/background colors)
CASLINK_QR_ALLOW_LOGOS=true (allow logo embedding in QR codes)
CASLINK_QR_MAX_LOGO_SIZE=50 (maximum logo size in pixels)

BULK OPERATIONS CONFIGURATION:
CASLINK_ENABLE_BULK_OPERATIONS=true (enable CSV import/export)
CASLINK_BULK_MAX_IMPORT_SIZE=10000 (maximum URLs per import operation)
CASLINK_BULK_SUPPORTED_FORMATS="csv,json" (supported bulk formats)
CASLINK_BULK_TIMEOUT=600s (bulk operation timeout)

AUTHENTICATION CONFIGURATION:
CASLINK_SESSION_SECRET=auto (session encryption key, auto-generated if not provided)
CASLINK_SESSION_TIMEOUT=24h (session timeout duration)
CASLINK_SESSION_SECURE=auto (secure cookies, auto-detected based on HTTPS)
CASLINK_PASSWORD_MIN_LENGTH=8 (minimum password length)
CASLINK_PASSWORD_REQUIRE_SPECIAL=false (require special characters in passwords)
CASLINK_API_TOKEN_LENGTH=32 (API token length in bytes)
CASLINK_API_TOKEN_EXPIRATION=never (API token expiration: 1h|24h|7d|30d|90d|never)

TWO-FACTOR AUTHENTICATION:
CASLINK_ENABLE_TOTP=true (enable TOTP/2FA support)
CASLINK_TOTP_ISSUER_NAME="Caslink" (TOTP issuer name in authenticator apps)
CASLINK_ENABLE_WEBAUTHN=true (enable WebAuthn/FIDO2 support)
CASLINK_WEBAUTHN_DISPLAY_NAME="Caslink URL Shortener" (WebAuthn display name)
CASLINK_WEBAUTHN_ID=auto (WebAuthn ID, auto-generated from domain)

OAUTH CONFIGURATION:
CASLINK_ENABLE_OAUTH=false (enable OAuth authentication)
CASLINK_OAUTH_PROVIDER=generic (oauth provider: generic|google|github|microsoft|authelia)
CASLINK_OAUTH_CLIENT_ID="" (OAuth client ID)
CASLINK_OAUTH_CLIENT_SECRET="" (OAuth client secret)
CASLINK_OAUTH_REDIRECT_URL=auto (OAuth redirect URL, auto-generated)
CASLINK_OAUTH_AUTHORIZE_URL="" (OAuth authorization endpoint)
CASLINK_OAUTH_TOKEN_URL="" (OAuth token endpoint)
CASLINK_OAUTH_USERINFO_URL="" (OAuth user info endpoint)
CASLINK_OAUTH_SCOPES="openid,profile,email" (OAuth scopes)

CUSTOM DOMAINS CONFIGURATION:
CASLINK_ENABLE_CUSTOM_DOMAINS=true (enable custom domain support)
CASLINK_DOMAIN_VERIFICATION_METHOD=dns (verification method: dns|file|email)
CASLINK_SSL_AUTO_PROVISION=false (automatic SSL certificate provisioning)
CASLINK_MAX_DOMAINS_PER_USER=unlimited (maximum custom domains per user)

FEDERATION CONFIGURATION:
CASLINK_ENABLE_FEDERATION=true (enable federation with other instances)
CASLINK_FEDERATION_DOMAIN=auto (federation domain, auto-detected)
CASLINK_FEDERATION_PRIVATE_KEY_PATH="/var/lib/caslink/federation.key" (federation private key)
CASLINK_FEDERATION_PUBLIC_KEY_PATH="/var/lib/caslink/federation.pub" (federation public key)
CASLINK_FEDERATION_SYNC_INTERVAL=1h (federation synchronization interval)
CASLINK_FEDERATION_SHARE_PUBLIC_URLS=true (share public URLs with federation)

WEBHOOK CONFIGURATION:
CASLINK_ENABLE_WEBHOOKS=true (enable webhook support)
CASLINK_WEBHOOK_TIMEOUT=10s (webhook delivery timeout)
CASLINK_WEBHOOK_RETRY_ATTEMPTS=3 (webhook retry attempts)
CASLINK_WEBHOOK_RETRY_BACKOFF=exponential (retry backoff strategy)
CASLINK_WEBHOOK_MAX_PAYLOAD_SIZE=1048576 (1MB maximum webhook payload)

BILLING CONFIGURATION (OPTIONAL):
CASLINK_BILLING_ENABLED=false (enable billing system)
CASLINK_BILLING_PROVIDER=none (billing provider: none|stripe|paypal|paddle|lemonsqueezy|manual)
CASLINK_BILLING_STRIPE_SECRET_KEY="" (Stripe secret key)
CASLINK_BILLING_STRIPE_WEBHOOK_SECRET="" (Stripe webhook secret)
CASLINK_BILLING_PAYPAL_CLIENT_ID="" (PayPal client ID)
CASLINK_BILLING_PAYPAL_CLIENT_SECRET="" (PayPal client secret)
CASLINK_BILLING_CURRENCY=USD (billing currency)
CASLINK_BILLING_GRACE_PERIOD_DAYS=7 (grace period for failed payments)
CASLINK_BILLING_TRIAL_PERIOD_DAYS=14 (trial period for new subscriptions)
CASLINK_BILLING_REQUIRE_PAYMENT=false (require payment method for trial)
CASLINK_BILLING_ENFORCE_LIMITS=false (enforce usage limits based on plans)
CASLINK_BILLING_USAGE_WARNING_THRESHOLD=80 (warning threshold percentage)
CASLINK_BILLING_AUTO_SUSPEND=false (automatically suspend on limit exceeded)

RATE LIMITING CONFIGURATION:
CASLINK_RATE_LIMIT_ENABLED=true (enable rate limiting)
CASLINK_RATE_LIMIT_REQUESTS_PER_MINUTE=60 (requests per minute per IP)
CASLINK_RATE_LIMIT_BURST=10 (burst allowance)
CASLINK_RATE_LIMIT_CLEANUP_INTERVAL=60s (rate limit cleanup interval)

SECURITY CONFIGURATION:
CASLINK_ENABLE_HTTPS_REDIRECT=auto (redirect HTTP to HTTPS, auto-detected)
CASLINK_HSTS_MAX_AGE=31536000 (HSTS max age in seconds)
CASLINK_CSP_POLICY="default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'" (Content Security Policy)
CASLINK_ALLOWED_ORIGINS="*" (CORS allowed origins)
CASLINK_MALWARE_DETECTION_ENABLED=false (enable malware URL detection)
CASLINK_MALWARE_API_KEY="" (malware detection API key)

LOGGING CONFIGURATION:
CASLINK_LOG_LEVEL=info (log level: debug|info|warn|error)
CASLINK_LOG_FORMAT=json (log format: json|text)
CASLINK_LOG_FILE="" (log file path, empty for stdout)
CASLINK_LOG_MAX_SIZE=100MB (maximum log file size)
CASLINK_LOG_MAX_BACKUPS=5 (number of log file backups)
CASLINK_LOG_MAX_AGE=30 (maximum age of log files in days)
CASLINK_LOG_COMPRESS=true (compress rotated log files)

MONITORING CONFIGURATION:
CASLINK_ENABLE_METRICS=true (enable Prometheus metrics)
CASLINK_METRICS_PATH=/metrics (metrics endpoint path)
CASLINK_ENABLE_HEALTH_CHECK=true (enable health check endpoint)
CASLINK_HEALTH_CHECK_PATH=/health (health check endpoint path)

EMAIL CONFIGURATION:
CASLINK_EMAIL_ENABLED=false (enable email notifications)
CASLINK_EMAIL_PROVIDER=smtp (email provider: smtp|sendgrid|ses)
CASLINK_EMAIL_FROM_ADDRESS="noreply@localhost" (sender email address)
CASLINK_EMAIL_FROM_NAME="Caslink" (sender display name)
CASLINK_SMTP_HOST="" (SMTP server hostname)
CASLINK_SMTP_PORT=587 (SMTP server port)
CASLINK_SMTP_USERNAME="" (SMTP username)
CASLINK_SMTP_PASSWORD="" (SMTP password)
CASLINK_SMTP_TLS=true (enable SMTP TLS)

CACHE CONFIGURATION:
CASLINK_CACHE_ENABLED=true (enable in-memory caching)
CASLINK_CACHE_TTL=3600s (cache TTL for URL data)
CASLINK_CACHE_CLEANUP_INTERVAL=600s (cache cleanup interval)
CASLINK_REDIS_ENABLED=false (enable Redis for caching and sessions)
CASLINK_REDIS_URL="" (Redis connection URL)

DATABASE SCHEMA DESIGN:
The application uses two separate databases for logical separation and performance optimization.

PRIMARY DATABASE (URLs and Analytics):
Table: urls
- id TEXT PRIMARY KEY (short code, 3-50 characters)
- original_url TEXT NOT NULL (original URL, max 2048 characters)
- is_custom BOOLEAN DEFAULT FALSE (true if user-provided custom code)
- title TEXT (optional page title, max 255 characters)
- description TEXT (optional description, max 500 characters)
- favicon_url TEXT (cached favicon URL)
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
- expires_at DATETIME (NULL for never expires)
- clicks INTEGER DEFAULT 0 (total click count)
- unique_clicks INTEGER DEFAULT 0 (unique IP click count)
- user_id TEXT (NULL for anonymous, foreign key to users.id)
- domain_id TEXT (custom domain, foreign key to domains.id)
- active BOOLEAN DEFAULT TRUE (URL active status)
- password TEXT (bcrypt hash for password protection)
- tags TEXT (comma-separated tags for organization)
- utm_source TEXT (UTM campaign tracking)
- utm_medium TEXT
- utm_campaign TEXT
- utm_term TEXT
- utm_content TEXT

Table: clicks
- id TEXT PRIMARY KEY (UUID)
- url_id TEXT NOT NULL (foreign key to urls.id)
- clicked_at DATETIME DEFAULT CURRENT_TIMESTAMP
- ip_address TEXT (anonymized for privacy)
- ip_hash TEXT (SHA256 hash for unique counting)
- user_agent TEXT (full user agent string)
- parsed_browser TEXT (parsed browser name)
- parsed_os TEXT (parsed operating system)
- parsed_device TEXT (parsed device type)
- referrer TEXT (referring URL)
- referrer_domain TEXT (extracted referring domain)
- country_code TEXT (2-letter ISO country code)
- country_name TEXT (full country name)
- region TEXT (state/province)
- city TEXT (city name)
- latitude REAL (geographic latitude)
- longitude REAL (geographic longitude)
- timezone TEXT (timezone identifier)
- is_bot BOOLEAN DEFAULT FALSE (detected bot traffic)
- is_unique BOOLEAN DEFAULT TRUE (first click from this IP for this URL)

Table: click_daily_stats (aggregated analytics)
- id TEXT PRIMARY KEY (url_id + date combination)
- url_id TEXT NOT NULL (foreign key to urls.id)
- date DATE NOT NULL (statistics date)
- clicks INTEGER DEFAULT 0 (total clicks for the day)
- unique_clicks INTEGER DEFAULT 0 (unique clicks for the day)
- top_countries TEXT (JSON array of top countries)
- top_referrers TEXT (JSON array of top referring domains)
- top_browsers TEXT (JSON array of top browsers)
- top_devices TEXT (JSON array of top device types)
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- UNIQUE constraint on (url_id, date)

Table: uploads (file attachments)
- id TEXT PRIMARY KEY (UUID)
- url_id TEXT NOT NULL (foreign key to urls.id)
- filename TEXT NOT NULL (sanitized original filename)
- mime_type TEXT NOT NULL (detected MIME type)
- size INTEGER NOT NULL (file size in bytes)
- data BLOB NOT NULL (file content)
- uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP

Table: qr_codes (generated QR codes cache)
- id TEXT PRIMARY KEY (UUID)
- url_id TEXT NOT NULL (foreign key to urls.id)
- format TEXT NOT NULL (png, svg, pdf)
- size INTEGER NOT NULL (pixels)
- style TEXT NOT NULL (square, circle, rounded)
- foreground_color TEXT (hex color)
- background_color TEXT (hex color)
- logo_url TEXT (optional logo URL)
- data BLOB NOT NULL (QR code image data)
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP

SECONDARY DATABASE (Users and Configuration):
Table: users
- id TEXT PRIMARY KEY (32-character hex string)
- username TEXT UNIQUE NOT NULL (3-50 characters)
- email TEXT UNIQUE (valid email format, can be NULL)
- password_hash TEXT NOT NULL (bcrypt with cost 12)
- is_admin BOOLEAN DEFAULT FALSE
- is_premium BOOLEAN DEFAULT FALSE
- premium_expires_at DATETIME
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- last_login DATETIME
- last_active DATETIME
- two_fa_secret TEXT DEFAULT '' (TOTP secret)
- two_fa_enabled BOOLEAN DEFAULT FALSE
- webauthn_credentials TEXT DEFAULT '' (JSON array of WebAuthn credentials)
- api_rate_limit INTEGER DEFAULT 1000 (API requests per hour)
- url_limit INTEGER DEFAULT -1 (total URLs allowed, -1 for unlimited)
- timezone TEXT DEFAULT 'UTC'
- language TEXT DEFAULT 'en'
- theme TEXT DEFAULT 'dark'

Table: domains (custom domains)
- id TEXT PRIMARY KEY (32-character hex string)
- domain TEXT UNIQUE NOT NULL (e.g., 'short.ly', 'brand.co')
- user_id TEXT NOT NULL (foreign key to users.id)
- is_default BOOLEAN DEFAULT FALSE
- ssl_enabled BOOLEAN DEFAULT FALSE
- ssl_cert_path TEXT
- ssl_key_path TEXT
- verified BOOLEAN DEFAULT FALSE
- verification_token TEXT
- verification_method TEXT (dns, file, email)
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- verified_at DATETIME

Table: api_tokens
- id TEXT PRIMARY KEY (32-character hex string)
- user_id TEXT NOT NULL (foreign key to users.id)
- name TEXT NOT NULL (token description)
- token TEXT UNIQUE NOT NULL (64-character hex string)
- permissions TEXT NOT NULL (JSON array of permissions)
- rate_limit INTEGER DEFAULT 1000 (requests per hour)
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- expires_at DATETIME
- last_used DATETIME
- last_used_ip TEXT
- active BOOLEAN DEFAULT TRUE

Table: sessions
- id TEXT PRIMARY KEY (session ID)
- user_id TEXT NOT NULL (foreign key to users.id)
- data TEXT NOT NULL (encrypted session data)
- expires_at DATETIME NOT NULL
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP
- ip_address TEXT
- user_agent TEXT

Table: server_config (runtime configuration)
- key TEXT PRIMARY KEY
- value TEXT NOT NULL
- type TEXT NOT NULL (string, integer, boolean, json)
- description TEXT
- updated_by TEXT (user ID who made the change)
- updated_at DATETIME DEFAULT CURRENT_TIMESTAMP

Table: audit_logs (comprehensive audit trail)
- id TEXT PRIMARY KEY (UUID)
- user_id TEXT (foreign key to users.id, NULL for anonymous)
- action TEXT NOT NULL (action performed)
- resource_type TEXT NOT NULL (urls, users, config, etc.)
- resource_id TEXT (ID of affected resource)
- old_values TEXT (JSON of previous values)
- new_values TEXT (JSON of new values)
- ip_address TEXT
- user_agent TEXT
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- success BOOLEAN DEFAULT TRUE
- error_message TEXT

BILLING TABLES (if billing enabled):
Table: billing_plans
- id TEXT PRIMARY KEY
- name TEXT NOT NULL (free, starter, pro, business, enterprise)
- display_name TEXT NOT NULL
- description TEXT
- price_monthly INTEGER NOT NULL (cents, 0 for free)
- price_yearly INTEGER NOT NULL (cents, 0 for free)
- currency TEXT DEFAULT 'USD'
- features TEXT NOT NULL (JSON array of feature names)
- limits TEXT NOT NULL (JSON object with limits)
- trial_days INTEGER DEFAULT 0
- active BOOLEAN DEFAULT TRUE
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP

Table: subscriptions
- id TEXT PRIMARY KEY (32-character hex string)
- user_id TEXT NOT NULL (foreign key to users.id)
- plan_id TEXT NOT NULL (foreign key to billing_plans.id)
- provider_subscription_id TEXT (external provider subscription ID)
- status TEXT NOT NULL (trialing, active, past_due, canceled, unpaid)
- billing_cycle TEXT NOT NULL (monthly, yearly)
- current_period_start DATETIME NOT NULL
- current_period_end DATETIME NOT NULL
- trial_start DATETIME
- trial_end DATETIME
- cancel_at_period_end BOOLEAN DEFAULT FALSE
- canceled_at DATETIME
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP
- updated_at DATETIME DEFAULT CURRENT_TIMESTAMP

Table: usage_records
- id TEXT PRIMARY KEY (32-character hex string)
- user_id TEXT NOT NULL (foreign key to users.id)
- subscription_id TEXT NOT NULL (foreign key to subscriptions.id)
- metric_name TEXT NOT NULL (urls_created, api_requests, qr_codes, etc.)
- quantity INTEGER NOT NULL
- timestamp DATETIME NOT NULL
- billing_period TEXT NOT NULL (YYYY-MM format)
- created_at DATETIME DEFAULT CURRENT_TIMESTAMP

FEDERATION TABLES:
Table: federation_instances
- id TEXT PRIMARY KEY (32-character hex string)
- domain TEXT UNIQUE NOT NULL
- public_key TEXT NOT NULL (RSA public key for signature verification)
- discovered_at DATETIME DEFAULT CURRENT_TIMESTAMP
- last_sync DATETIME
- active BOOLEAN DEFAULT TRUE
- blocked BOOLEAN DEFAULT FALSE
- sync_enabled BOOLEAN DEFAULT TRUE

Table: federated_urls
- id TEXT PRIMARY KEY (UUID)
- original_id TEXT NOT NULL (ID from source instance)
- source_instance TEXT NOT NULL (foreign key to federation_instances.domain)
- original_url TEXT NOT NULL
- short_code TEXT NOT NULL
- title TEXT
- created_at DATETIME NOT NULL
- synced_at DATETIME DEFAULT CURRENT_TIMESTAMP
- UNIQUE constraint on (source_instance, original_id)

DATABASE INDEXES FOR PERFORMANCE:
URLs Database Indexes:
- idx_urls_created_at ON urls(created_at DESC)
- idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL
- idx_urls_user_id ON urls(user_id) WHERE user_id IS NOT NULL
- idx_urls_domain_id ON urls(domain_id) WHERE domain_id IS NOT NULL
- idx_urls_active ON urls(active) WHERE active = TRUE
- idx_urls_tags ON urls(tags) WHERE tags IS NOT NULL AND tags != ''
- idx_clicks_url_id ON clicks(url_id)
- idx_clicks_clicked_at ON clicks(clicked_at DESC)
- idx_clicks_country_code ON clicks(country_code) WHERE country_code IS NOT NULL
- idx_clicks_is_bot ON clicks(is_bot) WHERE is_bot = FALSE
- idx_clicks_ip_hash_url ON clicks(ip_hash, url_id) for unique counting
- idx_click_daily_stats_url_date ON click_daily_stats(url_id, date DESC)
- idx_uploads_url_id ON uploads(url_id)
- idx_qr_codes_url_id ON qr_codes(url_id)

Users Database Indexes:
- idx_users_username ON users(username)
- idx_users_email ON users(email) WHERE email IS NOT NULL
- idx_users_created_at ON users(created_at DESC)
- idx_users_last_login ON users(last_login DESC) WHERE last_login IS NOT NULL
- idx_api_tokens_token ON api_tokens(token) WHERE active = TRUE
- idx_api_tokens_user_id ON api_tokens(user_id) WHERE active = TRUE
- idx_sessions_expires_at ON sessions(expires_at)
- idx_sessions_user_id ON sessions(user_id)
- idx_audit_logs_user_id ON audit_logs(user_id) WHERE user_id IS NOT NULL
- idx_audit_logs_created_at ON audit_logs(created_at DESC)
- idx_audit_logs_resource ON audit_logs(resource_type, resource_id)
- idx_domains_user_id ON domains(user_id)
- idx_domains_verified ON domains(verified) WHERE verified = TRUE

MIGRATION SYSTEM SPECIFICATIONS:
The migration system provides comprehensive database schema management with validation, rollback capabilities, and cross-database compatibility.

Migration Structure:
Each migration is defined as a Go struct implementing the Migration interface with the following requirements:
- Unique ID in format YYYYMMDD_HHMMSS_description
- Human-readable description
- Dependencies array for migration ordering
- Up and Down queries for each supported database type
- Optional validation function for pre-migration checks
- Optional post-migration hooks for data transformation
- Checksum calculation for integrity verification

Migration Validation Rules:
1. Structure Validation: Ensures all required fields are present and valid
2. SQL Syntax Validation: Database-specific syntax checking
3. Dependency Validation: Ensures all dependencies are applied
4. Rollback Capability Validation: Verifies DOWN queries can reverse UP queries
5. Simulation Testing: Dry-run execution in transaction with rollback
6. Custom Validation: Optional business logic validation
7. Conflict Detection: Prevents conflicting schema changes

Rollback Safety Mechanisms:
1. Automatic Backup: Creates database backup before major migrations
2. Transaction Isolation: Each migration runs in separate transaction
3. Rollback Planning: Generates rollback plan with data loss warnings
4. Dependency Tracking: Properly handles dependent migration rollbacks
5. Point-in-time Recovery: Supports rollback to any previous migration
6. Validation Before Rollback: Ensures rollback safety before execution

Migration Execution Process:
1. Load and validate all pending migrations
2. Create execution plan with dependency resolution
3. Perform pre-migration validation and testing
4. Create backup if configured
5. Execute migrations in transaction with timeout
6. Record migration status and timing
7. Handle failures with automatic rollback
8. Update schema version and audit logs

PORT ALLOCATION AND PERSISTENCE SYSTEM:
The application implements automatic port selection and persistence for zero-configuration startup.

Port Selection Algorithm:
1. Check environment variable CASLINK_SERVER_PORT
2. If set to "auto" or empty, proceed with automatic selection
3. If specific port specified, validate availability and use
4. Query database for previously assigned port
5. Test previous port availability
6. If unavailable, select new random port in range 64000-65535
7. Validate new port availability with bind test
8. Retry up to 10 times if port conflicts occur
9. Save selected port to database for future reuse
10. Display clear startup message with access URLs

Port Persistence Schema:
The selected port is stored in the server_config table with key "server_port" for reuse across restarts. Port change history is maintained in port_history table for troubleshooting.

External IP Detection:
The system attempts to detect external IP for remote access display:
1. Check cloud metadata services (AWS, GCP, Azure)
2. Check container environment variables
3. Scan network interfaces for non-loopback addresses
4. Display both localhost and external access URLs

PROXY DETECTION AND HEADER HANDLING:
Comprehensive proxy detection system supporting all major reverse proxies and load balancers.

Supported Proxy Headers (in priority order):
Client IP Detection:
- CF-Connecting-IP (Cloudflare - highest priority)
- True-Client-IP (Cloudflare Enterprise)
- X-Real-IP (nginx reverse proxy)
- X-Client-IP (generic client IP)
- X-Forwarded-For (standard, comma-separated chain)
- X-Cluster-Client-IP (Kubernetes ingress)
- X-Original-Forwarded-For (proxy chains)
- Forwarded (RFC 7239 standard format)

Protocol Detection:
- CF-Visitor (Cloudflare JSON format)
- X-Forwarded-Proto (standard protocol header)
- X-Forwarded-Protocol (alternative naming)
- X-Scheme (simple scheme header)
- Front-End-Https (Microsoft-style)
- X-Url-Scheme (alternative scheme)

Host Detection:
- X-Forwarded-Host (standard host header)
- X-Original-Host (preserved original host)
- X-Host (simple host header)

Port Detection:
- X-Forwarded-Port (forwarded port number)
- X-Original-Port (preserved original port)

Trusted Proxy Networks:
The system maintains configurable trusted proxy networks including:
- Private networks (RFC 1918): 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
- Loopback: 127.0.0.0/8, ::1/128
- Cloudflare IP ranges (automatically updated list)
- AWS ALB IP ranges (subset of AWS ranges)
- User-configured custom networks via CASLINK_SERVER_TRUSTED_PROXIES

Proxy Detection Methods:
1. Environment Variable Analysis: Checks for HTTP_* environment variables
2. Header Presence Detection: Scans for common proxy headers
3. Container Environment Detection: Identifies Kubernetes, Docker, cloud environments
4. Network Configuration Analysis: Examines bind addresses and ports
5. Request Pattern Analysis: Detects proxy-specific request patterns

CLIENT INFORMATION EXTRACTION:
The proxy service extracts comprehensive client information:
- Real client IP (bypassing proxy chain)
- Original protocol (HTTP/HTTPS)
- Original host and port
- Complete proxy chain reconstruction
- Geographic location (if geolocation enabled)
- User agent parsing (browser, OS, device)
- Bot detection and filtering

FIRST-RUN SETUP WIZARD SPECIFICATIONS:
Headless-friendly setup process optimized for server environments.

Setup Flow Design:
1. Application Startup: Display clear terminal instructions with URLs
2. Admin Account Creation: Web-based form for first admin user
3. First URL Creation: Guided tutorial for URL shortening
4. Optional Customization: Branding and basic configuration
5. Completion: Redirect to main dashboard

Terminal Display Format:
The application displays formatted terminal output with:
- Clear ASCII box drawing for visual separation
- Access URLs for both localhost and detected external IP
- Optional terminal QR code for mobile access
- Step-by-step instructions
- Progress indicators and status updates
- Helpful tips and commands

Setup Wizard Pages:
1. Welcome and Admin Creation (/setup/admin)
   - Username input with validation
   - Email input (optional)
   - Password input with strength requirements
   - Automatic login after creation

2. First URL Creation (/setup/first-url)
   - URL input with validation
   - Custom code input (optional)
   - Real-time availability checking
   - Success display with copy functionality

3. Customization (/setup/customize)
   - Brand name configuration
   - Custom domain setup (optional)
   - User registration settings
   - Theme selection

Setup API Endpoints:
- POST /api/v1/setup/admin: Create first admin user
- POST /api/v1/setup/first-url: Create demonstration URL
- POST /api/v1/setup/config: Save basic configuration
- GET /api/v1/setup/status: Check setup completion status

COMPREHENSIVE FEATURE SET IMPLEMENTATION:
All features are available to all users regardless of deployment method.

Core URL Management Features:
1. URL Creation and Shortening
   - Custom short codes (1-50 characters)
   - Auto-generated codes (6-8 characters)
   - URL validation and normalization
   - Malware and phishing detection (optional)
   - Expiration date setting
   - Password protection
   - UTM parameter injection
   - Bulk import/export (CSV, JSON)

2. Analytics and Tracking
   - Real-time click tracking
   - Geographic analytics with city-level precision
   - Device and browser detection
   - Referrer analysis and attribution
   - Bot detection and filtering
   - Time-series analytics with multiple granularities
   - Export capabilities (CSV, JSON, PDF)
   - Conversion funnel analysis
   - Custom event tracking

3. QR Code Generation
   - Multiple formats (PNG, SVG, PDF)
   - Customizable colors and styling
   - Logo embedding capability
   - Batch generation
   - High-resolution output (up to 1000px)
   - Vector format support
   - Print-optimized versions

4. Custom Domains and Branding
   - Unlimited custom domains
   - Domain verification (DNS, file-based)
   - Automatic SSL certificate provisioning
   - Complete white-labeling
   - Custom CSS and theming
   - Favicon customization
   - Email template customization

5. User Management and Authentication
   - User registration and profiles
   - Role-based access control (admin, user)
   - Multi-factor authentication (TOTP, WebAuthn)
   - OAuth integration (Google, GitHub, Microsoft, custom)
   - API token management
   - Session management
   - Password policies

6. Team and Organization Features
   - Organization creation and management
   - Team member invitation and roles
   - Shared URL libraries
   - Permission inheritance
   - Team analytics and reporting
   - Collaborative URL management

7. API and Integration Features
   - Complete REST API with OpenAPI documentation
   - GraphQL API for advanced queries
   - Webhook system with retry logic
   - Rate limiting and quotas
   - SDK libraries for popular languages
   - CLI tool for automation
   - Zapier integration

8. Federation and Sharing
   - Instance discovery via DNS and .well-known
   - Public URL sharing across instances
   - Federated analytics (optional)
   - Cross-instance link validation
   - Distributed short code registry

9. Advanced Analytics Features
   - Real-time dashboard updates
   - Geographic heat maps
   - Time-series visualization
   - Cohort analysis
   - A/B testing support
   - Custom dimension tracking
   - Automated reporting

10. Security and Compliance
    - Comprehensive audit logging
    - GDPR compliance tools
    - Data retention policies
    - IP geoblocking
    - Rate limiting and DDoS protection
    - Malicious URL detection
    - Content Security Policy enforcement

CLI TOOL SPECIFICATIONS:
Complete command-line interface for all operations.

Command Structure:
caslink [global-options] <command> [command-options] [arguments]

Global Options:
--config, -c: Configuration file path
--server, -s: Server URL (default: auto-detect)
--token, -t: API authentication token
--output, -o: Output format (json, table, yaml, csv)
--verbose, -v: Verbose output
--quiet, -q: Quiet mode (errors only)
--no-color: Disable colored output

Primary Commands:

1. Server Management:
   caslink webui: Start web server
   caslink config: Configuration management
   caslink migrate: Database migrations
   caslink backup: Database backup/restore
   caslink health: Health check and status

2. URL Management:
   caslink url create <url>: Create short URL
   caslink url list: List URLs
   caslink url get <id>: Get URL details
   caslink url update <id>: Update URL
   caslink url delete <id>: Delete URL
   caslink url analytics <id>: View analytics

3. User Management:
   caslink user create: Create user account
   caslink user list: List users (admin only)
   caslink user update: Update user profile
   caslink user delete: Delete user account
   caslink user tokens: Manage API tokens

4. Analytics and Reporting:
   caslink analytics summary: Analytics overview
   caslink analytics export: Export analytics data
   caslink analytics real-time: Real-time analytics
   caslink report generate: Generate reports

5. Bulk Operations:
   caslink bulk import <file>: Import URLs from file
   caslink bulk export: Export URLs to file
   caslink bulk status: Check operation status

6. QR Code Operations:
   caslink qr generate <id>: Generate QR code
   caslink qr batch: Batch QR generation
   caslink qr customize: QR customization

7. Administrative Commands:
   caslink admin users: User management
   caslink admin config: Server configuration
   caslink admin billing: Billing management (if enabled)
   caslink admin federation: Federation management

Shell Completion:
Complete shell completion support for:
- Bash: source <(caslink completion bash)
- Zsh: source <(caslink completion zsh)
- Fish: caslink completion fish | source
- PowerShell: caslink completion powershell | Out-String | Invoke-Expression

BILLING SYSTEM SPECIFICATIONS (OPTIONAL):
Comprehensive billing system that is purely optional and never gates features.

Billing Philosophy:
The billing system is designed as a business tool for monetization, not feature control. All features remain available regardless of billing status. Billing only affects usage enforcement and business operations.

Supported Payment Providers:
1. Stripe (recommended for global businesses)
   - Card payments, ACH, SEPA
   - Subscription management
   - Webhook integration
   - Global tax handling

2. PayPal (recommended for small businesses)
   - PayPal and card payments
   - Subscription billing
   - International support

3. Paddle (recommended for SaaS)
   - Global tax compliance
   - Subscription management
   - Revenue recognition

4. LemonSqueezy (modern alternative)
   - Developer-friendly API
   - Global payments
   - Modern checkout experience

5. Manual/Enterprise Billing
   - Invoice generation
   - Manual payment tracking
   - Custom billing cycles

Billing Plan Structure:
Plans are defined with:
- Usage limits (optional enforcement)
- Feature flags (informational only)
- Billing cycles (monthly, yearly)
- Trial periods
- Grace periods for failed payments

Usage Tracking Metrics:
- URLs created per month
- API requests per hour
- QR codes generated
- Analytics queries
- Custom domains count
- Team members count
- Storage usage

Billing Database Tables:
- billing_plans: Plan definitions and pricing
- subscriptions: User subscription records
- usage_records: Usage tracking data
- invoices: Generated invoices
- payments: Payment transaction records

Dunning Management:
- Automatic payment retry with intelligent scheduling
- Progressive email notifications
- Grace period enforcement
- Account suspension and reactivation
- Customer communication templates

SECURITY IMPLEMENTATION:
Comprehensive security measures for production deployment.

Authentication Security:
- bcrypt password hashing with cost 12
- Secure session management with rotation
- API token generation with cryptographic randomness
- Multi-factor authentication support
- WebAuthn for passwordless authentication
- OAuth integration with major providers

Authorization and Access Control:
- Role-based access control (RBAC)
- Resource-level permissions
- API rate limiting per user and token
- IP-based access restrictions
- Audit logging for all actions

Data Protection:
- Input validation and sanitization
- SQL injection prevention with prepared statements
- XSS protection with content security policies
- CSRF protection with tokens
- Data encryption for sensitive fields

Network Security:
- HTTPS enforcement with HSTS
- Secure cookie attributes
- CORS policy configuration
- Reverse proxy header validation
- DDoS protection and rate limiting

Vulnerability Protection:
- Dependency scanning and updates
- Security header enforcement
- Content type validation
- File upload restrictions
- URL validation and malware detection

TESTING SPECIFICATIONS:
Comprehensive testing suite for reliability and correctness.

Unit Tests:
- Package-level tests for all internal packages
- Mock dependencies for isolated testing
- Test coverage minimum 80%
- Benchmark tests for performance-critical code
- Property-based testing for validation logic

Integration Tests:
- Database integration testing across all supported databases
- HTTP API endpoint testing
- Authentication flow testing
- Migration testing with rollback validation
- External service integration testing

End-to-End Tests:
- Complete user workflow testing
- Cross-browser compatibility testing
- Mobile device testing
- Performance testing under load
- Security testing and penetration testing

Test Data and Fixtures:
- Reproducible test data sets
- Database fixtures for consistent testing
- Mock external services
- Test configuration variants

DEPLOYMENT SPECIFICATIONS:
Multiple deployment options for various environments.

Binary Deployment:
- Single static binary with embedded assets
- Cross-platform compilation (Linux, macOS, Windows)
- ARM64 support for modern hardware
- Minimal system requirements
- Zero external dependencies

Container Deployment:
- Optimized Docker images with multi-stage builds
- Alpine Linux base for minimal size
- Non-root user execution
- Health check implementation
- Volume mounting for data persistence

Orchestration Deployment:
- Kubernetes manifests with best practices
- Helm charts for flexible configuration
- Docker Compose for simple setups
- Docker Swarm support
- Auto-scaling configuration

Cloud Platform Deployment:
- AWS deployment guides and CloudFormation templates
- Google Cloud deployment with Cloud Run support
- Azure deployment with Container Instances
- DigitalOcean App Platform support
- Heroku deployment configuration

Package Manager Distribution:
- Debian/Ubuntu APT repository
- Red Hat/CentOS YUM repository
- Homebrew formula for macOS
- Chocolatey package for Windows
- Snap package for universal Linux

MONITORING AND OBSERVABILITY:
Production-ready monitoring and debugging capabilities.

Metrics Collection:
- Prometheus metrics export
- Custom business metrics
- Performance metrics (response times, throughput)
- Error rates and categorization
- Resource utilization metrics

Health Checks:
- Application health endpoint
- Database connectivity check
- External service dependency checks
- Detailed component status
- Graceful degradation indicators

Logging:
- Structured JSON logging
- Configurable log levels
- Request/response logging
- Error logging with stack traces
- Audit trail logging

Alerting:
- Critical error alerting
- Performance threshold alerting
- Capacity planning alerts
- Security event alerts
- Business metric alerts

DOCUMENTATION SPECIFICATIONS:
Comprehensive documentation for all user types.

User Documentation:
- Quick start guide with common scenarios
- Complete configuration reference
- API documentation with examples
- CLI command reference
- Troubleshooting guide

Administrator Documentation:
- Installation and deployment guides
- Security configuration guidelines
- Performance tuning recommendations
- Backup and recovery procedures
- Monitoring setup guides

Developer Documentation:
- Architecture overview and design decisions
- Contribution guidelines and coding standards
- API development guide
- Plugin development (if applicable)
- Testing guidelines

Business Documentation:
- Feature comparison matrices
- Billing system documentation
- SLA and support policies
- Migration guides from other services
- Case studies and use cases

BUILD AND RELEASE PROCESS:
Automated build and release pipeline for consistent deliveries.

Build Pipeline:
- Automated testing on all supported platforms
- Cross-compilation for all target architectures
- Security scanning and vulnerability assessment
- Performance regression testing
- Documentation generation and validation

Release Artifacts:
- Signed binaries for all platforms
- Container images with security scanning
- Package manager packages
- Source code archives
- Checksums and signatures for verification

Versioning Strategy:
- Semantic versioning (MAJOR.MINOR.PATCH)
- Release notes with breaking changes
- Migration guides between versions
- Deprecation notices and timelines
- Long-term support (LTS) releases

Distribution Channels:
- GitHub releases with automated uploads
- Container registries (Docker Hub, GitHub Container Registry)
- Package manager repositories
- Official website downloads
- Mirror networks for global distribution

This comprehensive specification provides every detail necessary to implement a complete, production-ready URL shortener application with enterprise features, optional billing, comprehensive database support, and deployment flexibility. The resulting application will be immediately functional upon compilation, requiring no additional configuration for basic operation while supporting advanced enterprise features for complex deployments.
```

