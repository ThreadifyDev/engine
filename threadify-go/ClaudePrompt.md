# Historical implementation plan

This file preserves an early implementation prompt. Its `Thread.create`,
`start_thread`, routing examples, and implementation checklist below are
historical design material, not the current SDK or WebSocket contract. Do not
copy those old entry points into new integrations.

Current integrations use JavaScript `connection.thread(threadKey, options?)`,
Python `await connection.thread(thread_key, options=None)`, or Go
`connection.Thread(ctx, threadKey, options...)`. The wire action is `thread` with
required `threadKey`; SDK option `contract` maps to wire field `contractName`.
Optional creation fields are `label`, `contractName`, `refs`, `tags`,
`serviceName`, and `role`.

The Engine atomically creates or resumes one thread for a trimmed, nonblank,
company-scoped key of at most 1024 UTF-8 bytes. Resuming loads the stored contract
and pinned version; callers need not send the contract again. Conflicting
contracts are rejected, and creation metadata does not overwrite stored values.
An unknown key without a contract creates a free-form thread, so initialize
contracted sessions before workers or telemetry report steps. Closed threads
cannot resume or accept writes, and their keys cannot create replacement threads.
OTLP and SDK exporters use `threadify.thread_key` for this same application key.

See [the current WebSocket API](../docs/WEBSOCKET.md) and
[SDK thread-key usage](../threadify-sdk/Documentation.md#create-or-resume-by-thread-key).

---

** THis is for Threadify-go **
** Ensure to check a file doesn't exist before attempting to create it **
** Ensure to check a function doesn't exist before attempting to create it **
** Ensure to check a struct doesn't exist before attempting to create it **
** Ensure to check a struct doesn't exist before attempting to create it **

** Ignore the architecture structure in prompt for folders that already exists but those that don't you should create **
** Ensure to put DB query (valkey or postgres) functions in the repository layer - so follow the architecture structure **

** Where you are not sure, ask questions **

** Draft a comprehensive documentation of the changes you made and the architectural structure that even a junior engineer new to GoLang can understand (this developer is experienced in JS and Kotlin/Ktor). **

** FOLLOW THE BELOW PROMPT BUT USING THE ABOVE AS YOUR GUIDE **

# LLM Instructions: Implement Thread.create Backend in Go

## Context
You are implementing the Thread.create backend for Threadify, a distributed workflow observability platform. This is the core entry point where workflows begin. The system uses Valkey (Redis) for hot storage, PostgreSQL for persistent storage, and WebSocket for real-time communication with SDKs.

**Existing infrastructure:**
- WebSocket server already running
- PostgreSQL connection pool active
- Valkey client connected
- Contract and ContractVersion CRUD operations exist (DO NOT MODIFY)

## Requirements

### Test-Driven Development (TDD)
- Write tests FIRST that fail (red)
- Implement code to make tests pass (green)
- Ensure each new green test doesn't break previous green tests
- Run tests after each implementation step
- Each test should be focused and test one thing

### Code Style
- **Concise code** - no over-abstraction, keep it simple
- **Separation of concerns** - database, cache, business logic in separate packages
- **Clean architecture** - follow Go's clean architecture patterns
- **Small files** - don't cram everything into one file, split logically
- **No premature optimization** - write clear code first

### Architecture Structure (Preserve Existing)
```
threadify/
├── cmd/
│   └── server/
│       └── main.go              # Already has WebSocket, DB, Valkey setup
├── internal/
│   ├── domain/
│   │   ├── contract.go          # EXISTS - DO NOT MODIFY
│   │   ├── contract_version.go  # EXISTS - ADD graph field
│   │   └── thread.go            # CREATE NEW
│   ├── service/
│   │   ├── contract.go          # EXISTS - DO NOT MODIFY
│   │   ├── contract_graph.go    # CREATE NEW - Graph building logic
│   │   └── thread.go            # CREATE NEW - Thread management
│   ├── repository/
│   │   ├── valkey/
│   │   │   ├── contract.go      # EXISTS - ADD cache methods
│   │   │   └── thread.go        # CREATE NEW
│   │   └── postgres/
│   │       ├── contract.go      # EXISTS - DO NOT MODIFY
│   │       ├── contract_version.go # EXISTS - ADD graph column
│   │       └── thread.go        # CREATE NEW
│   └── transport/
│       └── websocket/
│           └── thread.go        # CREATE NEW - HandleStartThread
└── pkg/
    └── errors/
        └── errors.go            # ADD thread-specific errors
```

---

## Task 1: Update ContractVersion Domain Entity

**File:** `internal/domain/contract_version.go` (EXISTING - MODIFY)

**Add graph field to existing struct:**
```go
type ContractVersion struct {
    ID              int64
    ContractID      int64
    Version         int
    Description     string
    Content         []byte  // YAML/JSON content - ALREADY EXISTS
    Graph           []byte  // NEW: Contract graph (JSON) - ADD THIS
    CreatedBy       string
    CreatedAt       time.Time
    UpdatedAt       time.Time
    // ... other existing fields
}
```

**Test requirements:**
- Test ContractVersion serialization with graph field
- Test ContractVersion with nil graph (should be valid)

---

## Task 2: Contract Graph Building Logic

**File:** `internal/service/contract_graph.go` (CREATE NEW)

**Purpose:** Convert contract YAML/JSON content into a DAG (Directed Acyclic Graph) for efficient validation and navigation.

### Graph Structure

```go
type ContractGraph struct {
    ContractID      string            `json:"contract_id"`
    Version         int               `json:"version"`
    Graph           Graph             `json:"graph"`
}

type Graph struct {
    Nodes      map[string]GraphNode `json:"nodes"`       // Key: step_id
    FinalStep  string               `json:"final_step"`  // Last step in workflow
}

type GraphNode struct {
    ID              string            `json:"id"`
    Owner           string            `json:"owner,omitempty"`
    Type            string            `json:"type"`              // "step" or "parallel_group"
    Mode            string            `json:"mode,omitempty"`    // "all_of" or "any_of" for groups
    Required        bool              `json:"required"`
    DependsOn       []string          `json:"depends_on"`
    Next            []string          `json:"next"`              // Steps that depend on this
    Steps           []string          `json:"steps,omitempty"`   // For parallel_group type
    Timeout         string            `json:"timeout,omitempty"`
    MaxDuration     string            `json:"max_duration,omitempty"`  // For groups
    BusinessContext map[string]string `json:"business_context,omitempty"`
    ParentGroup     string            `json:"parent_group,omitempty"`
}
```

### Contract YAML Structure (Reference)

```yaml
contract_name: payment_processing
version: 1
description: Payment processing with fraud checks

parties:
  - merchant
  - payment_processor
  - bank

steps:
  - id: payment_initiated
    owner: merchant
    business_context:
      amount: number
      currency: string

  - id: fraud_check
    owner: payment_processor
    depends_on: payment_initiated
    timeout: 2s

  - id: risk_assessment
    owner: payment_processor
    depends_on: payment_initiated
    timeout: 2s

  - id: bank_authorization
    owner: bank
    depends_on:
      - fraud_check
      - risk_assessment
    timeout: 5s

  - id: payment_complete
    owner: merchant
    depends_on: bank_authorization

groups:
  - id: fraud_validation
    steps:
      - fraud_check
      - risk_assessment
    rules:
      all_must_succeed: true
      max_combined_duration: 3s

validation:
  max_duration: 10s
```

### Graph Building Algorithm

```go
type GraphBuilder struct{}

func NewGraphBuilder() *GraphBuilder {
    return &GraphBuilder{}
}

// BuildGraph converts contract content (YAML/JSON) to ContractGraph
func (b *GraphBuilder) BuildGraph(contractName string, version int, content []byte) (*ContractGraph, error) {
    // 1. Parse YAML/JSON content into Contract struct
    var contract Contract
    err := yaml.Unmarshal(content, &contract)
    if err != nil {
        return nil, fmt.Errorf("failed to parse contract: %w", err)
    }
    
    // 2. Build nodes map
    nodes := make(map[string]GraphNode)
    stepMap := make(map[string]Step)
    
    // Index all steps
    for _, step := range contract.Steps {
        stepMap[step.ID] = step
    }
    
    // 3. Build step nodes with dependencies
    for _, step := range contract.Steps {
        dependsOn := step.DependsOn
        if dependsOn == nil {
            dependsOn = []string{}
        }
        
        // Find which steps depend on THIS step (reverse lookup for "next")
        next := []string{}
        for _, otherStep := range contract.Steps {
            if otherStep.DependsOn != nil {
                for _, dep := range otherStep.DependsOn {
                    if dep == step.ID {
                        next = append(next, otherStep.ID)
                    }
                }
            }
        }
        
        // Check if step belongs to a group
        parentGroup := findParentGroup(step.ID, contract.Groups)
        
        nodes[step.ID] = GraphNode{
            ID:              step.ID,
            Owner:           step.Owner,
            Type:            "step",
            Required:        true, // Default, can be in contract
            DependsOn:       dependsOn,
            Next:            next,
            Timeout:         step.Timeout,
            BusinessContext: step.BusinessContext,
            ParentGroup:     parentGroup,
        }
    }
    
    // 4. Build parallel group nodes
    for _, group := range contract.Groups {
        groupDependsOn := findGroupDependencies(group, contract.Steps)
        groupNext := findGroupNext(group, contract.Steps)
        
        mode := "any_of"
        if group.Rules != nil && group.Rules.AllMustSucceed {
            mode = "all_of"
        }
        
        maxDuration := ""
        if group.Rules != nil {
            maxDuration = group.Rules.MaxCombinedDuration
        }
        
        nodes[group.ID] = GraphNode{
            ID:          group.ID,
            Type:        "parallel_group",
            Mode:        mode,
            Required:    true,
            Steps:       group.Steps,
            DependsOn:   groupDependsOn,
            Next:        groupNext,
            MaxDuration: maxDuration,
        }
    }
    
    // 5. Find final step (step with no "next")
    finalStep := ""
    for id, node := range nodes {
        if node.Type == "step" && len(node.Next) == 0 {
            finalStep = id
            break
        }
    }
    
    return &ContractGraph{
        ContractID: contractName,
        Version:    version,
        Graph: Graph{
            Nodes:     nodes,
            FinalStep: finalStep,
        },
    }, nil
}

// Helper: Find which steps a group depends on
func findGroupDependencies(group Group, steps []Step) []string {
    // Look at group's steps, find their common dependencies
    dependsOn := []string{}
    for _, stepID := range group.Steps {
        for _, step := range steps {
            if step.ID == stepID && step.DependsOn != nil {
                // Take union of all dependencies
                for _, dep := range step.DependsOn {
                    if !contains(dependsOn, dep) && !contains(group.Steps, dep) {
                        dependsOn = append(dependsOn, dep)
                    }
                }
            }
        }
    }
    return dependsOn
}

// Helper: Find which steps depend on this group
func findGroupNext(group Group, steps []Step) []string {
    next := []string{}
    for _, step := range steps {
        if step.DependsOn != nil {
            for _, dep := range step.DependsOn {
                // If step depends on any step in the group, it depends on the group
                if contains(group.Steps, dep) {
                    next = append(next, step.ID)
                    break
                }
            }
        }
    }
    return next
}

// Helper: Find parent group for a step
func findParentGroup(stepID string, groups []Group) string {
    for _, group := range groups {
        if contains(group.Steps, stepID) {
            return group.ID
        }
    }
    return ""
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

### Contract Parsing Structs

```go
type Contract struct {
    ContractName string   `yaml:"contract_name" json:"contract_name"`
    Version      int      `yaml:"version" json:"version"`
    Description  string   `yaml:"description" json:"description"`
    Parties      []string `yaml:"parties" json:"parties"`
    Steps        []Step   `yaml:"steps" json:"steps"`
    Groups       []Group  `yaml:"groups,omitempty" json:"groups,omitempty"`
    Validation   *Validation `yaml:"validation,omitempty" json:"validation,omitempty"`
}

type Step struct {
    ID              string            `yaml:"id" json:"id"`
    Owner           string            `yaml:"owner" json:"owner"`
    DependsOn       []string          `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
    Timeout         string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
    BusinessContext map[string]string `yaml:"business_context,omitempty" json:"business_context,omitempty"`
}

type Group struct {
    ID    string      `yaml:"id" json:"id"`
    Steps []string    `yaml:"steps" json:"steps"`
    Rules *GroupRules `yaml:"rules,omitempty" json:"rules,omitempty"`
}

type GroupRules struct {
    AllMustSucceed       bool   `yaml:"all_must_succeed,omitempty" json:"all_must_succeed,omitempty"`
    MaxCombinedDuration  string `yaml:"max_combined_duration,omitempty" json:"max_combined_duration,omitempty"`
}

type Validation struct {
    MaxDuration string `yaml:"max_duration,omitempty" json:"max_duration,omitempty"`
}
```

**Test requirements:**
- Test graph building with simple linear workflow (A → B → C)
- Test graph building with parallel steps (A → [B, C] → D)
- Test graph building with groups (fraud_validation group)
- Test next[] calculation (reverse dependency lookup)
- Test final step detection
- Test parent group assignment
- Test with contract without groups
- Test error handling (invalid YAML, missing fields)

---

## Task 3: Update Postgres Repository for ContractVersion

**File:** `internal/repository/postgres/contract_version.go` (EXISTING - MODIFY)

**Update schema migration:**
```sql
-- Add graph column to existing contract_versions table
ALTER TABLE contract_versions 
ADD COLUMN graph JSONB;

-- Index for fast graph retrieval
CREATE INDEX idx_contract_versions_graph ON contract_versions 
USING GIN (graph) WHERE graph IS NOT NULL;
```

**Update repository methods (if they don't auto-serialize graph):**
- Ensure `CreateContractVersion()` saves graph field
- Ensure `UpdateContractVersion()` updates graph field
- Ensure `GetContractVersion()` retrieves graph field

**Test requirements:**
- Test inserting ContractVersion with graph
- Test retrieving ContractVersion and deserializing graph
- Test updating graph field
- Test graph field can be NULL (for old records)

---

## Task 4: Contract Service Hook for Graph Generation

**File:** `internal/service/contract.go` (EXISTING - MODIFY)

**Modify existing CreateContractVersion / UpdateContractVersion methods:**

```go
// In CreateContractVersion (existing method)
func (s *ContractService) CreateContractVersion(ctx context.Context, req CreateContractVersionRequest) (*domain.ContractVersion, error) {
    // ... existing validation logic ...
    
    // NEW: Build graph from content
    graphBuilder := NewGraphBuilder()
    contractGraph, err := graphBuilder.BuildGraph(req.ContractName, req.Version, req.Content)
    if err != nil {
        return nil, fmt.Errorf("failed to build contract graph: %w", err)
    }
    
    // Serialize graph to JSON
    graphJSON, err := json.Marshal(contractGraph)
    if err != nil {
        return nil, fmt.Errorf("failed to serialize graph: %w", err)
    }
    
    // Create contract version with graph
    contractVersion := &domain.ContractVersion{
        ContractID:  req.ContractID,
        Version:     req.Version,
        Description: req.Description,
        Content:     req.Content,
        Graph:       graphJSON,  // NEW: Store graph
        CreatedBy:   req.CreatedBy,
        CreatedAt:   time.Now(),
    }
    
    // ... existing save logic ...
    
    return contractVersion, nil
}

// Same modification for UpdateContractVersion
```

**Test requirements:**
- Test CreateContractVersion generates and stores graph
- Test UpdateContractVersion regenerates graph
- Test graph is valid JSON
- Test graph can be deserialized back to ContractGraph struct
- Test existing contract creation tests still pass (green stays green)

---

## Task 5: Define Thread Domain Entity

**File:** `internal/domain/thread.go` (CREATE NEW)

```go
type Thread struct {
    ThreadID        string                 // UUID
    OwnerID         string                 // From API key
    ContractName    string                 // Optional (empty for no-contract threads)
    ContractVersion int                    // 0 if no contract
    ServiceName     string                 // Initiating service
    Creator         string                 // Service/user that created thread
    IsCompleted     bool
    IsCancelled     bool
    CreatedAt       time.Time
    LastEventAt     time.Time
    EventCount      int
    Metadata        map[string]interface{} // JSONB
    
    // Thread state (Redis projection)
    State ThreadState
}

type ThreadState struct {
    CompletedSteps []string `json:"completed_steps"`
    ActiveSteps    []string `json:"active_steps"`
    FailedSteps    []string `json:"failed_steps"`
    ViolatedSteps  []string `json:"violated_steps"`
    WaitingSteps   []string `json:"waiting_steps"`
}

// NewThread creates a new thread with defaults
func NewThread(ownerID, contractName string, contractVersion int, serviceName, creator string) *Thread {
    now := time.Now()
    return &Thread{
        ThreadID:        uuid.New().String(),
        OwnerID:         ownerID,
        ContractName:    contractName,
        ContractVersion: contractVersion,
        ServiceName:     serviceName,
        Creator:         creator,
        IsCompleted:     false,
        IsCancelled:     false,
        CreatedAt:       now,
        LastEventAt:     now,
        EventCount:      0,
        Metadata:        make(map[string]interface{}),
        State: ThreadState{
            CompletedSteps: []string{},
            ActiveSteps:    []string{},
            FailedSteps:    []string{},
            ViolatedSteps:  []string{},
            WaitingSteps:   []string{},
        },
    }
}
```

**Test requirements:**
- Test NewThread generates valid UUID
- Test NewThread sets timestamps correctly
- Test NewThread initializes empty state arrays
- Test thread with contract
- Test thread without contract (empty contractName)

---

## Task 6: Valkey Repository for Threads

**File:** `internal/repository/valkey/thread.go` (CREATE NEW)

**Storage structure:**
```
Key: thread:metadata:{thread_id}
Type: Hash
Fields: thread_id, owner_id, contract_name, contract_version, service_name, 
        creator, is_completed, is_cancelled, created_at, last_event_at, 
        event_count, metadata (JSON string)
TTL: 24 hours (86400 seconds)

Key: thread:state:{thread_id}
Type: Hash  
Fields: completed_steps (JSON array), active_steps, failed_steps, 
        violated_steps, waiting_steps
TTL: 24 hours
```

```go
type ThreadRepository interface {
    // Create thread in Valkey with 24hr TTL
    CreateThread(ctx context.Context, thread *domain.Thread) error
    
    // Get thread from Valkey
    GetThread(ctx context.Context, threadID string) (*domain.Thread, error)
    
    // Check if thread exists in Valkey
    ThreadExists(ctx context.Context, threadID string) (bool, error)
    
    // Reset TTL to 24 hours
    ResetThreadTTL(ctx context.Context, threadID string) error
}

type threadRepository struct {
    client *redis.Client
}

func NewThreadRepository(client *redis.Client) ThreadRepository {
    return &threadRepository{client: client}
}

func (r *threadRepository) CreateThread(ctx context.Context, thread *domain.Thread) error {
    pipe := r.client.Pipeline()
    
    // Serialize metadata
    metadataJSON, err := json.Marshal(thread.Metadata)
    if err != nil {
        return fmt.Errorf("failed to marshal metadata: %w", err)
    }
    
    // Store metadata
    metadataKey := fmt.Sprintf("thread:metadata:%s", thread.ThreadID)
    pipe.HSet(ctx, metadataKey, map[string]interface{}{
        "thread_id":        thread.ThreadID,
        "owner_id":         thread.OwnerID,
        "contract_name":    thread.ContractName,
        "contract_version": thread.ContractVersion,
        "service_name":     thread.ServiceName,
        "creator":          thread.Creator,
        "is_completed":     thread.IsCompleted,
        "is_cancelled":     thread.IsCancelled,
        "created_at":       thread.CreatedAt.Format(time.RFC3339),
        "last_event_at":    thread.LastEventAt.Format(time.RFC3339),
        "event_count":      thread.EventCount,
        "metadata":         string(metadataJSON),
    })
    pipe.Expire(ctx, metadataKey, 24*time.Hour)
    
    // Serialize state arrays
    completedJSON, _ := json.Marshal(thread.State.CompletedSteps)
    activeJSON, _ := json.Marshal(thread.State.ActiveSteps)
    failedJSON, _ := json.Marshal(thread.State.FailedSteps)
    violatedJSON, _ := json.Marshal(thread.State.ViolatedSteps)
    waitingJSON, _ := json.Marshal(thread.State.WaitingSteps)
    
    // Store state
    stateKey := fmt.Sprintf("thread:state:%s", thread.ThreadID)
    pipe.HSet(ctx, stateKey, map[string]interface{}{
        "completed_steps": string(completedJSON),
        "active_steps":    string(activeJSON),
        "failed_steps":    string(failedJSON),
        "violated_steps":  string(violatedJSON),
        "waiting_steps":   string(waitingJSON),
    })
    pipe.Expire(ctx, stateKey, 24*time.Hour)
    
    _, err = pipe.Exec(ctx)
    return err
}

func (r *threadRepository) GetThread(ctx context.Context, threadID string) (*domain.Thread, error) {
    // Get metadata
    metadataKey := fmt.Sprintf("thread:metadata:%s", threadID)
    metadataMap, err := r.client.HGetAll(ctx, metadataKey).Result()
    if err != nil {
        return nil, err
    }
    if len(metadataMap) == 0 {
        return nil, fmt.Errorf("thread not found: %s", threadID)
    }
    
    // Get state
    stateKey := fmt.Sprintf("thread:state:%s", threadID)
    stateMap, err := r.client.HGetAll(ctx, stateKey).Result()
    if err != nil {
        return nil, err
    }
    
    // Deserialize
    thread := &domain.Thread{}
    thread.ThreadID = metadataMap["thread_id"]
    thread.OwnerID = metadataMap["owner_id"]
    thread.ContractName = metadataMap["contract_name"]
    thread.ContractVersion, _ = strconv.Atoi(metadataMap["contract_version"])
    thread.ServiceName = metadataMap["service_name"]
    thread.Creator = metadataMap["creator"]
    thread.IsCompleted, _ = strconv.ParseBool(metadataMap["is_completed"])
    thread.IsCancelled, _ = strconv.ParseBool(metadataMap["is_cancelled"])
    thread.CreatedAt, _ = time.Parse(time.RFC3339, metadataMap["created_at"])
    thread.LastEventAt, _ = time.Parse(time.RFC3339, metadataMap["last_event_at"])
    thread.EventCount, _ = strconv.Atoi(metadataMap["event_count"])
    
    json.Unmarshal([]byte(metadataMap["metadata"]), &thread.Metadata)
    
    // Deserialize state
    json.Unmarshal([]byte(stateMap["completed_steps"]), &thread.State.CompletedSteps)
    json.Unmarshal([]byte(stateMap["active_steps"]), &thread.State.ActiveSteps)
    json.Unmarshal([]byte(stateMap["failed_steps"]), &thread.State.FailedSteps)
    json.Unmarshal([]byte(stateMap["violated_steps"]), &thread.State.ViolatedSteps)
    json.Unmarshal([]byte(stateMap["waiting_steps"]), &thread.State.WaitingSteps)
    
    return thread, nil
}

func (r *threadRepository) ThreadExists(ctx context.Context, threadID string) (bool, error) {
    key := fmt.Sprintf("thread:metadata:%s", threadID)
    exists, err := r.client.Exists(ctx, key).Result()
    return exists > 0, err
}

func (r *threadRepository) ResetThreadTTL(ctx context.Context, threadID string) error {
    pipe := r.client.Pipeline()
    pipe.Expire(ctx, fmt.Sprintf("thread:metadata:%s", threadID), 24*time.Hour)
    pipe.Expire(ctx, fmt.Sprintf("thread:state:%s", threadID), 24*time.Hour)
    _, err := pipe.Exec(ctx)
    return err
}
```

**Test requirements:**
- Test CreateThread stores all fields correctly
- Test CreateThread sets 24hr TTL
- Test GetThread retrieves all fields correctly
- Test GetThread returns error for non-existent thread
- Test ThreadExists returns true for existing thread
- Test ThreadExists returns false for non-existent thread
- Test ResetThreadTTL updates TTL
- Test JSON serialization of metadata and state arrays
- Mock Valkey for unit tests

---

## Task 7: Valkey Repository for Contract Graph Cache

**File:** `internal/repository/valkey/contract.go` (EXISTING - ADD METHODS)

**Storage structure:**
```
Key: contract@{owner_id}:{contract_name}:{version}
Type: String (JSON)
Value: ContractGraph JSON
TTL: 24 hours (refreshed when new thread created)

Key: contract@{owner_id}:{contract_name}:latest
Type: String
Value: Version number
TTL: None (immutable pointer)
```

```go
// ADD to existing ContractRepository interface:

type ContractRepository interface {
    // ... existing methods ...
    
    // Cache contract graph with 24hr TTL
    CacheContractGraph(ctx context.Context, ownerID, contractName string, version int, graphJSON []byte) error
    
    // Get cached contract graph
    GetCachedContractGraph(ctx context.Context, ownerID, contractName string, version int) ([]byte, error)
    
    // Check if contract graph exists in cache
    ContractGraphCached(ctx context.Context, ownerID, contractName string, version int) (bool, error)
    
    // Refresh TTL on contract graph (when new thread created)
    RefreshContractGraphTTL(ctx context.Context, ownerID, contractName string, version int) error
}

// Implementation:

func (r *contractRepository) CacheContractGraph(ctx context.Context, ownerID, contractName string, version int, graphJSON []byte) error {
    key := fmt.Sprintf("contract@%s:%s:%d", ownerID, contractName, version)
    return r.client.Set(ctx, key, graphJSON, 24*time.Hour).Err()
}

func (r *contractRepository) GetCachedContractGraph(ctx context.Context, ownerID, contractName string, version int) ([]byte, error) {
    key := fmt.Sprintf("contract@%s:%s:%d", ownerID, contractName, version)
    return r.client.Get(ctx, key).Bytes()
}

func (r *contractRepository) ContractGraphCached(ctx context.Context, ownerID, contractName string, version int) (bool, error) {
    key := fmt.Sprintf("contract@%s:%s:%d", ownerID, contractName, version)
    exists, err := r.client.Exists(ctx, key).Result()
    return exists > 0, err
}

func (r *contractRepository) RefreshContractGraphTTL(ctx context.Context, ownerID, contractName string, version int) error {
    key := fmt.Sprintf("contract@%s:%s:%d", ownerID, contractName, version)
    return r.client.Expire(ctx, key, 24*time.Hour).Err()
}
```

**Test requirements:**
- Test CacheContractGraph stores graph with 24hr TTL
- Test GetCachedContractGraph retrieves graph
- Test GetCachedContractGraph returns error for non-existent graph
- Test ContractGraphCached returns true/false correctly
- Test RefreshContractGraphTTL extends TTL

---

## Task 8: Postgres Repository for Threads

**File:** `internal/repository/postgres/thread.go` (CREATE NEW)

**Database schema:**
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
    
    -- State arrays (for cold storage queries)
    completed_steps TEXT[],
    active_steps TEXT[],
    failed_steps TEXT[],
    violated_steps TEXT[],
    waiting_steps TEXT[]
);

CREATE INDEX idx_threads_owner ON threads(owner_id);
CREATE INDEX idx_threads_contract ON threads(contract_name, contract_version);
CREATE INDEX idx_threads_created ON threads(created_at DESC);
CREATE INDEX idx_threads_completed ON threads(is_completed, created_at DESC);
```

```go
type ThreadRepository interface {
    // Create thread in Postgres
    CreateThread(ctx context.Context, thread *domain.Thread) error
    
    // Get thread from Postgres (for cold reads)
    GetThread(ctx context.Context, threadID string) (*domain.Thread, error)
    
    // Check if thread exists
    ThreadExists(ctx context.Context, threadID string) (bool, error)
}

type threadRepository struct {
    db *sql.DB
}

func NewThreadRepository(db *sql.DB) ThreadRepository {
    return &threadRepository{db: db}
}

func (r *threadRepository) CreateThread(ctx context.Context, thread *domain.Thread) error {
    metadataJSON, _ := json.Marshal(thread.Metadata)
    
    query := `
        INSERT INTO threads (
            thread_id, owner_id, contract_name, contract_version, 
            service_name, creator, is_completed, is_cancelled,
            created_at, last_event_at, event_count, metadata,
            completed_steps, active_steps, failed_steps, 
            violated_steps, waiting_steps
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 
            $13, $14, $15, $16, $17
        )
    `
    
    _, err := r.db.ExecContext(ctx, query,
        thread.ThreadID,
        thread.OwnerID,
        thread.ContractName,
        thread.ContractVersion,
        thread.ServiceName,
        thread.Creator,
        thread.IsCompleted,
        thread.IsCancelled,
        thread.CreatedAt,
        thread.LastEventAt,
        thread.EventCount,
        metadataJSON,
        pq.Array(thread.State.CompletedSteps),
        pq.Array(thread.State.ActiveSteps),
        pq.Array(thread.State.FailedSteps),
        pq.Array(thread.State.ViolatedSteps),
        pq.Array(thread.State.WaitingSteps),
    )
    
    return err
}

func (r *threadRepository) GetThread(ctx context.Context, threadID string) (*domain.Thread, error) {
    query := `
        SELECT thread_id, owner_id, contract_name, contract_version,
               service_name, creator, is_completed, is_cancelled,
               created_at, last_event_at, event_count, metadata,
               completed_steps, active_steps, failed_steps,
               violated_steps, waiting_steps
        FROM threads
        WHERE thread_id = $1
    `
    
    thread := &domain.Thread{}
    var metadataJSON []byte
    
    err := r.db.QueryRowContext(ctx, query, threadID).Scan(
        &thread.ThreadID,
        &thread.OwnerID,
        &thread.ContractName,
        &thread.ContractVersion,
        &thread.ServiceName,
        &thread.Creator,
        &thread.IsCompleted,
        &thread.IsCancelled,
        &thread.CreatedAt,
        &thread.LastEventAt,
        &thread.EventCount,
        &metadataJSON,
        pq.Array(&thread.State.CompletedSteps),
        pq.Array(&thread.State.ActiveSteps),
        pq.Array(&thread.State.FailedSteps),
        pq.Array(&thread.State.ViolatedSteps),
        pq.Array(&thread.State.WaitingSteps),
    )
    
    if err != nil {
        return nil, err
    }
    
    json.Unmarshal(metadataJSON, &thread.Metadata)
    return thread, nil
}

func (r *threadRepository) ThreadExists(ctx context.Context, threadID string) (bool, error) {
    var exists bool
    query := "SELECT EXISTS(SELECT 1 FROM threads WHERE thread_id = $1)"
    err := r.db.QueryRowContext(ctx, query, threadID).Scan(&exists)
    return exists, err
}
```

**Test requirements:**
- Test CreateThread inserts record
- Test GetThread retrieves record
- Test JSONB metadata serialization
- Test array field handling (completed_steps, etc)
- Test ThreadExists returns correct boolean
- Use test database or mock for unit tests

---

## Task 9: Thread Service (Business Logic)

**File:** `internal/service/thread.go` (CREATE NEW)

**This is the core orchestration layer**

```go
type ThreadService interface {
    CreateThread(ctx context.Context, req CreateThreadRequest) (*domain.Thread, error)
}

type CreateThreadRequest struct {
    OwnerID         string                 // From API key
    ContractName    string                 // Optional
    ContractVersion *int                   // Optional, nil = use latest
    ServiceName     string                 // Required
    Creator         string                 // Required
    Metadata        map[string]interface{} // Optional
}

type threadService struct {
    valkeyThreadRepo    valkey.ThreadRepository
    valkeyContractRepo  valkey.ContractRepository
    postgresThreadRepo  postgres.ThreadRepository
    postgresContractRepo postgres.ContractVersionRepository
}

func NewThreadService(
    valkeyThreadRepo valkey.ThreadRepository,
    valkeyContractRepo valkey.ContractRepository,
    postgresThreadRepo postgres.ThreadRepository,
    postgresContractRepo postgres.ContractVersionRepository,
) ThreadService {
    return &threadService{
        valkeyThreadRepo:     valkeyThreadRepo,
        valkeyContractRepo:   valkeyContractRepo,
        postgresThreadRepo:   postgresThreadRepo,
        postgresContractRepo: postgresContractRepo,
    }
}

func (s *threadService) CreateThread(ctx context.Context, req CreateThreadRequest) (*domain.Thread, error) {
    // 1. Validate input
    if req.OwnerID == "" || req.ServiceName == "" || req.Creator == "" {
        return nil, fmt.Errorf("owner_id, service_name, and creator are required")
    }
    
    // 2. Determine contract version
    contractVersion := 0
    if req.ContractName != "" {
        if req.ContractVersion == nil {
            // Get latest version from Postgres
            latestVersion, err := s.postgresContractRepo.GetLatestVersion(ctx, req.OwnerID, req.ContractName)
            if err != nil {
                return nil, fmt.Errorf("failed to get latest contract version: %w", err)
            }
            contractVersion = latestVersion
        } else {
            contractVersion = *req.ContractVersion
        }
        
        // 3. Ensure contract graph is cached in Valkey
        cached, err := s.valkeyContractRepo.ContractGraphCached(ctx, req.OwnerID, req.ContractName, contractVersion)
        if err != nil {
            return nil, fmt.Errorf("failed to check contract cache: %w", err)
        }
        
        if !cached {
            // Load from Postgres and cache
            contractVersionEntity, err := s.postgresContractRepo.GetContractVersion(ctx, req.OwnerID, req.ContractName, contractVersion)
            if err != nil {
                return nil, fmt.Errorf("contract version not found: %w", err)
            }
            
            if contractVersionEntity.Graph == nil {
                return nil, fmt.Errorf("contract version has no graph (this should not happen)")
            }
            
            // Cache in Valkey with 24hr TTL
            err = s.valkeyContractRepo.CacheContractGraph(ctx, req.OwnerID, req.ContractName, contractVersion, contractVersionEntity.Graph)
            if err != nil {
                return nil, fmt.Errorf("failed to cache contract graph: %w", err)
            }
        } else {
            // Refresh TTL (new thread using this contract)
            err = s.valkeyContractRepo.RefreshContractGraphTTL(ctx, req.OwnerID, req.ContractName, contractVersion)
            if err != nil {
                // Log warning but don't fail (TTL refresh is not critical)
                log.Warn().Err(err).Msg("failed to refresh contract graph TTL")
            }
        }
    }
    
    // 4. Create thread entity
    thread := domain.NewThread(
        req.OwnerID,
        req.ContractName,
        contractVersion,
        req.ServiceName,
        req.Creator,
    )
    
    if req.Metadata != nil {
        thread.Metadata = req.Metadata
    }
    
    // 5. Save to Valkey (hot storage with 24hr TTL)
    err := s.valkeyThreadRepo.CreateThread(ctx, thread)
    if err != nil {
        return nil, fmt.Errorf("failed to save thread to Valkey: %w", err)
    }
    
    // 6. Save to Postgres (durable storage)
    err = s.postgresThreadRepo.CreateThread(ctx, thread)
    if err != nil {
        // Log error but don't fail (Postgres write is async/eventual)
        log.Error().Err(err).Str("thread_id", thread.ThreadID).Msg("failed to save thread to Postgres")
    }
    
    return thread, nil
}
```

**Test requirements:**
- Test thread creation with contract
- Test thread creation without contract
- Test contract version defaults to latest
- Test contract version explicit
- Test contract graph cache hit (cached in Valkey)
- Test contract graph cache miss → Load from Postgres → Cache
- Test contract graph TTL refresh on cache hit
- Test both Valkey and Postgres repositories called
- Test Valkey write failure returns error
- Test Postgres write failure logs but doesn't fail request
- Test input validation (missing required fields)
- Test metadata is stored correctly
- Mock all repositories for unit tests

---

## Task 10: WebSocket Handler - HandleStartThread

**File:** `internal/transport/websocket/thread.go` (CREATE NEW)

**WebSocket message format (incoming from SDK):**
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

**WebSocket response format (outgoing to SDK):**
```json
{
  "message_id": "msg_abc123",
  "success": true,
  "data": {
    "thread_id": "thread_abc123",
    "contract_name": "payment-flow-v1",
    "contract_version": 1,
    "service_name": "merchant-service",
    "creator": "merchant-service",
    "is_completed": false,
    "created_at": "2024-12-13T10:00:00Z"
  }
}
```

**Error response:**
```json
{
  "message_id": "msg_abc123",
  "success": false,
  "error": "contract version not found"
}
```

**WebSocket connection structure (already exists in main.go):**
- Each WebSocket connection has authenticated API key
- Connection object has `GetAPIKey()` method
- Connection object has `Send(message)` method

```go
type WSMessage struct {
    MessageID string                 `json:"message_id"`
    Action    string                 `json:"action"`
    Payload   map[string]interface{} `json:"payload"`
}

type WSResponse struct {
    MessageID string      `json:"message_id"`
    Success   bool        `json:"success"`
    Data      interface{} `json:"data,omitempty"`
    Error     string      `json:"error,omitempty"`
}

type ThreadHandler struct {
    threadService service.ThreadService
}

func NewThreadHandler(threadService service.ThreadService) *ThreadHandler {
    return &ThreadHandler{
        threadService: threadService,
    }
}

// HandleStartThread processes "start_thread" action from WebSocket
func (h *ThreadHandler) HandleStartThread(conn *websocket.Connection, msg WSMessage) {
    // 1. Extract API key from connection
    apiKey := conn.GetAPIKey()
    if apiKey == "" {
        h.sendError(conn, msg.MessageID, "unauthorized: missing API key")
        return
    }
    
    // 2. Derive owner_id from API key (assuming API key has format: owner_id:key)
    ownerID := extractOwnerIDFromAPIKey(apiKey)
    
    // 3. Parse payload
    contractName, _ := msg.Payload["contract_name"].(string)
    serviceName, ok := msg.Payload["service_name"].(string)
    if !ok || serviceName == "" {
        h.sendError(conn, msg.MessageID, "service_name is required")
        return
    }
    
    var contractVersion *int
    if v, ok := msg.Payload["contract_version"].(float64); ok {
        version := int(v)
        contractVersion = &version
    }
    
    metadata, _ := msg.Payload["metadata"].(map[string]interface{})
    
    // 4. Create thread via service
    thread, err := h.threadService.CreateThread(context.Background(), service.CreateThreadRequest{
        OwnerID:         ownerID,
        ContractName:    contractName,
        ContractVersion: contractVersion,
        ServiceName:     serviceName,
        Creator:         serviceName, // Creator = service_name for now
        Metadata:        metadata,
    })
    
    if err != nil {
        h.sendError(conn, msg.MessageID, fmt.Sprintf("failed to create thread: %v", err))
        return
    }
    
    // 5. Send success response
    h.sendSuccess(conn, msg.MessageID, map[string]interface{}{
        "thread_id":        thread.ThreadID,
        "contract_name":    thread.ContractName,
        "contract_version": thread.ContractVersion,
        "service_name":     thread.ServiceName,
        "creator":          thread.Creator,
        "is_completed":     thread.IsCompleted,
        "created_at":       thread.CreatedAt.Format(time.RFC3339),
    })
}

func (h *ThreadHandler) sendSuccess(conn *websocket.Connection, messageID string, data interface{}) {
    response := WSResponse{
        MessageID: messageID,
        Success:   true,
        Data:      data,
    }
    conn.Send(response)
}

func (h *ThreadHandler) sendError(conn *websocket.Connection, messageID string, errMsg string) {
    response := WSResponse{
        MessageID: messageID,
        Success:   false,
        Error:     errMsg,
    }
    conn.Send(response)
}

func extractOwnerIDFromAPIKey(apiKey string) string {
    // Assuming API key format: "owner_abc123:key_xyz789"
    parts := strings.Split(apiKey, ":")
    if len(parts) > 0 {
        return parts[0]
    }
    return ""
}
```

**Test requirements:**
- Test HandleStartThread with valid payload
- Test response contains thread_id
- Test missing service_name returns error
- Test contract_version parsing (int from JSON float)
- Test metadata is passed through
- Test API key extraction and owner_id derivation
- Test error response format
- Test success response format
- Mock ThreadService for unit tests
- Mock WebSocket connection for unit tests

---

## Task 11: Wire Handler into WebSocket Router

**File:** `cmd/server/main.go` (EXISTING - MODIFY)

**Assuming WebSocket router already exists, add thread handler:**

```go
// In main() function, after WebSocket server initialization:

// Initialize repositories
valkeyThreadRepo := valkey.NewThreadRepository(valkeyClient)
valkeyContractRepo := valkey.NewContractRepository(valkeyClient)
postgresThreadRepo := postgres.NewThreadRepository(postgresDB)
postgresContractVersionRepo := postgres.NewContractVersionRepository(postgresDB)

// Initialize services
threadService := service.NewThreadService(
    valkeyThreadRepo,
    valkeyContractRepo,
    postgresThreadRepo,
    postgresContractVersionRepo,
)

// Initialize handlers
threadHandler := websocket.NewThreadHandler(threadService)

// Register with WebSocket router
wsRouter.Handle("start_thread", threadHandler.HandleStartThread)
// Assuming wsRouter has Handle(action string, handler func(*Connection, WSMessage))
```

**Test requirements:**
- Integration test: Send "start_thread" via WebSocket → Thread created in Valkey
- Integration test: Send "start_thread" via WebSocket → Thread created in Postgres
- Integration test: Contract graph cached in Valkey after thread creation
- Integration test: Contract graph TTL refreshed if already cached

---

## Task 12: Custom Errors

**File:** `pkg/errors/errors.go` (MODIFY EXISTING)

**Add thread-specific errors:**

```go
var (
    ErrThreadAlreadyExists     = errors.New("thread already exists")
    ErrThreadNotFound          = errors.New("thread not found")
    ErrInvalidContractVersion  = errors.New("invalid contract version")
    ErrContractNotFound        = errors.New("contract not found")
    ErrContractGraphNotFound   = errors.New("contract graph not found")
    ErrValkeyConnectionFailed  = errors.New("valkey connection failed")
    ErrPostgresConnectionFailed = errors.New("postgres connection failed")
    ErrMissingRequiredField    = errors.New("missing required field")
)
```

---

## Implementation Order (TDD)

**Follow this order, writing tests first for each:**

1. **Update ContractVersion domain** (add graph field) + tests
2. **Graph building logic** (contract_graph.go) + comprehensive tests
3. **Update ContractService** (generate graph on create/update) + tests
4. **Update Postgres ContractVersion repo** (store graph) + tests + migration
5. **Thread domain entity** (thread.go) + tests
6. **Valkey thread repository** + tests
7. **Valkey contract repository** (cache methods) + tests
8. **Postgres thread repository** + tests + migration
9. **Thread service** (business logic) + tests
10. **WebSocket handler** (HandleStartThread) + tests
11. **Wire into main.go** + integration tests

---

## Success Criteria

✅ All unit tests pass (green)
✅ All integration tests pass
✅ Contract graph generated on contract create/update
✅ Contract graph cached in Valkey when thread created
✅ Contract graph TTL refreshed when reused
✅ Thread created in both Valkey (hot) and Postgres (cold)
✅ Thread has 24hr TTL in Valkey
✅ WebSocket "start_thread" creates thread successfully
✅ WebSocket returns thread_id and metadata
✅ No-contract threads work (contract_name empty)
✅ Contract version defaults to latest if not specified
✅ Code follows clean architecture (separated layers)
✅ Code is concise, no over-abstraction
✅ Existing contract CRUD code still works (no regression)

---

**Start with Task 1. Write failing tests first, then implement to make them green. Then check that all test that where green still stayed green **

** Follow the architecture that I told you to use in the prompt **

** Don't complicae it, and any one you're not sure how to modify, ask me. Don't try to force your way into correction. **