#!/bin/sh
set -e

# Ensure we have netcat for service checks
if ! command -v nc >/dev/null 2>&1; then
    echo "Warning: netcat not available, service checks will be skipped"
    NC_AVAILABLE=false
else
    NC_AVAILABLE=true
fi

# Function to log messages
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Function to wait for a service to be ready
wait_for_service() {
    local host=$1
    local port=$2
    local service=$3
    local max_attempts=30
    local attempt=1

    log "Waiting for $service to be ready at $host:$port..."

    while [ $attempt -le $max_attempts ]; do
        if [ "$NC_AVAILABLE" = "true" ]; then
            if nc -z "$host" "$port" 2>/dev/null; then
                log "$service is ready!"
                return 0
            fi
        else
            log "Netcat not available, skipping $service connectivity check"
            return 0
        fi

        log "Attempt $attempt/$max_attempts: $service not ready yet, waiting..."
        sleep 2
        attempt=$((attempt + 1))
    done

    log "ERROR: $service failed to become ready after $max_attempts attempts"
    return 1
}

# Function to setup database
setup_database() {
    local db_type=${CASLINK_DATABASE_TYPE:-sqlite}

    case $db_type in
        postgresql)
            local db_host=${CASLINK_DATABASE_HOST:-postgres}
            local db_port=${CASLINK_DATABASE_PORT:-5432}
            wait_for_service "$db_host" "$db_port" "PostgreSQL"
            ;;
        mysql|mariadb)
            local db_host=${CASLINK_DATABASE_HOST:-mysql}
            local db_port=${CASLINK_DATABASE_PORT:-3306}
            wait_for_service "$db_host" "$db_port" "MySQL/MariaDB"
            ;;
        sqlserver)
            local db_host=${CASLINK_DATABASE_HOST:-sqlserver}
            local db_port=${CASLINK_DATABASE_PORT:-1433}
            wait_for_service "$db_host" "$db_port" "SQL Server"
            ;;
        sqlite)
            log "Using SQLite database"
            # Ensure data directory exists
            mkdir -p /var/lib/caslink
            ;;
    esac
}

# Function to setup Redis if enabled
setup_redis() {
    if [ "${CASLINK_REDIS_ENABLED:-false}" = "true" ]; then
        local redis_url=${CASLINK_REDIS_URL:-redis://redis:6379}
        local redis_host=$(echo "$redis_url" | sed -n 's|redis://\([^:]*\).*|\1|p')
        local redis_port=$(echo "$redis_url" | sed -n 's|redis://[^:]*:\([0-9]*\).*|\1|p')
        redis_port=${redis_port:-6379}

        wait_for_service "$redis_host" "$redis_port" "Redis"
    fi
}

# Function to run migrations
run_migrations() {
    if [ "${CASLINK_DATABASE_AUTO_MIGRATE:-true}" = "true" ]; then
        log "Running database migrations..."
        if ./caslink migrate up; then
            log "Database migrations completed successfully"
        else
            log "ERROR: Database migrations failed"
            exit 1
        fi
    else
        log "Auto-migration disabled, skipping..."
    fi
}

# Function to generate federation keys if they don't exist
setup_federation() {
    if [ "${CASLINK_FEDERATION_ENABLED:-true}" = "true" ]; then
        local key_dir="/var/lib/caslink"
        local private_key="$key_dir/federation.key"
        local public_key="$key_dir/federation.pub"

        if [ ! -f "$private_key" ] || [ ! -f "$public_key" ]; then
            log "Generating federation keys..."
            # Generate RSA key pair for federation
            openssl genrsa -out "$private_key" 2048 2>/dev/null
            openssl rsa -in "$private_key" -pubout -out "$public_key" 2>/dev/null
            log "Federation keys generated"
        fi
    fi
}

# Function to create default admin user if needed
setup_admin() {
    # This will be handled by the first-run setup wizard
    log "Admin setup will be handled by the web interface"
}

# Main entrypoint logic
main() {
    log "Starting Caslink container initialization..."

    # Setup dependencies
    setup_database
    setup_redis

    # Run migrations
    run_migrations

    # Setup federation
    setup_federation

    # Setup admin (if needed)
    setup_admin

    log "Container initialization completed"
    log "Starting Caslink server..."

    # Execute the main command
    exec "$@"
}

# Run main function with all arguments
main "$@"