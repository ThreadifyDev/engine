# Architecture Documentation Corrections

**Date**: December 24, 2025  
**Purpose**: Document discrepancies found between documentation and actual implementation

---

## Summary of Findings

After reviewing the actual codebase (`internal/handlers`, `internal/service`, `internal/repository`), I found several discrepancies between the documentation and implementation. This document summarizes what was corrected.

---

## Key Corrections Made

### 1. Thread Storage Layer - MAJOR CORRECTION

**Documentation Said:**
- Hybrid storage: Valkey for fast access, PostgreSQL for persistence
- Threads stored in both systems

**Actual Implementation:**
- **Threads are stored ONLY in Valkey** (ephemeral with TTL)
- PostgreSQL thread repository exists in code but is **NOT USED** in production
- No thread persistence to PostgreSQL in the actual flow

**Why This Matters:**
- Threads are ephemeral (lost after TTL expires)
- No long-term thread history in database
- This is a design decision for performance/simplicity

**Files Affected:**
- `internal/repository/postgres/thread.go` - EXISTS but unused
- `internal/repository/valkey/thread.go` - ACTIVELY USED
- `cmd/server/main.go` - Only initializes Valkey thread repository

---

### 2. Connection Management - In-Memory, Not Valkey

**Documentation Said:**
- WebSocket sessions stored in Valkey
- Session management uses Redis

**Actual Implementation:**
- **ConnectionService** uses in-memory `sync.Map`
- No Valkey storage for connections
- Sessions managed entirely in memory

**Implementation:**
```go
// internal/service/connection.go
type ConnectionService struct {
    clients map[string]*models.ConnectedClient  // In-memory
    mu      sync.RWMutex
}
```

---

### 3. Step Event Processing - More Sophisticated

**Documentation Said:**
- Basic event queue and processing

**Actual Implementation:**
- **Thread-specific queues**: Each thread gets its own queue for sequential processing
- **Global batching**: Events batched for performance (configurable size/timeout)
- **In-memory hash cache**: Last hash per thread cached in memory
- **Worker pool**: 4 workers by default

**Key Features:**
```go
// internal/service/step_event.go
type StepEventService struct {
    threadQueues map[string]chan models.StepEvent  // Per-thread queues
    batch        []models.HashedStepEvent          // Global batch
    lastHashes   map[string]string                 // In-memory cache
    workers      int                               // Worker pool
}
```

---

### 4. Additional Services Not Documented

**Found in Implementation:**

#### AuditEventService (`internal/service/audit.go`)
- Logs invitation token creation/usage
- Logs thread join events
- Stores in Valkey queues with configurable retention
- Async, non-blocking

#### InvitationTokenService (`internal/service/invitation.go`)
- Creates JWT tokens for cross-org threading
- Validates tokens and extracts claims
- Role and permission validation
- Supports partner threading

#### CacheService (`internal/service/cache.go`)
- In-memory first-tier cache
- Caches threads and contract graphs
- Thread-safe with RWMutex
- Reduces Valkey lookups

---

### 5. Contract Graph Storage - Three-Tier Caching

**Documentation Said:**
- Valkey cache, PostgreSQL fallback

**Actual Implementation:**
- **Three-tier caching**:
  1. In-memory (CacheService)
  2. Valkey (with TTL)
  3. PostgreSQL (persistent)

**Flow:**
```
Request → In-memory cache → Valkey cache → PostgreSQL → Cache back up
```

---

## Updated Documentation Files

### 1. COMPLETE_SYSTEM_ARCHITECTURE.md

**Changes Made:**
- ✅ Updated Data Flow Matrix to reflect actual storage patterns
- ✅ Corrected PostgreSQL usage (contracts only, not threads)
- ✅ Added Valkey data patterns (accurate key structures)
- ✅ Documented in-memory services (ConnectionService, CacheService)
- ✅ Added StepEventService details (thread-specific queues, batching)
- ✅ Added AuditEventService, InvitationTokenService, CacheService sections

**Key Corrections:**
- Thread storage: Valkey only (not PostgreSQL)
- Connection management: In-memory (not Valkey)
- Step events: In-memory queues + Valkey batching (not PostgreSQL)
- Audit logging: Valkey queues (not PostgreSQL)

---

## What Was Correct in Documentation

