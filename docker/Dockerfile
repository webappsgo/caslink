# =============================================================================
# Build Stage - Compile Go binary
# =============================================================================
FROM casjaysdev/go:latest AS builder

ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID

WORKDIR /build

# Copy go.mod first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -buildvcs=false -trimpath \
    -ldflags "-s -w -X 'main.Version=${VERSION}' -X 'main.CommitID=${COMMIT_ID}' -X 'main.BuildDate=${BUILD_DATE}'" \
    -o /build/binary/caslink ./src

# =============================================================================
# Runtime Stage - Minimal Alpine image
# =============================================================================
FROM alpine:latest

# OCI labels are intentionally NOT baked here — CI applies all
# org.opencontainers.image.* metadata at build time via docker/metadata-action
# (labels:/annotations:), per AI.md PART 27.

# Install required packages including Tor
RUN apk add --no-cache \
    git \
    curl \
    bash \
    tini \
    tor

# Create directories with proper structure
RUN mkdir -p /config /config/security /data/db /data/logs /data/tor /data/backup

# Copy binary from builder stage (multi-stage build)
COPY --from=builder /build/binary/caslink /usr/local/bin/caslink

# Copy BUILD-TIME overlay (entrypoint.sh) from docker/rootfs/ into image
# Note: This is docker/rootfs/ (build context), NOT runtime ./rootfs/ volumes
COPY docker/rootfs/ /

# Copy Dockerfile to image (for reference and backup)
COPY docker/Dockerfile /root/Dockerfile

# Make all binaries/scripts in /usr/local/bin executable (755)
RUN chmod 755 /usr/local/bin/*

# Set environment
# Note: Tor auto-enabled because tor binary is installed above.
# MODE is intentionally unset — the binary defaults to production; only the
# dev/test compose files set MODE (AI.md PART 27).
ENV TZ=America/New_York

# Expose internal port (always 80)
EXPOSE 80

# Stop signal for graceful shutdown
STOPSIGNAL SIGRTMIN+3

# Health check (AI.md PART 27 timing: start 90s, interval 10s, timeout 5s)
HEALTHCHECK --start-period=90s --interval=10s --timeout=5s --retries=3 \
    CMD /usr/local/bin/caslink --status || exit 1

# Use tini as init with signal propagation
# -p SIGTERM: propagate SIGTERM to child processes
ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]
