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

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/server cmd/server/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS and wget for health check
RUN apk --no-cache add ca-certificates wget

# Create non-root user and group
RUN addgroup -g 1001 -S threadify && \
    adduser -u 1001 -S threadify -G threadify

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/bin/server .

# Copy config files
COPY --from=builder /app/config/config.docker.yaml ./config/config.yaml

# Change ownership to non-root user
RUN chown -R threadify:threadify /app

# Switch to non-root user
USER threadify

# Expose port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

# Run the application
CMD ["./server"]
