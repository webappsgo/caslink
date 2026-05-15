#!/usr/bin/env bash
set -e

# =============================================================================
# Container Entrypoint Script
# Handles service startup, signal handling, and graceful shutdown
# =============================================================================

APP_NAME="caslink"
APP_BIN="/usr/local/bin/${APP_NAME}"

# Container defaults (exported for app to use)
# Timezone - default to America/New_York
export TZ="${TZ:-America/New_York}"

# Configurable paths (via env vars or CLI flags)
export CONFIG_DIR="/config"
export DATA_DIR="/data"
export LOG_DIR="/data/logs"
export DATABASE_DIR="/data/db"
export BACKUP_DIR="/data/backup"

# Fixed subdirectories (always under DATA_DIR, not exported)
TOR_DATA_DIR="${DATA_DIR}/tor"

# Tor auto-detection: if tor binary is installed, Tor is enabled
# Docker image always installs tor, so always enabled in containers
TOR_INSTALLED=$(command -v tor >/dev/null 2>&1 && echo "true" || echo "false")

# Array to track background PIDs
declare -a PIDS=()

# -----------------------------------------------------------------------------
# Logging
# -----------------------------------------------------------------------------
log() {
    echo "[entrypoint] $(date '+%Y-%m-%d %H:%M:%S') $*"
}

log_error() {
    echo "[entrypoint] $(date '+%Y-%m-%d %H:%M:%S') ERROR: $*" >&2
}

# Check if value is truthy (case-insensitive)
# Usage: if is_truthy "$DEBUG"; then ...
is_truthy() {
    local val="${1:-false}"
    val="${val,,}"  # lowercase
    [[ "$val" =~ ^(1|y|t|yes|true|on|ok|enable|enabled|sure|yep|yup|yeah|aye|si|oui|da|hai|affirmative|accept|allow|totally)$ ]]
}

