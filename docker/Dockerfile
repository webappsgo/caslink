# =============================================================================
# Build Stage - Compile Go binary
# =============================================================================
FROM golang:alpine AS builder

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X 'main.Version=${VERSION}' -X 'main.CommitID=${COMMIT_ID}' -X 'main.BuildDate=${BUILD_DATE}'" \
    -o /build/binary/caslink ./src

# =============================================================================
# Runtime Stage - Minimal Alpine image
# =============================================================================
FROM alpine:latest

# ARGs for build-time values (set by docker build --build-arg)
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID
ARG LICENSE=MIT

# Static Labels
LABEL maintainer="casapps <noreply@casapps.pro>" \
      org.opencontainers.image.vendor="casapps" \
      org.opencontainers.image.authors="casapps" \
      org.opencontainers.image.title="caslink" \
      org.opencontainers.image.base.name="caslink" \
      org.opencontainers.image.description="Containerized version of caslink" \
      org.opencontainers.image.url="https://github.com/casapps/caslink" \
      org.opencontainers.image.source="https://github.com/casapps/caslink" \
      org.opencontainers.image.documentation="https://github.com/casapps/caslink" \
      org.opencontainers.image.vcs-type="Git" \
      com.github.containers.toolbox="false"

# Dynamic Labels (from ARGs)
LABEL org.opencontainers.image.licenses="${LICENSE}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.schema-version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT_ID}"

# Install required packages including Tor
RUN apk add --no-cache \
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
# Note: Tor auto-enabled because tor binary is installed above
ENV MODE=development \
    TZ=America/New_York

# Expose internal port (always 80)
EXPOSE 80

# Stop signal for graceful shutdown
STOPSIGNAL SIGRTMIN+3

# Health check (long start period for services that need initialization)
HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3 \
    CMD /usr/local/bin/caslink --status || exit 1

# Use tini as init with signal propagation
# -p SIGTERM: propagate SIGTERM to child processes
ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]
