#!/bin/bash
set -e

# Caslink Docker Build Script
# Builds optimized Docker images for different environments

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DOCKER_DIR="$SCRIPT_DIR/docker"

# Default values
IMAGE_NAME="caslink"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
PUSH_IMAGES="${PUSH_IMAGES:-false}"
REGISTRY="${REGISTRY:-}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
TARGET="${TARGET:-production}"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to show usage
show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Build Docker images for Caslink URL Shortener

OPTIONS:
    -h, --help              Show this help message
    -t, --target TARGET     Build target (production, development) [default: production]
    -v, --version VERSION   Version tag [default: auto-detected from git]
    -r, --registry REGISTRY Docker registry prefix (e.g., docker.io/username)
    -p, --push              Push images to registry after building
    --platforms PLATFORMS   Target platforms for multi-arch build [default: linux/amd64,linux/arm64]
    --single-arch           Build for current architecture only
    --no-cache              Build without cache
    --parallel              Build multiple targets in parallel

EXAMPLES:
    # Build production image
    $0

    # Build development image
    $0 --target development

    # Build and push to registry
    $0 --registry docker.io/myuser --push

    # Build multi-architecture images
    $0 --platforms linux/amd64,linux/arm64

    # Build for single architecture (faster)
    $0 --single-arch

EOF
}

