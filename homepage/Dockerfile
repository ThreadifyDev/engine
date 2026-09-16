# syntax=docker/dockerfile:1

# The Remix build output is platform-independent, so build it natively rather
# than emulating each target architecture.
FROM --platform=$BUILDPLATFORM node:24-alpine AS builder

# Set working directory
WORKDIR /app

# Sources are resolved from the homepage build context.
COPY package*.json ./

# Install dependencies
RUN --mount=type=cache,target=/root/.npm npm ci --include=dev

# Copy source code from homepage
COPY . ./

# Build the Remix application (Registry credentials are supplied only at runtime)
RUN npm run build

# Production stage
FROM node:24-alpine

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create app user
RUN addgroup -g 1001 -S nodejs && \
    adduser -S remix -u 1001

WORKDIR /app

# Copy package files and install production dependencies only
COPY package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=dev

# Copy built application from builder
COPY --from=builder --chown=remix:nodejs /app/build ./build

# Switch to non-root user
USER remix

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000', (r) => {process.exit(r.statusCode === 200 ? 0 : 1)})"

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--"]

# Run the application
CMD ["npm", "start"]
