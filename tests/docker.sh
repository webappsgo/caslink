#!/usr/bin/env bash
# docker.sh - Docker-based integration tests for caslink
# See AI.md PART 29 for testing rules

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_NAME="caslink"
PROJECT_ORG="casapps"
TEST_NETWORK="caslink-test-$$"
TEST_CONTAINER="caslink-test-$$"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

cleanup() {
    log_info "Cleaning up test resources..."
    docker rm -f "$TEST_CONTAINER" 2>/dev/null || true
    docker network rm "$TEST_NETWORK" 2>/dev/null || true
}
trap cleanup EXIT

log_info "Building caslink for testing..."
cd "$PROJECT_DIR"

# Build dev binary
TMPDIR_BUILD="$(mktemp -d -t "${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX" 2>/dev/null || mktemp -d)"
trap "cleanup; rm -rf '$TMPDIR_BUILD'" EXIT

docker run --rm \
    -v "${PROJECT_DIR}:/build" \
    -v "${HOME}/.cache/go-build:/root/.cache/go-build" \
    -v "${HOME}/go/pkg/mod:/go/pkg/mod" \
    -w /build \
    -e CGO_ENABLED=0 \
    golang:alpine \
    go build -o "/build/binaries/${PROJECT_NAME}-test" ./src

log_info "Creating test network: $TEST_NETWORK"
docker network create "$TEST_NETWORK"

log_info "Starting test container..."
docker run --rm \
    --name "$TEST_CONTAINER" \
    --network "$TEST_NETWORK" \
    -v "${PROJECT_DIR}/binaries/${PROJECT_NAME}-test:/usr/local/bin/${PROJECT_NAME}" \
    -e DEBUG=true \
    alpine:latest \
    sh -c "
        apk add --no-cache curl bash file jq &&
        ${PROJECT_NAME} --version &&
        ${PROJECT_NAME} --help &&
        echo 'Tests passed'
    "

log_info "All Docker integration tests passed."
