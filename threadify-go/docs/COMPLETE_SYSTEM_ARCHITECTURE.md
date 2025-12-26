# Threadify Complete System Architecture

## Table of Contents
1. [System Overview](#system-overview)
2. [Master Architecture Diagram](#master-architecture-diagram)
3. [Entry Points Layer](#entry-points-layer)
4. [Service Layer](#service-layer)
5. [Repository Layer](#repository-layer)
6. [Storage Layer](#storage-layer)
7. [Data Flow Matrix](#data-flow-matrix)
8. [Complete Execution Flows](#complete-execution-flows)
9. [Authentication & Security](#authentication--security)
10. [Error Handling & Troubleshooting](#error-handling--troubleshooting)

## System Overview

Threadify is a multi-tenant, event-driven workflow engine with dual entry points (REST API and WebSocket) and a hybrid storage strategy using both Valkey (Redis-compatible) and PostgreSQL.

### Core Architectural Principles
- **Event-Driven**: Step processing through cryptographic event pipeline
- **Multi-Tenant**: Company-based data isolation
- **Hybrid Storage**: Valkey for fast access, PostgreSQL for persistence
- **Dual Entry Points**: REST for contracts, WebSocket for real-time workflows

## Master Architecture Diagram

```mermaid
graph TB
    subgraph "Entry Points"
        REST[REST API Handler]
        WS[WebSocket Handler]
    end
    
    subgraph "Service Layer"
        TS[ThreadService]
        CS[ContractService]
        SES[StepEventService]
        AS[AuthService]
        IS[InvitationService]
        AUS[AuditService]
        ConnS[ConnectionService]
        CacheS[CacheService]
        ValS[ContractValidationService]
    end
    
    subgraph "Repository Layer"
        subgraph "PostgreSQL"
            CPR[Contract Repository]
            TR[Thread Repository]
        end
        
        subgraph "Valkey"
            VCR[Contract Graph Repository]
            VTR[Thread Repository]
            VQ[Client Queue]
        end
    end
    
    subgraph "Storage Layer"
        PG[(PostgreSQL)]
        VK[(Valkey)]
    end
    
    REST --> CS
    REST --> AS
    WS --> TS
    WS --> SES
    WS --> IS
    WS --> AUS
    
    TS --> AS
    TS --> ConnS
    TS --> ValS
    TS --> VTR
    TS --> CacheS
    
    CS --> CPR
    CS --> AS
    
    SES --> VQ
    
    ValS --> VCR
    ValS --> CPR
    ValS --> CacheS
    
    CPR --> PG
    TR --> PG
    VCR --> VK
    VTR --> VK
    VQ --> VK
    CacheS --> VK
```

## Entry Points Layer

### REST API Handler (`internal/handlers/contracts.go`)

**Purpose**: Contract management and authentication for external systems

| Endpoint | Handler | Service | Authentication |
|----------|---------|---------|----------------|
| `POST /contracts/login` | `Login()` | AuthService | JWT-based |
| `GET /contracts` | `GetAllContracts()` | ContractService | JWT Bearer |
| `POST /contracts` | `CreateContract()` | ContractService | JWT Bearer |
| `GET /contracts/:id` | `GetContract()` | ContractService | JWT Bearer |
| `PUT /contracts/:id` | `UpdateContract()` | ContractService | JWT Bearer |
| `DELETE /contracts/:id` | `DeleteContract()` | ContractService | JWT Bearer |
| `GET /contracts/:id/versions` | `GetAllContractVersions()` | ContractService | JWT Bearer |
| `DELETE /contracts/:id/versions/:version` | `DeleteContractVersion()` | ContractService | JWT Bearer |

**Authentication Flow**:
```mermaid
sequenceDiagram
    participant Client
    participant REST as REST Handler
    participant Auth as AuthService
    participant CS as ContractService
    
    Client->>REST: POST /contracts/login {userId}
    REST->>Auth: GenerateToken(userId)
    Auth-->>REST: JWT Token
    REST-->>Client: {token, userId}
    
    Note over Client: Subsequent requests use Bearer token
    Client->>REST: GET /contracts Authorization: Bearer <token>
    REST->>REST: Validate JWT → Extract ownerId
    REST->>CS: GetAllContracts(context, ownerId)
    CS-->>REST: Contract list
    REST-->>Client: Contracts response
```

### WebSocket Handler (`internal/handlers/thread.go`)

**Purpose**: Real-time workflow execution and step processing

| Action | Handler | Service | Description |
|--------|---------|---------|-------------|
| `connect` | `HandleConnect()` | ThreadService | API key authentication |
| `startThread` | `HandleStartThread()` | ThreadService | Thread creation |
| `recordThreadEvent` | `HandleRecordEvent()` | ThreadService | Step event processing |
| `closeConnection` | `HandleClose()` | ThreadService | Connection cleanup |

## Service Layer

### ThreadService (`internal/service/thread.go`)

**Core Responsibilities**:
- WebSocket connection management
- Thread lifecycle (create, start, complete, fail)
- Event processing pipeline
- Contract validation integration

**Key Methods**:
```go
func (s *ThreadService) HandleConnect(req *models.ConnectRequest) *models.ConnectResponse
func (s *ThreadService) HandleStartThread(req *models.StartThreadRequest, ownerID, companyID string) *models.StartThreadResponse
func (s *ThreadService) HandleRecordEvent(req *models.RecordEventRequest, ownerID string) *models.RecordEventResponse
func (s *ThreadService) HandleClose(ownerID string) *models.CloseConnectionResponse
```

**Thread Creation Flow**:
```mermaid
graph TD
    A[StartThread Request] --> B{Contract Name Provided?}
    B -->|Yes| C[Parse contract:version]
    B -->|No| D[Non-contract workflow]
    C --> E[Load contract graph]
    D --> F[Create thread v0]
    E --> G[Create thread with contract]
    F --> H[Save to Valkey]
    G --> H
    H --> I[Cache thread]
    I --> J[Return thread ID]
```

### ContractService (`internal/service/contract.go`)

**Core Responsibilities**:
- Contract CRUD operations
- Version management
- YAML parsing and validation
- Multi-tenancy enforcement

**Key Methods**:
```go
func (s *ContractService) CreateContract(ctx context.Context, ownerID, createdBy, contractYAML string) (int, interface{})
func (s *ContractService) GetContract(ctx context.Context, contractID, requesterID string, version *int) (int, interface{})
func (s *ContractService) UpdateContract(ctx context.Context, contractID, ownerID, createdBy, contractYAML string) (int, interface{})
func (s *ContractService) DeleteContract(ctx context.Context, contractID, ownerID string) (int, interface{})
```

**Contract Processing Pipeline**:
```mermaid
graph TD
    A[YAML Contract] --> B[Parse YAML]
    B --> C[Validate Structure]
    C --> D[Build Contract Graph]
    D --> E[Store in PostgreSQL]
    E --> F[Cache Graph in Valkey]
    F --> G[Return Contract ID]
```

### StepEventService (`internal/service/step_event.go`)

**Core Responsibilities**:
- Event queue management (thread-specific sequential processing)
- Cryptographic processing pipeline (SHA-256 hashing)
- Event batching for performance
- Asynchronous processing with worker pool

**Key Features**:
- **Thread-specific queues**: Each thread gets its own queue for sequential event processing
- **Global batching**: Events batched globally (configurable batch size/timeout)
- **In-memory hash cache**: Last hash per thread cached in memory
- **Worker pool**: Configurable number of workers (default: 4)

**Processing Pipeline**:
```mermaid
graph TD
    A[Step Event] --> B[Thread-Specific Queue]
    B --> C[Sequential Processing per Thread]
    C --> D[Generate SHA-256 Hash]
    D --> E[Add to Global Batch]
    E --> F{Batch Full or Timeout?}
    F -->|Yes| G[Write Batch to Valkey]
    F -->|No| H[Wait for More Events]
    G --> I[Update Thread State]
```

**Additional Services**:

### AuditEventService (`internal/service/audit.go`)

- Logs invitation token creation/usage
- Logs thread join events
- Stores events in Valkey queues with configurable retention
- Async processing (non-blocking)

### InvitationTokenService (`internal/service/invitation.go`)

- Creates JWT tokens for cross-org threading
- Validates tokens and extracts claims
- Role and permission validation
- Configurable expiry (24h default)

### CacheService (`internal/service/cache.go`)

- In-memory caching for threads and contract graphs
- Thread-safe with RWMutex
- First-tier cache before Valkey lookup

### ConnectionService (`internal/service/connection.go`)

- In-memory WebSocket connection tracking
- Company-based multi-tenancy support
- Session management (no Valkey storage)

### AuthService (`internal/service/auth.go`)

**Core Responsibilities**:
- JWT token generation and validation
- API key validation (WebSocket)
- User authentication and authorization

**Authentication Methods**:
```go
func (s *AuthService) GenerateToken(userID string) (string, error)
func (s *AuthService) ValidateToken(tokenString string) (*jwt.MapClaims, error)
func (s *AuthService) ValidateApiKey(apiKey string) (*UserInfo, error)
```

## Repository Layer

### PostgreSQL Repositories (`internal/repository/postgres/`)

#### Contract Repository (`contract.go`)
- **Purpose**: Persistent contract storage
- **Operations**: CRUD operations, versioning, soft deletes
- **Schema**: Contracts, contract_versions, users tables

#### Thread Repository (`thread.go`)
- **Purpose**: Thread metadata persistence
- **Operations**: Thread storage, retrieval, TTL management
- **Schema**: Threads, thread_events tables

### Valkey Repositories (`internal/repository/valkey/`)

#### Contract Graph Repository (`contract_graph.go`)
- **Purpose**: High-performance contract graph caching
- **Operations**: Graph storage, retrieval, TTL management
- **Data Structure**: JSON-serialized contract graphs

#### Thread Repository (`thread.go`)
- **Purpose**: Real-time thread state management
- **Operations**: Thread caching, state updates, session management
- **Data Structure**: Hash-based thread storage

#### Client Queue (`client_queue.go`)
- **Purpose**: Event queuing and processing
- **Operations**: Queue management, event streaming
- **Data Structure**: Redis streams or lists

## Storage Layer

### PostgreSQL

**Purpose**: Persistent, relational data storage
**Usage**:
- Contract definitions and versions
- Contract graphs (stored in contract_versions.graph)
- User and company information (future)

**Key Tables**:
```sql
contracts (id, owner_id, name, content_hash, latest_version, is_public, is_deleted, created_at, updated_at)
contract_versions (id, contract_id, version, content, content_hash, graph, created_by, is_deleted, created_at, updated_at)
```

**Note**: Thread storage in PostgreSQL exists in code but is **not used** in production. Threads are ephemeral (Valkey only).

### Valkey (Redis-compatible)

**Purpose**: High-performance caching and real-time data
**Usage**:
- Active thread states
- Contract graph caching
- Session management
- Event queuing
- Real-time counters and metrics

**Key Data Patterns**:
```redis
# Thread storage (ephemeral with TTL)
thread:{thread_id} → JSON thread object

# Contract graph caching (with TTL)
contract_graph:{contract_name}:{version} → JSON graph

# Step event batching
step_events:batch → List of hashed events (batched writes)

# Audit event queuing
queue:audit_events → List of audit events (with retention)

# Client connections (managed in-memory, not Valkey)
# Sessions managed by ConnectionService (in-memory)
```

## Data Flow Matrix

| Operation | Valkey | PostgreSQL | Both | Purpose |
|-----------|--------|------------|-------|---------|
| **Contract CRUD** | ❌ | ✅ | ❌ | Persistent contract storage |
| **Contract Graph Access** | ✅ | ✅ | ✅ | Valkey cache + PostgreSQL persistence |
| **Thread Creation** | ✅ | ❌ | ❌ | Real-time thread state (ephemeral) |
| **Thread Storage** | ✅ | ❌ | ❌ | Valkey only with TTL (no PostgreSQL) |
| **Step Events** | ✅ | ❌ | ❌ | In-memory queues + Valkey batching |
| **User Authentication** | ❌ | ❌ | ❌ | In-memory session management |
| **Audit Logging** | ✅ | ❌ | ❌ | Valkey queues with retention |
| **Contract Validation** | ✅ | ✅ | ✅ | Three-tier: In-memory → Valkey → PostgreSQL |
| **Connection Management** | ❌ | ❌ | ❌ | In-memory (ConnectionService) |

## Complete Execution Flows

### 1. Contract Creation Flow (REST)

```mermaid
sequenceDiagram
    participant Client
    participant REST as REST Handler
    participant Auth as AuthService
    participant CS as ContractService
    participant CPR as Contract Repo
    participant VCR as Valkey Contract Repo
    participant PG as PostgreSQL
    
    Client->>REST: POST /contracts (JWT + YAML)
    REST->>REST: Validate JWT → Extract ownerID
    REST->>CS: CreateContract(ctx, ownerID, userID, yaml)
    CS->>CS: Parse and validate YAML
    CS->>CPR: Save contract metadata
    CPR->>PG: INSERT into contracts table
    CS->>VCR: Cache contract graph
    VCR->>Valkey: SET contract:{id}:1
    CS-->>REST: Contract created response
    REST-->>Client: {contractId, version, status}
```

### 2. Thread Execution Flow (WebSocket)

```mermaid
sequenceDiagram
    participant Client
    participant WS as WebSocket Handler
    participant TS as ThreadService
    participant Auth as AuthService
    participant VTR as Valkey Thread Repo
    participant SES as StepEventService
    participant VQ as Valkey Queue
    
    Client->>WS: {action: "startThread", contract: "payment:2"}
    WS->>TS: HandleStartThread(req, ownerID, companyID)
    TS->>TS: Parse contract identifier
    TS->>VTR: Create thread with metadata
    VTR->>Valkey: SET thread:{threadId}
    TS-->>WS: {status: "success", threadId}
    WS-->>Client: Thread started
    
    Client->>WS: {action: "recordThreadEvent", stepData}
    WS->>TS: HandleRecordEvent(req, ownerID)
    TS->>TS: Validate step in contract
    TS->>SES: ProcessStepEvent(event)
    SES->>VQ: Queue event for processing
    SES-->>TS: Processing started
    TS-->>WS: {status: "success"}
    WS-->>Client: Event recorded
```

### 3. Event Processing Flow (Async)

```mermaid
sequenceDiagram
    participant Queue as Event Queue
    participant Processor as Event Processor
    participant VTR as Valkey Thread Repo
    participant CPR as Contract Repo
    participant PG as PostgreSQL
    
    Queue->>Processor: Dequeue step event
    Processor->>Processor: Validate event structure
    Processor->>Processor: Generate cryptographic hash
    Processor->>VTR: Update thread state
    Processor->>CPR: Validate step permissions
    Processor->>PG: Persist event history
    Processor->>Queue: Queue next step (if applicable)
    Processor-->>Queue: Mark event processed
```

## Authentication & Security

### JWT Authentication (REST API)

```mermaid
graph TD
    A[Login Request] --> B[Generate JWT Token]
    B --> C[Token contains: userId, ownerId, companyId, role, exp]
    C --> D[Client stores token]
    D --> E[Subsequent requests include Bearer token]
    E --> F[Server validates signature and expiration]
    F --> G[Extract claims for authorization]
```

### API Key Authentication (WebSocket)

```mermaid
graph TD
    A[WebSocket Connect] --> B[Client sends API key]
    B --> C[Server validates API key]
    C --> D[Derive ownerID and companyID]
    D --> E[Create authenticated session]
    E --> F[All subsequent requests use session auth]
```

### Multi-Tenancy Enforcement

| Layer | Enforcement Mechanism |
|-------|----------------------|
| **REST Handler** | JWT claims contain companyID |
| **WebSocket Handler** | Session stores companyID |
| **Service Layer** | All queries filtered by companyID |
| **Repository Layer** | Company-based data isolation |
| **Storage Layer** | Separate keyspaces and tables |

## Error Handling & Troubleshooting

### Error Classification by Layer

| Layer | Common Errors | Troubleshooting |
|-------|---------------|----------------|
| **Entry Points** | Invalid JWT, malformed requests | Check token format, request body |
| **Service Layer** | Validation failures, permission denied | Verify user permissions, data format |
| **Repository Layer** | Connection failures, timeouts | Check database connectivity, query performance |
| **Storage Layer** | Disk full, memory issues | Monitor storage, memory usage |

### Common Issues and Solutions

#### "Step is still running" Issue
**Symptoms**: Step events not processing, threads stuck
**Root Causes**:
1. Event queue backup
2. StepEventService not running
3. Valkey connection issues

**Troubleshooting**:
```bash
# Check event queue depth
> LLEN step_events:queue

# Check StepEventService status
> ps aux | grep step_event_service

# Check Valkey connectivity
> redis-cli ping
```

#### Connection Timeouts
**Symptoms**: WebSocket connections dropping
**Root Causes**:
1. Network issues
2. Session cleanup problems
3. Memory pressure

**Troubleshooting**:
```bash
# Check active sessions
> HGETALL sessions:*

# Check WebSocket handler logs
> tail -f logs/websocket.log

# Monitor memory usage
> top | grep threadify
```

#### Contract Validation Failures
**Symptoms**: Contract creation/update failures
**Root Causes**:
1. Invalid YAML syntax
2. Circular dependencies
3. Missing required fields

**Troubleshooting**:
```bash
# Validate YAML locally
> yamllint contract.yaml

# Check contract graph structure
> redis-cli GET contract:{id}:{version}

# Review validation logs
> grep -i validation logs/contract.log
```

### Monitoring and Observability

#### Key Metrics to Monitor
- **WebSocket Connections**: Active sessions, connection rate
- **Thread Processing**: Creation rate, completion rate, error rate
- **Event Processing**: Queue depth, processing latency
- **Storage Performance**: Valkey memory usage, PostgreSQL query times
- **Authentication**: Token validation rate, API key validation failures

#### Log Patterns
```bash
# Authentication failures
grep "Invalid.*token\|Invalid.*api" logs/auth.log

# Thread lifecycle events
grep "thread.*started\|thread.*completed" logs/thread.log

# Event processing metrics
grep "event.*queued\|event.*processed" logs/event.log

# Storage performance
grep "slow.*query\|connection.*timeout" logs/storage.log
```

## Configuration Management

### Environment Variables
```bash
# Database Configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=threadify
POSTGRES_USER=threadify
POSTGRES_PASSWORD=password

# Valkey Configuration  
VALKEY_HOST=localhost
VALKEY_PORT=6379
VALKEY_DB=0

# Authentication
JWT_SECRET=your-secret-key
JWT_EXPIRATION=24h

# Service Configuration
WEBSOCKET_PORT=8080
REST_PORT=8081
CONTRACT_TTL=3600
THREAD_TTL=7200
```

### Service Dependencies
```mermaid
graph TD
    A[Threadify Server] --> B[PostgreSQL]
    A --> C[Valkey]
    A --> D[Configuration]
    
    B --> E[Contracts]
    B --> F[Users]
    B --> G[Audit Logs]
    
    C --> H[Thread States]
    C --> I[Contract Graphs]
    C --> J[Event Queues]
    C --> K[Sessions]
    
    D --> L[Database URLs]
    D --> M[Auth Secrets]
    D --> N[TTL Settings]
```

---

*This documentation covers the complete Threadify system architecture as of v2.0, including the dual entry points (REST/WebSocket), hybrid storage strategy, and event-driven processing pipeline.*