# Function to check dependencies
check_dependencies() {
    local deps=("docker" "git")
    local missing=()

    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            missing+=("$dep")
        fi
    done

    if [ ${#missing[@]} -ne 0 ]; then
        print_error "Missing dependencies: ${missing[*]}"
        print_error "Please install the missing dependencies and try again."
        exit 1
    fi

    # Check Docker version
    if ! docker version &> /dev/null; then
        print_error "Docker is not running or not accessible"
        exit 1
    fi
}

# Function to validate Docker context
validate_docker_context() {
    if [ ! -f "$PROJECT_ROOT/go.mod" ]; then
        print_error "go.mod not found. Are you running from the correct directory?"
        exit 1
    fi

    if [ ! -f "$DOCKER_DIR/Dockerfile.prod" ]; then
        print_error "Dockerfile.prod not found in $DOCKER_DIR"
        exit 1
    fi
}

# Function to build image
build_image() {
    local target="$1"
    local dockerfile="$2"
    local tag_suffix="$3"
    local full_tag=""

    if [ -n "$REGISTRY" ]; then
        full_tag="$REGISTRY/$IMAGE_NAME:$VERSION$tag_suffix"
    else
        full_tag="$IMAGE_NAME:$VERSION$tag_suffix"
    fi

    print_status "Building $target image: $full_tag"

    # Build arguments
    local build_args=(
        "--build-arg" "VERSION=$VERSION"
        "--build-arg" "COMMIT=$COMMIT"
        "--build-arg" "BUILD_DATE=$BUILD_DATE"
        "--target" "$target"
        "--tag" "$full_tag"
        "--file" "$dockerfile"
    )

    # Add cache options
    if [ "$NO_CACHE" = "true" ]; then
        build_args+=("--no-cache")
    fi

    # Add platform options for multi-arch builds
    if [ "$SINGLE_ARCH" != "true" ] && command -v docker buildx &> /dev/null; then
        print_status "Building multi-architecture image for platforms: $PLATFORMS"
        build_args=(
            "buildx" "build"
            "--platform" "$PLATFORMS"
            "${build_args[@]}"
        )

        if [ "$PUSH_IMAGES" = "true" ]; then
            build_args+=("--push")
        else
            build_args+=("--load")
        fi
    else
        build_args=("build" "${build_args[@]}")
    fi

    # Execute build
    print_status "Running: docker ${build_args[*]} $PROJECT_ROOT"
    if docker "${build_args[@]}" "$PROJECT_ROOT"; then
        print_success "Successfully built $target image: $full_tag"

        # Also tag as latest if this is the main version
        if [ "$target" = "production" ] && [ "$VERSION" != "dev" ]; then
            local latest_tag
            if [ -n "$REGISTRY" ]; then
                latest_tag="$REGISTRY/$IMAGE_NAME:latest"
            else
                latest_tag="$IMAGE_NAME:latest"
            fi

            if [ "$SINGLE_ARCH" = "true" ] || [ "$PUSH_IMAGES" != "true" ]; then
                docker tag "$full_tag" "$latest_tag"
                print_success "Tagged as latest: $latest_tag"
            fi
        fi
    else
        print_error "Failed to build $target image"
        return 1
    fi
}

# Function to push images
push_image() {
    local tag="$1"
    print_status "Pushing image: $tag"

    if docker push "$tag"; then
        print_success "Successfully pushed: $tag"
    else
        print_error "Failed to push: $tag"
        return 1
    fi
}

# Function to show build info
show_build_info() {
    print_status "Build Information:"
    echo "  Version: $VERSION"
    echo "  Commit: $COMMIT"
    echo "  Build Date: $BUILD_DATE"
    echo "  Target: $TARGET"
    echo "  Registry: ${REGISTRY:-local}"
    echo "  Platforms: $PLATFORMS"
    echo "  Push: $PUSH_IMAGES"
    echo ""
}

# Function to build all targets
build_all() {
    local dockerfile="$DOCKER_DIR/Dockerfile.prod"
    local success=true

    case "$TARGET" in
        "production")
            build_image "production" "$dockerfile" "" || success=false
            ;;
        "development")
            build_image "development" "$dockerfile" "-dev" || success=false
            ;;
        "all")
            build_image "production" "$dockerfile" "" || success=false
            build_image "development" "$dockerfile" "-dev" || success=false
            ;;
        *)
            print_error "Unknown target: $TARGET"
            print_error "Valid targets: production, development, all"
            exit 1
            ;;
    esac

    if [ "$success" = "false" ]; then
        print_error "One or more builds failed"
        exit 1
    fi

    # Push images if requested and not using buildx (buildx pushes automatically)
    if [ "$PUSH_IMAGES" = "true" ] && [ "$SINGLE_ARCH" = "true" ]; then
        if [ -n "$REGISTRY" ]; then
            case "$TARGET" in
                "production")
                    push_image "$REGISTRY/$IMAGE_NAME:$VERSION"
                    if [ "$VERSION" != "dev" ]; then
                        push_image "$REGISTRY/$IMAGE_NAME:latest"
                    fi
                    ;;
                "development")
                    push_image "$REGISTRY/$IMAGE_NAME:$VERSION-dev"
                    ;;
                "all")
                    push_image "$REGISTRY/$IMAGE_NAME:$VERSION"
                    push_image "$REGISTRY/$IMAGE_NAME:$VERSION-dev"
                    if [ "$VERSION" != "dev" ]; then
                        push_image "$REGISTRY/$IMAGE_NAME:latest"
                    fi
                    ;;
            esac
        else
            print_warning "No registry specified, skipping push"
        fi
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -t|--target)
            TARGET="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -r|--registry)
            REGISTRY="$2"
            shift 2
            ;;
        -p|--push)
            PUSH_IMAGES="true"
            shift
            ;;
        --platforms)
            PLATFORMS="$2"
            shift 2
            ;;
        --single-arch)
            SINGLE_ARCH="true"
            PLATFORMS="linux/$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')"
            shift
            ;;
        --no-cache)
            NO_CACHE="true"
            shift
            ;;
        --parallel)
            PARALLEL="true"
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Main execution
main() {
    print_status "Starting Caslink Docker build process..."

    # Validate environment
    check_dependencies
    validate_docker_context

    # Show build information
    show_build_info

    # Build images
    build_all

    print_success "Docker build process completed successfully!"
    print_status "You can now run the container with:"

    if [ -n "$REGISTRY" ]; then
        echo "  docker run -p 64000:64000 $REGISTRY/$IMAGE_NAME:$VERSION"
    else
        echo "  docker run -p 64000:64000 $IMAGE_NAME:$VERSION"
    fi
}

# Run main function
main "$@"