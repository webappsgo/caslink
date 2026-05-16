#!/usr/bin/env bash
# incus.sh - Incus-based integration tests for caslink (PREFERRED)
# See AI.md PART 29 for testing rules

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_NAME="caslink"
PROJECT_ORG="casapps"
INCUS_NAME="caslink-test-$$"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

if ! command -v incus &>/dev/null; then
    log_error "incus not found. Install Incus or use tests/docker.sh instead."
    exit 1
fi

cleanup() {
    log_info "Cleaning up Incus container: $INCUS_NAME"
    incus delete "$INCUS_NAME" --force 2>/dev/null || true
}
trap cleanup EXIT

log_info "Building caslink for testing..."
cd "$PROJECT_DIR"

docker run --rm \
    -v "${PROJECT_DIR}:/build" \
    -v "${HOME}/.cache/go-build:/root/.cache/go-build" \
    -v "${HOME}/go/pkg/mod:/go/pkg/mod" \
    -w /build \
    -e CGO_ENABLED=0 \
    golang:alpine \
    go build -o "/build/binaries/${PROJECT_NAME}-linux-amd64" ./src

log_info "Creating Incus container: $INCUS_NAME"
incus launch images:debian/12 "$INCUS_NAME"
sleep 5

log_info "Copying binary into container..."
incus file push "${PROJECT_DIR}/binaries/${PROJECT_NAME}-linux-amd64" \
    "${INCUS_NAME}/usr/local/bin/${PROJECT_NAME}"
incus exec "$INCUS_NAME" -- chmod +x "/usr/local/bin/${PROJECT_NAME}"

log_info "Running tests in Incus container..."
incus exec "$INCUS_NAME" -- bash -c "
    set -euo pipefail
    echo 'Testing binary...'
    ${PROJECT_NAME} --version
    ${PROJECT_NAME} --help
    echo 'All Incus tests passed.'
"

log_info "All Incus integration tests passed."
