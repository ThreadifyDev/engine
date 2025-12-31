 # Thread.create Backend Implementation Guide

## Overview

This document explains the implementation of the Thread.create backend for Threadify, a distributed workflow observability platform. The implementation follows Test-Driven Development (TDD) and clean architecture principles.

## Architecture Overview

Threadify uses an event-sourcing architecture where:
- **Threads** are minimal metadata objects
- **State** is derived from events via async projection
- **Valkey (Redis)** provides hot storage with 24hr TTL
- **PostgreSQL** provides cold/durable storage
- **WebSocket** enables real-time SDK communication

## Project Structure

```
threadify-go/
├── internal/
│   ├── domain/              # Domain entities (business objects)
│   │   ├── contract_graph.go    # NEW: Graph structures
│   │   └── thread.go            # NEW: Thread entity
│   ├── service/             # Business logic layer
│   │   ├── contract.go          # EXISTING: Contract CRUD
│   │   ├── contract_graph.go    # NEW: Graph building
│   │   └── thread.go            # NEW: Thread management
│   ├── repository/          # Data access layer
│   │   ├── valkey/
│   │   │   ├── contract.go      # MODIFY: Add cache methods
│   │   │   └── thread.go        # NEW: Thread hot storage
│   │   └── postgres/
│   │       ├── contract_version.go # MODIFY: Add graph column
│   │       └── thread.go        # NEW: Thread cold storage
│   ├── transport/           # API/WebSocket handlers
│   │   └── websocket/
│   │       └── thread.go        # NEW: HandleStartThread
│   └── models/              # Data transfer objects
│       └── contract.go          # MODIFY: Add Graph field
└── cmd/server/main.go       # MODIFY: Wire dependencies

```

## Implementation Tasks

### ✅ Task 1: Add Graph Field to ContractVersion (COMPLETED)

**What was done:**
- Added `Graph json.RawMessage` field to `ContractVersion` struct in `/internal/models/contract.go`
- Created tests in `/internal/models/contract_test.go` to verify:
  - Serialization with graph field
  - Nil graph is valid (for backwards compatibility)
  - Deserialization works correctly

**Why json.RawMessage?**
- Allows storing arbitrary JSON without parsing
- Efficient for pass-through scenarios
- Can be unmarshaled later when needed

### Task 2: Contract Graph Building Logic (IN PROGRESS)

**Purpose:** Convert contract YAML/JSON into a Directed Acyclic Graph (DAG) for efficient validation and navigation.

**Key Concepts:**

1. **Graph Structure:**
   - **Nodes**: Each step or parallel group becomes a node
   - **Dependencies**: `depends_on` defines prerequisites
   - **Next**: Reverse lookup - which steps depend on this one
   - **Final Step**: The last step with no dependents

2. **Node Types:**
   - `step`: Individual workflow step
   - `parallel_group`: Multiple steps that can run in parallel

3. **Parallel Groups:**
   - `all_of` mode: All steps must succeed
   - `any_of` mode: Any step success is sufficient

**Example Contract:**
```yaml
contract_name: payment_processing
version: 1
steps:
  - id: payment_initiated
    owner: merchant
  
  - id: fraud_check
    owner: payment_processor
    depends_on: payment_initiated
    timeout: 2s
  
  - id: bank_authorization
    owner: bank
    depends_on: fraud_check
    timeout: 5s
```

**Resulting Graph:**
```json
{
  "contract_id": "payment_processing",
  "version": 1,
  "graph": {
    "nodes": {
      "payment_initiated": {
        "id": "payment_initiated",
        "type": "step",
        "depends_on": [],
        "next": ["fraud_check"]
      },
      "fraud_check": {
        "id": "fraud_check",
        "type": "step",
        "depends_on": ["payment_initiated"],
        "next": ["bank_authorization"]
      },
      "bank_authorization": {
        "id": "bank_authorization",
        "type": "step",
        "depends_on": ["fraud_check"],
        "next": []
      }
    },
    "final_step": "bank_authorization"
  }
}
```

**Files to create:**
- `/internal/domain/contract_graph.go` ✅ (Created)
- `/internal/service/contract_graph.go` (Graph builder logic)
- `/internal/service/contract_graph_test.go` (Comprehensive tests)

