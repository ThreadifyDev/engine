# Threadify Documentation

## Quick Links
- [What is Threadify?](../WhatIsThreadify.md) - Product overview and core concepts
- [README](../README.md) - Project setup and getting started

## Core Documentation

### Features
- **[Contracts](./CONTRACTS.md)** - Workflow definitions, validation rules, and examples
- **[WebSocket API](./WEBSOCKET.md)** - Real-time communication protocol and client implementation
- **[OTLP Trace Ingestion](./OTLP_INGESTION.md)** - Direct OpenTelemetry trace ingestion and Threadify attributes
- **[JWT Authentication](./JWT_AUTHENTICATION.md)** - Authentication and authorization
- **[Violation Reference](./VIOLATION_SEVERITY_REFERENCE.md)** - All validation types and severities
- **[Architecture](./ARCHITECTURE.md)** - System architecture and design decisions
- **[Archiver Service](./ARCHIVER_SERVICE.md)** - Event archival to Postgres

### Guides
- **[Personal AI Gateway](./PERSONAL_AI_GATEWAY.md)** - Configure Ollama or a custom model gateway, bearer authentication, TLS and client certificates
- **[Docker Setup](./guides/DOCKER.md)** - Container deployment guide
- **[Notification Testing](./guides/NOTIFICATION_TEST_GUIDE.md)** - Testing real-time notifications

### Implementation
Technical documentation for developers working on Threadify internals.

- **[Validation System](./implementation/VALIDATION_SYSTEM.md)** - Real-time validation and notification system
- **[NATS Messaging](./implementation/NATS_MESSAGING.md)** - NATS JetStream integration and WebSocket delivery
- **[Partitioned Streams](./implementation/PARTITIONED_STREAMS_IMPLEMENTATION.md)** - Scalable event archival
- **[Scalability Analysis](./implementation/SCALABILITY_ANALYSIS.md)** - Performance and scaling considerations

## Key Concepts

### Threads
A thread represents a complete business process - every step, participant, error, and decision from start to finish. Think of it as a distributed transaction log for your workflow.

### Steps
Individual actions within a thread. Each step has:
- **Name**: What happened (e.g., "payment_validated")
- **Status**: "success", "failed", or "error"
- **Context**: Business data associated with the step
- **Timestamp**: When it occurred

### Contracts
Optional workflow definitions that enforce:
- Entry points (where threads can start)
- Allowed transitions (valid step sequences)
- Terminal steps (where threads end)
- Timeouts and retry limits

### Notifications
Real-time validation results delivered via WebSocket:
- **`"passed"`**: Step succeeded with no violations
- **`"violated"`**: Step has validation violations
- **`"none"`**: No validation performed (no contract)

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        Client SDKs                          │
│              (JavaScript, Python, Go, etc.)                 │
└─────────────────────────────────────────────────────────────┘
                            ↓ WebSocket
┌─────────────────────────────────────────────────────────────┐
│                    Threadify Server (Go)                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Thread     │  │  Validation  │  │     NATS     │     │
│  │   Service    │→ │   Service    │→ │  Publisher   │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│         ↓                                      ↓            │
│  ┌──────────────┐                    ┌──────────────┐     │
│  │    Valkey    │                    │     NATS     │     │
│  │   (Redis)    │                    │  JetStream   │     │
│  └──────────────┘                    └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
         ↓                                      ↓
┌──────────────────┐                  ┌──────────────────┐
│    Archiver      │                  │   WebSocket      │
│   (Consumer)     │                  │    Consumer      │
└──────────────────┘                  └──────────────────┘
         ↓                                      ↓
┌──────────────────┐                  ┌──────────────────┐
│    Postgres      │                  │     Clients      │
│   (Archive)      │                  │   (Real-time)    │
└──────────────────┘                  └──────────────────┘
```

## Data Flow

### Step Recording
1. Client records step via WebSocket
2. Server validates and stores in Valkey
3. Async validation triggered
4. Notification published to NATS
5. WebSocket delivers to subscribed clients
6. Archiver writes to Postgres

### Validation Flow
1. Go validations (timeout, duration, missing fields)
2. Lua script validations (transitions, retries, terminals)
3. Combine all violations
4. Single notification with all results
5. Publish to NATS with scope routing

## Development

### Running Locally
```bash
# Start dependencies
docker-compose up -d

# Run server
cd threadify-go
go run ./cmd/server

# Run archiver
go run ./cmd/archiver
```

### Testing
```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./...

# Test notifications
open test-notifications.html
```

## Contributing

When adding new features:
1. Update relevant documentation in `/docs`
2. Add tests for new functionality
3. Update `WhatIsThreadify.md` if user-facing
4. Create implementation docs for internal changes

## Support

For questions or issues:
- Check existing documentation
- Review implementation guides
- Open an issue on GitHub