# -----------------------------------------------------------------------------
# Signal handling
# -----------------------------------------------------------------------------
cleanup() {
    log "Received shutdown signal, stopping services..."

    # Stop services in reverse order
    for ((i=${#PIDS[@]}-1; i>=0; i--)); do
        pid="${PIDS[i]}"
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping PID $pid..."
            kill -TERM "$pid" 2>/dev/null || true
        fi
    done

    # Wait for processes to exit (max 30 seconds)
    local timeout=30
    while [ $timeout -gt 0 ]; do
        local running=0
        for pid in "${PIDS[@]}"; do
            if kill -0 "$pid" 2>/dev/null; then
                running=1
                break
            fi
        done
        [ $running -eq 0 ] && break
        sleep 1
        ((timeout--))
    done

    # Force kill any remaining
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            log "Force killing PID $pid..."
            kill -9 "$pid" 2>/dev/null || true
        fi
    done

    log "Shutdown complete"
    exit 0
}

# Trap signals for graceful shutdown
# SIGRTMIN+3 (37) is the Docker STOPSIGNAL
# SIGTERM is propagated by tini -p SIGTERM
trap cleanup SIGTERM SIGINT SIGQUIT
trap cleanup SIGRTMIN+3 2>/dev/null || trap cleanup 37

# -----------------------------------------------------------------------------
# Directory setup
# -----------------------------------------------------------------------------
# Container directory structure:
#   $CONFIG_DIR          - configuration files (mounted: ./rootfs/config)
#   $CONFIG_DIR/security - TLS certs, keys
#   $DATA_DIR            - all persistent data (mounted: ./rootfs/data)
#   $DATABASE_DIR        - SQLite databases (changeable, defaults to $DATA_DIR/db)
#   $LOG_DIR             - application and service logs
#   $TOR_DATA_DIR        - Tor hidden service data (fixed: $DATA_DIR/tor)
#   $BACKUP_DIR          - backup files (changeable, defaults to $DATA_DIR/backup)
# -----------------------------------------------------------------------------
setup_directories() {
    log "Setting up directories..."
    mkdir -p "$CONFIG_DIR" "$CONFIG_DIR/security" \
             "$DATABASE_DIR" "$LOG_DIR" "$TOR_DATA_DIR" "$BACKUP_DIR"

    # Fix permissions for Tor (runs as tor user)
    if [ "$TOR_INSTALLED" = "true" ]; then
        chown -R tor:tor "$TOR_DATA_DIR"
        chmod 700 "$TOR_DATA_DIR"
    fi
}

# -----------------------------------------------------------------------------
# Start Tor (auto-detected: if tor binary installed, it's enabled)
# -----------------------------------------------------------------------------
start_tor() {
    if [ "$TOR_INSTALLED" != "true" ]; then
        log "Tor not installed, skipping..."
        return 0
    fi

    log "Starting Tor hidden service..."

    # Create torrc if not exists
    if [ ! -f "$CONFIG_DIR/torrc" ]; then
        cat > "$CONFIG_DIR/torrc" <<EOF
DataDirectory ${TOR_DATA_DIR}
HiddenServiceDir ${TOR_DATA_DIR}/hidden_service
HiddenServicePort 80 127.0.0.1:80
Log notice file ${LOG_DIR}/tor.log
EOF
    fi

    # Start Tor in background
    tor -f "$CONFIG_DIR/torrc" &
    PIDS+=($!)
    log "Tor started (PID: ${PIDS[-1]})"

    # Wait for .onion address
    local timeout=60
    while [ $timeout -gt 0 ]; do
        if [ -f "${TOR_DATA_DIR}/hidden_service/hostname" ]; then
            local onion_addr
            onion_addr=$(cat "${TOR_DATA_DIR}/hidden_service/hostname")
            log "Tor hidden service: ${onion_addr}"
            break
        fi
        sleep 1
        ((timeout--))
    done
}

# -----------------------------------------------------------------------------
# Start main application
# -----------------------------------------------------------------------------
start_app() {
    log "Starting ${APP_NAME}..."

    # Container defaults: 0.0.0.0:80 (override with ADDRESS/PORT env vars)
    local listen_addr="${ADDRESS:-0.0.0.0}"
    local listen_port="${PORT:-80}"
    local debug_flag=""

    # Enable debug mode if DEBUG is truthy (see Boolean Values table)
    if is_truthy "$DEBUG"; then
        debug_flag="--debug"
        log "Debug mode enabled"
    fi

    # Run the main application with container directory paths
    # Uses exported env vars that match volume mounts in docker-compose.yml
    # App can also read DATABASE_DIR, BACKUP_DIR env vars directly
    "$APP_BIN" \
        --address "$listen_addr" \
        --port "$listen_port" \
        --config "$CONFIG_DIR" \
        --data "$DATA_DIR" \
        --log "$LOG_DIR" \
        --pid "$DATA_DIR/${APP_NAME}.pid" \
        $debug_flag \
        "$@" &
    PIDS+=($!)
    log "${APP_NAME} started on ${listen_addr}:${listen_port} (PID: ${PIDS[-1]})"
}

# -----------------------------------------------------------------------------
# Wait for services
# -----------------------------------------------------------------------------
wait_for_services() {
    log "All services started, waiting..."

    # Wait for any process to exit
    while true; do
        for pid in "${PIDS[@]}"; do
            if ! kill -0 "$pid" 2>/dev/null; then
                log_error "Process $pid exited unexpectedly"
                cleanup
            fi
        done
        sleep 5
    done
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------
main() {
    log "Container starting..."
    log "MODE: ${MODE:-development}"
    log "DEBUG: ${DEBUG:-false}"
    log "TZ: ${TZ:-America/New_York}"
    log "ADDRESS: ${ADDRESS:-0.0.0.0}"
    log "PORT: ${PORT:-80}"
    log "TOR_INSTALLED: ${TOR_INSTALLED}"

    setup_directories
    start_tor
    start_app "$@"
    wait_for_services
}

main "$@"