### Task 3: Update Postgres Repository

**Database Migration:**
```sql
ALTER TABLE contract_versions 
ADD COLUMN graph JSONB;

CREATE INDEX idx_contract_versions_graph 
ON contract_versions USING GIN (graph) 
WHERE graph IS NOT NULL;
```

**Why JSONB?**
- Efficient storage and querying
- Can index specific fields if needed
- Native PostgreSQL type for JSON

### Task 4: Hook Graph Generation into ContractService

When a contract version is created/updated, automatically:
1. Parse the contract content (YAML/JSON)
2. Build the graph using GraphBuilder
3. Serialize graph to JSON
4. Store in `graph` field

### Task 5: Thread Domain Entity

**Thread Structure:**
```go
type Thread struct {
    ThreadID        string    // UUID
    OwnerID         string    // From API key
    ContractName    string    // Optional
    ContractVersion int       // 0 if no contract
    ServiceName     string    // Initiating service
    Creator         string    // Who created it
    IsCompleted     bool
    IsCancelled     bool
    CreatedAt       time.Time
    LastEventAt     time.Time
    EventCount      int
    Metadata        map[string]interface{}
    State           ThreadState
}

type ThreadState struct {
    CompletedSteps []string
    ActiveSteps    []string
    FailedSteps    []string
    ViolatedSteps  []string
    WaitingSteps   []string
}
```

**Key Points:**
- Thread can exist without a contract (ad-hoc workflows)
- State arrays track workflow progress
- Metadata allows arbitrary context

### Task 6: Valkey Thread Repository

**Storage Pattern:**
```
Key: thread:metadata:{thread_id}
Type: Hash
TTL: 24 hours

Key: thread:state:{thread_id}
Type: Hash
TTL: 24 hours
```

**Why separate keys?**
- Metadata changes infrequently
- State changes frequently (on every event)
- Separate keys = more efficient updates

### Task 7: Contract Graph Cache in Valkey

**Storage Pattern:**
```
Key: contract@{owner_id}:{contract_name}:{version}
Type: String (JSON)
TTL: 24 hours (refreshed on use)
```

**Cache Strategy:**
1. Check cache on thread creation
2. If miss: Load from Postgres → Cache
3. If hit: Refresh TTL (active contract)

**Why cache?**
- Contract graphs are read-heavy
- Same contract used by many threads
- Reduces Postgres load significantly

### Task 8: Postgres Thread Repository

**Schema:**
```sql
CREATE TABLE threads (
    thread_id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    contract_name TEXT,
    contract_version INTEGER DEFAULT 0,
    service_name TEXT NOT NULL,
    creator TEXT NOT NULL,
    is_completed BOOLEAN DEFAULT FALSE,
    is_cancelled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    last_event_at TIMESTAMP NOT NULL,
    event_count INTEGER DEFAULT 0,
    metadata JSONB,
    completed_steps TEXT[],
    active_steps TEXT[],
    failed_steps TEXT[],
    violated_steps TEXT[],
    waiting_steps TEXT[]
);
```

**Indexes:**
- `owner_id`: Query threads by owner
- `contract_name, contract_version`: Query by contract
- `created_at DESC`: Recent threads first
- `is_completed, created_at DESC`: Active threads

### Task 9: Thread Service (Business Logic)

**CreateThread Flow:**
```
1. Validate input (owner_id, service_name required)
2. Determine contract version (explicit or latest)
3. Ensure contract graph is cached
   - Check Valkey cache
   - If miss: Load from Postgres → Cache
   - If hit: Refresh TTL
4. Create Thread entity with UUID
5. Save to Valkey (hot storage, 24hr TTL)
6. Save to Postgres (cold storage, durable)
7. Return Thread to caller
```

**Error Handling:**
- Valkey write failure → Return error (critical)
- Postgres write failure → Log but continue (eventual consistency)

### Task 10: WebSocket Handler

**Message Format (from SDK):**
```json
{
  "message_id": "msg_abc123",
  "action": "start_thread",
  "payload": {
    "contract_name": "payment-flow-v1",
    "contract_version": 1,
    "service_name": "merchant-service",
    "metadata": { "order_id": "12345" }
  }
}
```

