# Threadify Bible - Complete WebSocket Flow Documentation

This folder contains detailed, step-by-step documentation of every WebSocket handler case in the ThreadifyEngine system, tracing the complete flow from client request to database persistence.

## Documentation Structure

Each case is documented in a separate file with complete pseudocode flows, database operations, and architectural details.

### WebSocket Handler Cases

1. **[CASE_1_connect.md](./CASE_1_connect.md)** - Authentication & Session Setup
   - JWT validation
   - In-memory connection management
   - WebSocket client registration

2. **[Create or resume a thread](./CASE_2_startThread.md)** - Thread-key resolution and creation (legacy filename retained for links)
   - Contract validation (3-tier cache)
   - Thread initialization
   - Access control setup
   - NATS archival

3. **[CASE_3_recordThreadEvent.md](./CASE_3_recordThreadEvent.md)** - Step Recording
   - Idempotency handling
   - Contract validation
   - Cryptographic hash chain
   - Async validations
   - Real-time notifications

4. **[CASE_4_inviteParty.md](./CASE_4_inviteParty.md)** - Invitation Token Generation
   - Permission validation
   - JWT token creation
   - Role verification

5. **[CASE_5_joinThread.md](./CASE_5_joinThread.md)** - Thread Joining
   - Token validation
   - Access grant
   - Activity logging

6. **[CASE_6_closeConnection.md](./CASE_6_closeConnection.md)** - Connection Cleanup
   - Session cleanup
   - Notification unsubscribe
   - Resource deallocation

7. **[CASE_7_ack_notification.md](./CASE_7_ack_notification.md)** - Notification Acknowledgment
   - Client ACK handling
   - Pending notification cleanup
   - Metrics update

8. **[CASE_8_subscribe.md](./CASE_8_subscribe.md)** - Step-Level Subscription
   - Subscription management
   - Event type filtering
   - Contract-based routing

9. **[CASE_9_unsubscribe.md](./CASE_9_unsubscribe.md)** - Subscription Removal
   - Subscription cleanup
   - Client state update

10. **[CASE_10_addRefs.md](./CASE_10_addRefs.md)** - Add External References
   - Thread-level metadata management
   - External system integration
   - Incremental reference addition

11. **[CASE_11_graphql_data_retrieval.md](./CASE_11_graphql_data_retrieval.md)** - GraphQL Data Retrieval
   - Cache-aside pattern (Valkey → PostgreSQL)
   - Lazy-loaded cryptographic verification
   - Flexible filtering and pagination
   - SDK integration patterns

### System Documentation

12. **[PERMISSIONS_AND_ROLES.md](./PERMISSIONS_AND_ROLES.md)** - Permission and Role Model
   - Two-layer permission system (app_level + runtime_level)
   - Default roles and permissions
   - Notification scope resolution
   - Authorization flow and security
   - RBAC configuration and best practices

## Architecture Overview

### Data Flow Layers

1. **Hot Path (Valkey - Immediate)**: Sub-millisecond operational queries, synchronous writes
2. **Warm Path (NATS JetStream - Async)**: Reliable message delivery with at-least-once guarantees
3. **Cold Path (PostgreSQL - Eventually Consistent)**: Durable archival storage via Archiver service
4. **In-Memory State**: Session management, caching, and WebSocket connections

### Key Architectural Patterns

- **Cache-Aside**: Check cache → Miss → Load from source → Populate cache
- **Write-Through**: Write to primary store → Async write to archive
- **Event Sourcing**: All state changes published as events to NATS
- **CQRS**: Separate write path (Valkey) from read path (Valkey → PostgreSQL)
- **At-Least-Once Delivery**: NATS JetStream with consumer groups and explicit ACK

## Database Impact Summary

### Valkey (Hot Cache)
- Thread metadata: `thread:{id}`, `thread:{id}:meta`
- Thread refs: `thread:{id}:meta` (fields: `refs:{key}`)
- Access control: `thread:{id}:access:{userId}`, `thread:{id}:users`
- Step state: `thread:{id}:steps:{name}:{idemp}`
- Activity log: `thread:{id}:activity` (7-day TTL)
- Contract cache: `contract_graph:{companyID}:{contractName}:v{version}`

### NATS JetStream (Reliable Queue)
- `activity.log` → Activity events
- `metadata.thread` → Thread/refs metadata
- `access.thread` → Access events
- `validations.thread` → Validation results
- `state.step` → Step state snapshots (for archival)
- `notifications.{threadID}.{stepName}` → Real-time notifications

### PostgreSQL (Durable Storage)
- `threads` → Thread metadata
- `thread_refs` → External references
- `thread_access` → Access control records
- `thread_activities` → Complete audit trail
- `thread_validations` → Validation summaries
- `thread_step_states` → Step state snapshots (for fast queries)

## How to Use This Documentation

1. **For Project Assessment**: Read each case file to understand the complete data flow
2. **For Debugging**: Trace a specific action through its case file to identify issues
3. **For Architecture Review**: Study the patterns and database interactions
4. **For Onboarding**: Use as a comprehensive guide to the system's internals

## Related Documentation

- **[ThreadifyArchiver.md](../ThreadifyArchiver.md)** - Archiver service flow from NATS to PostgreSQL
- **[ARCHITECTURE.md](../docs/ARCHITECTURE.md)** - High-level system architecture
- **[NATS_MESSAGING.md](../docs/implementation/NATS_MESSAGING.md)** - NATS messaging patterns

---

*Last Updated: January 16, 2026 - Added GraphQL data retrieval documentation (CASE_11) with cryptographic verification, SDK integration, and performance characteristics*
