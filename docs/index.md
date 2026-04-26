# Caslink Documentation

Welcome to the Caslink documentation!

## What is Caslink?

Caslink is a secure, mobile-first, feature-rich, fully self-hosted URL shortener application written in Go that compiles into a single static binary with zero external dependencies.

## Features

- **Universal Feature Parity** - All users get all features, regardless of deployment type
- **Zero-Config Startup** - Works immediately with intelligent defaults
- **Single Static Binary** - No external dependencies, just one executable
- **Database Agnostic** - SQLite, PostgreSQL, MySQL, SQL Server support
- **Advanced Analytics** - Real-time click tracking with geolocation
- **QR Code Generation** - Customizable QR codes in PNG, SVG, PDF formats
- **API Documentation** - Swagger and GraphQL included
- **Admin Panel** - Full-featured admin interface
- **Production Ready** - Migrations, audit trails, backups

## Quick Links

- [Installation Guide](installation.md)
- [Configuration Reference](configuration.md)
- [API Documentation](api.md)
- [Admin Guide](admin.md)
- [Development Guide](development.md)

## Getting Started

```bash
# Using Docker
docker run -d -p 64580:80 casapps/caslink:latest

# Using binary
./caslink
```

Access the setup wizard at `http://localhost:64580/setup`
