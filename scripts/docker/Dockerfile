# Production Dockerfile for Caslink
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    gcc \
    musl-dev \
    sqlite-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.version=docker -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ) -s -w" \
    -a -installsuffix cgo \
    -o caslink \
    cmd/main.go

# Build CLI tool
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.version=docker -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ) -s -w" \
    -a -installsuffix cgo \
    -o caslink-cli \
    cmd/caslink-cli/main.go

# Production stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    sqlite

# Create app user
RUN addgroup -g 1001 caslink && \
    adduser -D -s /bin/sh -u 1001 -G caslink caslink

# Create directories
RUN mkdir -p /var/lib/caslink && \
    chown -R caslink:caslink /var/lib/caslink

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/caslink /app/caslink-cli ./

# Copy example configuration
COPY config.toml.example ./config.toml.example

# Switch to app user
USER caslink

# Expose port
EXPOSE 64000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ./caslink-cli health || exit 1

# Default command
CMD ["./caslink", "webui"]