✅ **WebSocket Flow**: Connect → StartThread → RecordEvent → Close  
✅ **Event-Driven Architecture**: Thread model without Context/Steps fields  
✅ **Multi-Tenancy**: Company-based isolation with companyID  
✅ **Server-Driven Auth**: API key → ownerID/companyID derivation  
✅ **Contract Validation**: Optional contracts with runtime validation  
✅ **Cryptographic Pipeline**: SHA-256 hashing for events  

---

## Architecture Summary (Corrected)

### Storage Strategy

| Data Type | Storage | Persistence | TTL |
|-----------|---------|-------------|-----|
| **Threads** | Valkey only | Ephemeral | Configurable (24h default) |
| **Contracts** | PostgreSQL | Persistent | N/A |
| **Contract Graphs** | PostgreSQL + Valkey cache | Persistent + Cache | Configurable |
| **Step Events** | In-memory queues → Valkey | Batched writes | N/A |
| **Connections** | In-memory | Ephemeral | Session lifetime |
| **Audit Events** | Valkey queues | Retention period | Configurable |

### Service Layer (Complete List)

**Core Services:**
- ThreadService - Thread lifecycle management
- ContractService - Contract CRUD operations
- StepEventService - Event processing with batching
- AuthService - JWT and API key authentication

**Supporting Services:**
- CacheService - In-memory caching (threads, graphs)
- ConnectionService - WebSocket connection tracking
- InvitationTokenService - Cross-org threading tokens
- AuditEventService - Audit event logging
- ContractValidationService - Three-tier contract validation

---

## Implications for Product Development

### 1. Thread History
**Current**: Threads are ephemeral (TTL-based)  
**Implication**: No long-term thread history without external persistence  
**Consideration**: May need to add PostgreSQL persistence for compliance/audit use cases

### 2. Scalability
**Current**: In-memory connection management  
**Implication**: Connections lost on server restart  
**Consideration**: May need distributed session management for multi-instance deployment

### 3. Event Processing
**Current**: Sophisticated batching and per-thread queues  
**Implication**: High performance, sequential consistency per thread  
**Benefit**: Good foundation for scale

### 4. Cross-Org Threading
**Current**: Fully implemented with JWT tokens  
**Implication**: Ready for partner integration  
**Benefit**: Network effects moat is technically feasible

---

## Recommendations

### Immediate (No Changes Needed)
- Current architecture is solid for MVP
- In-memory services are appropriate for early scale
- Ephemeral threads reduce complexity

### Short-Term (Consider for Production)
- Add optional PostgreSQL thread persistence (for compliance customers)
- Document thread TTL clearly in SDK/docs
- Add metrics for batch sizes and processing times

### Long-Term (Scale Considerations)
- Distributed session management (Redis-backed ConnectionService)
- Thread archival strategy (cold storage for old threads)
- Multi-region deployment strategy

---

## Files Reviewed

### Handlers
- `internal/handlers/thread.go` - WebSocket message handling
- `internal/handlers/contracts.go` - REST API for contracts

### Services
- `internal/service/thread.go` - Thread lifecycle
- `internal/service/step_event.go` - Event processing
- `internal/service/contract.go` - Contract management
- `internal/service/auth.go` - Authentication
- `internal/service/cache.go` - In-memory caching
- `internal/service/connection.go` - Connection management
- `internal/service/invitation.go` - Token-based invitations
- `internal/service/audit.go` - Audit logging

### Repositories
- `internal/repository/valkey/thread.go` - Valkey thread storage (USED)
- `internal/repository/valkey/contract_graph.go` - Graph caching
- `internal/repository/valkey/client_queue.go` - Client queue operations
- `internal/repository/postgres/thread.go` - PostgreSQL threads (NOT USED)
- `internal/repository/postgres/contract.go` - Contract persistence

### Main
- `cmd/server/main.go` - Service initialization and wiring

---

## Conclusion

The actual implementation is **more sophisticated** than documented in some areas (step event processing, caching) and **simpler** in others (thread storage, connection management).

The architecture is well-suited for the MVP and early growth phases. The main gap is the lack of long-term thread persistence, which may be needed for compliance-heavy customers (fintech, healthcare).

**Overall Assessment**: Solid foundation, documentation now accurate, ready for SDK development and customer onboarding.
