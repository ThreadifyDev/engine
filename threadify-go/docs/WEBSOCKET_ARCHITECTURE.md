# Threadify WebSocket Server Architecture

## Table of Contents
1. [Architecture Overview](#architecture-overview)
2. [Component Responsibilities](#component-responsibilities)
3. [Message Flows](#message-flows)
   - [Connect Flow](#connect-flow)
   - [Start Thread Flow](#start-thread-flow)
   - [Record Event Flow](#record-event-flow)
   - [Close Connection Flow](#close-connection-flow)
4. [Data Models](#data-models)
5. [Execution Graph](#execution-graph)

## Architecture Overview

```mermaid
graph TB
    Client[WebSocket Client] --> WS[WebSocket Handler]
    WS --> Session[Session Management]
    WS --> TS[ThreadService]
    WS --> SES[StepEventService]
    WS --> IS[InvitationService]
    WS --> AS[AuditService]
    
    TS --> Auth[AuthService]
    TS --> CM[ConnectionManager]
    TS --> CV[ContractValidator]
    TS --> CR[ThreadRepository]
    TS --> Cache[CacheManager]
    
    SES --> Queue[Event Queue]
    SES --> Processor[Event Processor]
    
    CR --> Valkey[(Valkey)]
    CR --> Postgres[(PostgreSQL)]
    
    Cache --> Valkey
    CV --> Valkey
    CV --> Postgres
```

### Key Architectural Changes

**Recent Refactoring (v2.0):**
- ✅ **Server-driven authentication**: API key → ownerID/companyID derivation
- ✅ **Event-driven architecture**: Removed Context/Steps from Thread model
- ✅ **Non-contract workflows**: Support for threads without contracts
- ✅ **Contract version parsing**: `contract_name:version` format support

## Component Responsibilities

### WebSocket Handler (`internal/handlers/thread.go`)

| Component | Responsibility | Key Methods |
|-----------|----------------|-------------|
| **WebSocketHandler** | WebSocket connection management, message routing, session lifecycle | `HandleWebSocket()`, `handleMessage()` |
| **Session** | Per-connection state (ownerID, companyID, threadIDs) | N/A (struct) |

**Key Features:**
- WebSocket upgrade with configurable timeouts
- Session-based authentication state management
- Message routing to appropriate services
- Automatic cleanup on disconnection

### ThreadService (`internal/service/thread.go`)

| Responsibility | Description |
|----------------|-------------|
| **Connection Management** | Handles API key validation, user authentication, company assignment |
| **Thread Lifecycle** | Creates threads (contract/non-contract), manages thread metadata |
| **Event Processing** | Validates step events, processes through event pipeline |
| **Contract Validation** | Validates steps against contract definitions |

**Key Methods:**
- `HandleConnect()` - API key authentication
- `HandleStartThread()` - Thread creation (contract/non-contract)
- `HandleRecordEvent()` - Step event processing
- `HandleClose()` - Connection cleanup

### StepEventService (`internal/service/step_event.go`)

| Responsibility | Description |
|----------------|-------------|
| **Event Processing** | Processes step events through cryptographic pipeline |
| **Queue Management** | Manages event queue and processing workflow |
| **Service Lifecycle** | Starts/stops event processing service |

## Message Flows

### Connect Flow

```mermaid
sequenceDiagram
    participant Client
    participant WS as WebSocket Handler
    participant TS as ThreadService
    participant Auth as AuthService
    participant CM as ConnectionManager
    participant Sessions as Session Store

    Client->>WS: WebSocket Upgrade
    WS->>WS: Create Session
    Client->>WS: {action: "connect", apiKey: "key123", serviceName: "payment"}
    WS->>TS: HandleConnect(request)
    TS->>Auth: ValidateApiKey("key123")
    Auth-->>TS: {ownerID: "user123", companyID: "company456"}
    TS->>CM: ConnectWithOwnerAndCompany("user123", "key123", "payment", "company456")
    CM-->>TS: Success
    TS-->>WS: {status: "success", ownerID: "user123", companyID: "company456"}
    WS->>Sessions: Store session with ownerID/companyID
    WS-->>Client: Connected successfully
```

**Request:**
```json
{
  "action": "connect",
  "apiKey": "api-key-123",
  "serviceName": "payment-service"
}
```

**Response:**
```json
{
  "action": "connect",
  "status": "success",
  "message": "Connected successfully",
  "ownerId": "user-123",
  "companyId": "company-abc"
}
```

### Start Thread Flow

```mermaid
sequenceDiagram
    participant Client
    participant WS as WebSocket Handler
    participant TS as ThreadService
    participant CV as ContractValidator
    participant Repo as ThreadRepository
    participant Cache as CacheManager

    Client->>WS: {action: "startThread", contractName: "payment:2", role: "processor"}
    WS->>TS: HandleStartThread(request, ownerID, companyID)
    TS->>TS: Parse contract identifier ("payment:2" → name="payment", version=2)
    TS->>CV: LoadContractGraphIntoCache("payment", 2)
    CV-->>TS: Contract loaded
    TS->>TS: Create thread with metadata
    TS->>Repo: Save(thread)
    TS->>Cache: SetThread(threadID, thread)
    TS-->>WS: {status: "success", threadId: "uuid-123"}
    WS-->>Client: Thread started successfully
```

**Non-Contract Workflow:**
```json
{
  "action": "startThread",
  "refs": {"serviceName": "billing-service"}
}
```

**Contract Workflow:**
```json
{
  "action": "startThread",
  "contractName": "payment-flow:2",
  "role": "payment_processor",
  "refs": {"serviceName": "payment-service"}
}
```

### Record Event Flow

```mermaid
sequenceDiagram
    participant Client
    participant WS as WebSocket Handler
    participant TS as ThreadService
    participant CV as ContractValidator
    participant SES as StepEventService
    participant Queue as Event Queue

    Client->>WS: {action: "recordThreadEvent", threadId: "uuid-123", stepName: "process_payment"}
    WS->>TS: HandleRecordEvent(request, ownerID)
    TS->>Repo: GetThread(threadID)
    TS->>CV: ValidateStepInContract(contract, version, step, context)
    CV-->>TS: Validation result
    TS->>TS: Create StepEvent from request
    TS->>SES: ProcessStepEvent(stepEvent)
    SES->>Queue: Queue event for processing
    SES-->>TS: Processing started
    TS-->>WS: {status: "success", threadId: "uuid-123"}
    WS-->>Client: Event recorded successfully
```

**Request:**
```json
{
  "action": "recordThreadEvent",
  "threadId": "thread-uuid-123",
  "stepName": "data_processing",
  "type": "step_completed",
  "status": "success",
  "context": {"result": "processed", "count": 42},
  "startedAt": "2024-01-01T10:00:00Z",
  "finishedAt": "2024-01-01T10:05:00Z"
}
```

### Close Connection Flow

```mermaid
sequenceDiagram
    participant Client
    participant WS as WebSocket Handler
    participant TS as ThreadService
    participant CM as ConnectionManager
    participant Sessions as Session Store

    Client->>WS: {action: "closeConnection"}
    WS->>TS: HandleClose(ownerID)
    TS->>CM: Disconnect(ownerID)
    WS->>Sessions: Delete session
    WS-->>Client: Connection closed
    Note over WS: WebSocket connection terminated
```

## Data Models

### WebSocket Messages

#### ConnectRequest
```go
type ConnectRequest struct {
    Action       string   `json:"action"`
    ApiKey       string   `json:"apiKey"`
    ServiceName  string   `json:"serviceName,omitempty"`
    SubscribedEvents []string `json:"subscribedEvents,omitempty"`
}
```

#### StartThreadRequest
```go
type StartThreadRequest struct {
    Action       string            `json:"action"`
    ContractName string            `json:"contractName"` // Format: "name" or "name:version"
    Role         string            `json:"role,omitempty"`
    Refs         map[string]string `json:"refs,omitempty"`
}
```

#### RecordEventRequest
```go
type RecordEventRequest struct {
    Action      string            `json:"action"`
    ThreadID    string            `json:"threadId"`
    StepName    string            `json:"stepName"`
    Type        string            `json:"type"`
    StartedAt   string            `json:"startedAt"`
    FinishedAt  string            `json:"finishedAt"`
    Context     map[string]string `json:"context"`
    Refs        map[string]string `json:"refs,omitempty"`
    Status      string            `json:"status"`
    ServiceName string            `json:"serviceName,omitempty"`
}
```

### Thread Model (v2.0 - Event-Driven)

```go
type Thread struct {
    ID              string                 `json:"id"`
    ContractID      *string                `json:"contractId,omitempty"`
    ContractVersion *int                   `json:"contractVersion,omitempty"`
    ContractName    string                 `json:"contractName,omitempty"`
    Refs            map[string]string      `json:"refs,omitempty"`
    OwnerID         string                 `json:"ownerId"`
    CompanyID       string                 `json:"companyId"`
    Status          ThreadStatus           `json:"status"`
    CurrentStep     string                 `json:"currentStep"`
    LastHash        string                 `json:"lastHash"`
    StartedAt       time.Time              `json:"startedAt"`
    CompletedAt     *time.Time             `json:"completedAt,omitempty"`
    Error           string                 `json:"error,omitempty"`
}
```

**Key Changes in v2.0:**
- ❌ **Removed:** `Context` field (now in events)
- ❌ **Removed:** `Steps` field (now in events)
- ✅ **Added:** `Refs` field for external references
- ✅ **Enhanced:** Company-based multi-tenancy

### Step Event Model

```go
type StepEvent struct {
    StepID      string                 `json:"step_id"`
    Type        string                 `json:"type"` // step_started, step_completed, step_failed
    Context     map[string]interface{} `json:"context"`
    Status      string                 `json:"status"` // success, failure
    Timestamp   time.Time              `json:"timestamp"`
    ServiceName string                 `json:"service_name"`
    ThreadID    string                 `json:"thread_id"`
    StepName    string                 `json:"step_name"`
    StartedAt   string                 `json:"started_at"`
    FinishedAt  string                 `json:"finished_at"`
}
```

## Execution Graph

### Complete Request Processing Flow

```mermaid
graph TD
    A[WebSocket Message] --> B{Parse Action}
    
    B -->|connect| C[Connect Flow]
    B -->|startThread| D[Start Thread Flow]
    B -->|recordThreadEvent| E[Record Event Flow]
    B -->|closeConnection| F[Close Flow]
    
    C --> C1[Validate API Key]
    C1 --> C2[Derive ownerID/companyID]
    C2 --> C3[Store Session]
    C3 --> C4[Return Success]
    
    D --> D1[Parse Contract Identifier]
    D1 --> D2{Contract Provided?}
    D2 -->|Yes| D3[Load Contract Graph]
    D2 -->|No| D4[Create Non-Contract Thread]
    D3 --> D5[Create Thread with Contract]
    D4 --> D6[Create Thread without Contract]
    D5 --> D7[Save Thread]
    D6 --> D7
    D7 --> D8[Cache Thread]
    D8 --> D9[Return Thread ID]
    
    E --> E1[Get Thread from Repository]
    E1 --> E2{Contract Thread?}
    E2 -->|Yes| E3[Validate Step in Contract]
    E2 -->|No| E4[Skip Contract Validation]
    E3 --> E5[Create Step Event]
    E4 --> E5
    E5 --> E6[Process Step Event]
    E6 --> E7[Queue for Processing]
    E7 --> E8[Return Success]
    
    F --> F1[Remove Session]
    F1 --> F2[Disconnect from ConnectionManager]
    F2 --> F3[Close WebSocket]
```

### Service Dependencies

```mermaid
graph LR
    subgraph "WebSocket Layer"
        WH[WebSocket Handler]
        S[Session]
    end
    
    subgraph "Service Layer"
        TS[ThreadService]
        SES[StepEventService]
        IS[InvitationService]
        AS[AuditService]
    end
    
    subgraph "Data Layer"
        Auth[AuthService]
        CM[ConnectionManager]
        CV[ContractValidator]
        CR[ThreadRepository]
        CS[CacheService]
    end
    
    subgraph "Storage Layer"
        V[(Valkey)]
        P[(PostgreSQL)]
    end
    
    WH --> TS
    WH --> SES
    WH --> IS
    WH --> AS
    
    TS --> Auth
    TS --> CM
    TS --> CV
    TS --> CR
    TS --> CS
    
    CR --> V
    CR --> P
    CV --> V
    CV --> P
    CS --> V
```

### Error Handling Flow

```mermaid
graph TD
    A[Request Received] --> B[Validate Input]
    B --> C{Valid?}
    C -->|No| D[Return Error Response]
    C -->|Yes| E[Process Request]
    E --> F{Service Error?}
    F -->|Yes| G[Log Error]
    G --> H[Return Error Response]
    F -->|No| I[Return Success Response]
    
    D --> J[Client Handles Error]
    H --> J
    I --> K[Client Continues]
```

## Performance Considerations

### Connection Management
- **Session Storage**: In-memory `sync.Map` for fast lookup
- **Connection Limits**: Configurable via ConnectionManager
- **Timeout Handling**: WebSocket handshake timeout (10s default)

### Data Storage Strategy
- **Thread Metadata**: Valkey for fast access with TTL
- **Contract Graphs**: Three-tier caching (Valkey → In-memory → PostgreSQL)
- **Event Processing**: Queue-based for high throughput

### Scalability Features
- **Multi-tenancy**: Company-based isolation
- **Event-driven**: Reduced thread model size
- **Async Processing**: Step events processed separately

## Security Architecture

### Authentication Flow
1. **API Key Validation**: Mock service maps keys to user/company
2. **Session Management**: OwnerID/companyID stored in session
3. **Request Authorization**: Session-based for all subsequent requests

### Data Isolation
- **Company-based**: All data filtered by companyID
- **User Threads**: Users can only access their own threads
- **Contract Access**: Validated against company permissions

## Configuration

### WebSocket Configuration
```go
var upgrader = websocket.Upgrader{
    CheckOrigin:      func(r *http.Request) bool { return true },
    HandshakeTimeout: 10 * time.Second,
    ReadBufferSize:   1024,
    WriteBufferSize:  1024,
}
```

### Service Configuration
- **Contract TTL**: Configurable cache duration
- **Thread TTL**: Configurable thread persistence
- **Auth Secret**: JWT secret for token generation

---

*This documentation covers the complete WebSocket server architecture as of v2.0, including the recent refactoring to event-driven processing and server-driven authentication.*
