#!/usr/bin/env bash
# run_tests.sh - Auto-detect and run all tests
# See AI.md PART 29 for testing rules

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

log_info "Running caslink test suite..."

# Detect preferred test environment
if command -v incus &>/dev/null; then
    log_info "Incus available - running full integration tests (PREFERRED)"
    exec "$SCRIPT_DIR/incus.sh" "$@"
elif command -v docker &>/dev/null; then
    log_info "Docker available - running Docker integration tests"
    exec "$SCRIPT_DIR/docker.sh" "$@"
else
    log_error "Neither incus nor docker found. Install one to run integration tests."
    exit 1
fi
