# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.BuildTime=$(date -u '+%Y-%m-%d_%H:%M:%S')" \
    -a -installsuffix cgo \
    -o k8s-gsidecar .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 sidecar && \
    adduser -u 1000 -G sidecar -s /bin/sh -D sidecar

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/k8s-gsidecar .

# Create config directory with proper permissions
RUN mkdir -p /config && chown -R sidecar:sidecar /app /config

# Use non-root user
USER sidecar

# Health check (optional, can be customized based on your app)
# HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
#   CMD ["./k8s-gsidecar", "--health"] || exit 1

ENTRYPOINT ["./k8s-gsidecar"]