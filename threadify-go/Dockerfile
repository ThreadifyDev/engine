# syntax=docker/dockerfile:1

FROM alpine:3.22 AS valkey-builder
RUN apk add --no-cache build-base linux-headers curl
COPY scripts/build-valkey.sh /build-valkey.sh
RUN sh /build-valkey.sh /bundle

# Build on the host architecture and cross-compile for each Buildx target.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_JOBS=2
ARG BUILD_PROCS=2

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files and shared directory (required by replace directive)
COPY go.mod go.sum ./
COPY shared/ ./shared/

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy source code
COPY . .

# CI supplies static dashboard assets before this source build.
RUN test -s internal/dashboard/dist/index.html

# Build the application with optimizations
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOMAXPROCS=$BUILD_PROCS CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -p $BUILD_JOBS \
    -ldflags="-s -w" \
    -trimpath \
    -o bin/threadify ./cmd/server

# The distroless runtime runs as uid 65532 and needs a writable persistent store.
RUN mkdir -p /data/jetstream /data/valkey /data/agent && chown -R 65532:65532 /data

# Final stage - using distroless for minimal size
# Python wheels also need C/C++ shared libraries (for example gRPC).
FROM gcr.io/distroless/cc-debian12:nonroot

WORKDIR /app

# Runtime assets are embedded; only sanitized configuration templates are shipped.
COPY --from=builder /app/bin/threadify /app/threadify
COPY --from=valkey-builder /bundle /app/libexec
COPY --from=builder /app/config/config.selfhost.yaml /app/config/config.yaml
COPY --from=builder --chown=65532:65532 /data /data

ENV CONFIG_PATH=/app/config/config.yaml NATS_STORE_DIR=/data/jetstream REDIS_STORE_DIR=/data/valkey THREADIFY_AGENT_CACHE_DIR=/data/agent
VOLUME ["/data"]

# Expose port
EXPOSE 8081

# Health check using the binary itself (no wget needed)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/threadify", "--healthcheck"]

# Run the application
ENTRYPOINT ["/app/threadify"]
