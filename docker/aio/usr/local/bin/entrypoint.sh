#!/usr/bin/env bash
set -e

# =============================================================================
# AIO Container Entrypoint Script
# Handles PostgreSQL initialization, then launches supervisord which manages
# all services: postgresql + valkey + tor (optional) + app
# =============================================================================

APP_NAME="caslink"

# Set timezone
if [ -n "$TZ" ]; then
    ln -snf "/usr/share/zoneinfo/$TZ" /etc/localtime
    echo "$TZ" > /etc/timezone
fi

log() {
    echo "[entrypoint] $(date '+%Y-%m-%d %H:%M:%S') $*"
}

log_error() {
    echo "[entrypoint] $(date '+%Y-%m-%d %H:%M:%S') ERROR: $*" >&2
}

# -----------------------------------------------------------------------------
# Setup directories for external services (PostgreSQL, Valkey)
# NOTE: App directories (config, data, sqlite, logs) are created by server binary
# External services need special ownership that binary cannot set
# -----------------------------------------------------------------------------
setup_directories() {
    log "Setting up service directories..."
    mkdir -p \
        /data/db/postgres \
        /data/db/valkey \
        /data/db/sqlite \
        /data/log/postgres \
        /run/postgresql \
        /run/valkey

    chown -R postgres:postgres /data/db/postgres /data/log/postgres /run/postgresql
    chmod 700 /data/db/postgres
    chmod 755 /run/valkey
}

# -----------------------------------------------------------------------------
# Initialize PostgreSQL (first-run only)
# -----------------------------------------------------------------------------
init_postgres() {
    if [ -f /data/db/postgres/PG_VERSION ]; then
        log "PostgreSQL already initialized, skipping..."
        return 0
    fi

    log "Initializing PostgreSQL database..."
    su - postgres -c "initdb -D /data/db/postgres"

    # Copy optimized config
    cp /config/postgres/postgresql.conf /data/db/postgres/postgresql.conf

    # Start PostgreSQL temporarily to create database and user
    su - postgres -c "pg_ctl -D /data/db/postgres -l /data/log/postgres/init.log start"

    # Wait for PostgreSQL to be ready
    local timeout=30
    while [ $timeout -gt 0 ]; do
        if su - postgres -c "pg_isready -q" 2>/dev/null; then
            break
        fi
        sleep 1
        ((timeout--))
    done

    if [ $timeout -eq 0 ]; then
        log_error "PostgreSQL did not start in time during init"
        exit 1
    fi

    # Create application database and user
    local db_user="${DB_USER:-caslink}"
    local db_name="${DB_NAME:-caslink}"
    local db_pass="${DB_PASSWORD:-caslink}"

    su - postgres -c "psql -c \"CREATE USER ${db_user} WITH PASSWORD '${db_pass}';\""
    su - postgres -c "psql -c \"CREATE DATABASE ${db_name} OWNER ${db_user};\""
    su - postgres -c "psql -c \"GRANT ALL PRIVILEGES ON DATABASE ${db_name} TO ${db_user};\""

    # Stop PostgreSQL (supervisor will restart it)
    su - postgres -c "pg_ctl -D /data/db/postgres stop"

    log "PostgreSQL initialization complete"
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------
log "Container starting (AIO mode)..."
log "MODE: ${MODE:-production}"
log "DEBUG: ${DEBUG:-false}"
log "TZ: ${TZ:-America/New_York}"
log "PORT: ${PORT:-80}"
log "TOR_ENABLED: ${TOR_ENABLED:-false}"

setup_directories
init_postgres

# Export TOR_ENABLED for supervisord %(ENV_TOR_ENABLED)s interpolation
export TOR_ENABLED="${TOR_ENABLED:-false}"

log "Starting supervisord (manages postgresql + valkey + tor + ${APP_NAME})..."
exec /usr/bin/supervisord -c /etc/supervisor/supervisord.conf