**Response Format:**
```json
{
  "message_id": "msg_abc123",
  "success": true,
  "data": {
    "thread_id": "thread_abc123",
    "contract_name": "payment-flow-v1",
    "contract_version": 1,
    "created_at": "2024-12-13T10:00:00Z"
  }
}
```

**Handler Responsibilities:**
1. Extract API key from WebSocket connection
2. Parse payload
3. Call ThreadService.CreateThread
4. Send success/error response

### Task 11: Wire into main.go

**Dependency Injection:**
```go
// Repositories
valkeyThreadRepo := valkey.NewThreadRepository(valkeyClient)
valkeyContractRepo := valkey.NewContractRepository(valkeyClient)
postgresThreadRepo := postgres.NewThreadRepository(postgresDB)

// Services
threadService := service.NewThreadService(
    valkeyThreadRepo,
    valkeyContractRepo,
    postgresThreadRepo,
    postgresContractVersionRepo,
)

// Handlers
threadHandler := websocket.NewThreadHandler(threadService)

// Register
wsRouter.Handle("start_thread", threadHandler.HandleStartThread)
```

## Testing Strategy

### Unit Tests
- Mock all external dependencies (DB, cache)
- Test each layer independently
- Focus on business logic and edge cases

### Integration Tests
- Use test database and Redis
- Test full flow: WebSocket → Service → Repository
- Verify data consistency

### Test Coverage Goals
- Domain entities: 100%
- Services: >90%
- Repositories: >85%
- Handlers: >80%

## Key Go Concepts Used

### 1. Interfaces for Dependency Injection
```go
type ThreadRepository interface {
    CreateThread(ctx context.Context, thread *Thread) error
    GetThread(ctx context.Context, threadID string) (*Thread, error)
}
```
**Why?** Enables mocking for tests, loose coupling

### 2. Context for Cancellation
```go
func (s *Service) CreateThread(ctx context.Context, req Request) (*Thread, error)
```
**Why?** Timeout handling, request cancellation, tracing

### 3. Error Wrapping
```go
return nil, fmt.Errorf("failed to cache graph: %w", err)
```
**Why?** Preserves error chain, better debugging

### 4. json.RawMessage for Pass-Through JSON
```go
type ContractVersion struct {
    Graph json.RawMessage `json:"graph,omitempty"`
}
```
**Why?** Efficient, no unnecessary parsing

### 5. Struct Tags for Serialization
```go
type Thread struct {
    ThreadID string `json:"thread_id"`
}
```
**Why?** Control JSON field names, omit empty fields

## Common Patterns

### Repository Pattern
- Abstracts data access
- Interface in domain/service layer
- Implementation in repository layer

### Service Pattern
- Contains business logic
- Orchestrates multiple repositories
- Returns domain entities

### Clean Architecture Layers
```
Transport (WebSocket) 
    ↓
Service (Business Logic)
    ↓
Repository (Data Access)
    ↓
Database/Cache
```

## Comparison with Kotlin/Ktor

| Concept | Kotlin/Ktor | Go |
|---------|-------------|-----|
| Dependency Injection | Koin/Manual | Constructor injection |
| Async | Coroutines | Goroutines + Context |
| Error Handling | Exceptions | Error returns |
| JSON | kotlinx.serialization | encoding/json |
| HTTP Server | Ktor | net/http, Gin |
| Testing | JUnit, MockK | testing, testify |

## Next Steps

1. Complete Task 2: Graph builder implementation
2. Write comprehensive tests for each task
3. Run integration tests
4. Document any deviations from plan
5. Performance testing with load

## Questions to Consider

1. **Contract versioning:** How to handle breaking changes?
2. **Graph validation:** Should we validate DAG for cycles?
3. **TTL strategy:** Is 24hrs appropriate for all use cases?
4. **Scaling:** How many concurrent threads can we handle?
5. **Monitoring:** What metrics should we track?

## Status

- ✅ Task 1: ContractVersion graph field
- 🔄 Task 2: Graph building logic (IN PROGRESS)
- ⏳ Tasks 3-11: Pending

---

**Last Updated:** December 13, 2025
**Author:** AI Assistant
**Reviewer:** TBD
