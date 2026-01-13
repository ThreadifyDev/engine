# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w" \
    -trimpath \
    -o bin/server cmd/server/main.go

# Final stage - using distroless for minimal size
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy the binary and config (distroless already runs as nonroot user)
COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/config/config.docker.yaml /app/config/config.yaml

# Expose port
EXPOSE 8081

# Health check using the binary itself (no wget needed)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/server", "-healthcheck"]

# Run the application
ENTRYPOINT ["/app/server"]
