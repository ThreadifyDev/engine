# Threadify Go SDK - Syntax Guide

> **Prerequisites:** Read [AI.md](https://threadify.dev/AI.md) for core concepts.

This file contains **Go-specific syntax only**. For concepts, see AI.md.

Threadify follows work your services and agents already perform. Bind a contract
when rules are needed; use recorded steps for evidence and explicitly await a
permission check where the application must wait before acting. Recording
telemetry alone does not stop actions or guarantee a rule passed.

---

## Installation

```bash
go get github.com/ThreadifyDev/go-sdk
```

## Import

```go
import (
    "context"
    "github.com/ThreadifyDev/go-sdk"
)
```

---

## Syntax Reference

### Connect
```go
ctx := context.Background()
conn, err := threadify.Connect(ctx, "api-key", threadify.WithServiceName("my-service"))
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// With options
conn, err := threadify.Connect(ctx, "api-key",
    threadify.WithServiceName("my-service"),
    threadify.WithWSURL("wss://your-threadify-engine.example/threads"),
    threadify.WithDebug(true),
)
```

### Create or Resume a Thread

```go
// Initialize once before requests or telemetry use this session.
thread, err := conn.Thread(ctx, sessionID, threadify.ThreadOptions{
    Label:    "Agent session",
    Contract: "agent_contract:3", // Omit for a free-form session.
    Refs:     map[string]string{"customer_id": customerID},
    Tags:     []string{"production"},
})
if err != nil {
    log.Fatal(err)
}

// A later request or worker uses only the application's session ID.
resumed, err := conn.Thread(ctx, sessionID)
if err != nil {
    log.Fatal(err)
}
```

`ThreadOptions` fields: `Label`, `Contract`, `Refs` (`map[string]string`), `Tags`,
`ServiceName`, and `Role`. Use `thread.AddRefs(ctx, refs)` for explicit reference updates.

The key identifies one workflow or agent session within your company; Threadify
manages the internal `threadId`. Reuse the same key across requests and workers.
Keys are trimmed, nonblank strings of at most 1024 UTF-8 bytes. Concurrent calls
resolve to one thread and normal write permissions still apply.

The optional object supplies creation defaults. Resuming loads the stored contract
and pinned version without redefining them. A conflicting contract or version is
rejected. Existing labels, refs, and tags are preserved; update refs explicitly
with the SDK's reference method.

A new key with no options creates a free-form thread. Initialize contracted
sessions before their turns or telemetry begin: a free-form thread cannot acquire
a contract on resume. Closed threads cannot resume or accept writes; choose a new
key for a new execution. Set `threadify.thread_key` to this same key in OTLP spans
or `ThreadifySpanExporter` instrumentation.

### Record Step
```go
result, err := thread.Step("order_placed").
    AddContext(map[string]any{"order_id": "123", "amount": 99.99}).
    Success(ctx)
if err != nil {
    log.Fatal(err)
}
```

### Add Context
```go
.AddContext(map[string]any{
    "key": "value",
    "another_key": "another_value",
})
```

### Set Idempotency Key

**Manual idempotency key** (use external system IDs):
```go
// Using payment provider transaction ID
result, err := thread.Step("charge_payment").
    IdempotencyKey(payment.ID).  // e.g., "pi_3ABC123"
    AddContext(map[string]any{"amount": 99.99}).
    Success(ctx)
if err != nil {
    log.Fatal(err)
}

// Using user-initiated retry with request ID
result, err := thread.Step("retry_payment").
    IdempotencyKey(requestID).
    AddContext(map[string]any{"attempt": 2}).
    Success(ctx)
if err != nil {
    log.Fatal(err)
}
```

**Auto-generated** (default - no `.IdempotencyKey()` call):
```go
// SDK generates hash from stepName + context
result, err := thread.Step("validate_cart").
    AddContext(map[string]any{"items": 3, "total": 99.99}).
    Success(ctx)
if err != nil {
    log.Fatal(err)
}
// Idempotency key auto-generated from: 'validate_cart' + '{"items":"3","total":"99.99"}'
```

### Add Sub-Steps
```go
// Add sub-steps to track granular operations within a step
result, err := thread.Step("payment_processed").
    SubStep("validate_card", map[string]any{"cardType": "visa"}, "success").
    SubStep("check_fraud", map[string]any{"fraudScore": 0.15}, "success").
    SubStep("authorize_payment", map[string]any{"authCode": "AUTH-123"}, "success").
    AddContext(map[string]any{"totalAmount": 299.99}).
    Success(ctx)
if err != nil {
    log.Fatal(err)
}

// Sub-step status can be "success" or "failed" (optional, defaults to "success")
.SubStep("error_handler", map[string]any{"error": "timeout"}, "failed")
```

### Step Status
```go
.Success(ctx)
.Success(ctx, "Order placed successfully")
.Success(ctx, map[string]any{"message": "Order placed", "order_id": "ORD-123"})

.Failed(ctx)
.Failed(ctx, "Payment declined")
.Failed(ctx, map[string]any{"error": "Payment declined", "code": "DECLINED"})

.Error(ctx)
.Error(ctx, "Service unavailable")
.Error(ctx, map[string]any{"error": "Timeout", "service": "payment-api"})
```

### Add External References

**Thread-level references** (called on thread object):

```go
// Add references to the thread
err := thread.AddRefs(ctx, map[string]string{
    "payment_id": "pi_123",
    "order_id": "ORD-456",
})
if err != nil {
    log.Fatal(err)
}
```

### Link Threads
```go
err := childThread.LinkThread(ctx, parentThread.ThreadID, "parent")
if err != nil {
    log.Fatal(err)
}

// Default relationship is "parent" if not specified
err := childThread.LinkThread(ctx, parentThread.ThreadID, "")
```

### Add Private Context
```go
// Private context is prefixed with "private_" and excluded from certain queries
result, err := thread.Step("process_payment").
    AddPrivateContext(map[string]any{
        "card_number": "4111111111111111",
        "cvv": "123",
    }).
    AddContext(map[string]any{"amount": 99.99}).
    Success(ctx)
if err != nil {
    log.Fatal(err)
}
```

### Query Thread Chain
```go
// Wait for archival (1-2 seconds)
time.Sleep(2 * time.Second)

// Query from any thread in chain
chain, err := conn.GetThreadChain(ctx, startThreadID, 3)
if err != nil {
    log.Fatal(err)
}

for _, t := range chain {
    fmt.Println("Thread ID:", t.ID)
}
```

### Query Threads by Reference
```go
// Find threads by external reference
threads, err := conn.GetThreadsByRef(ctx, &threadify.RefQuery{
    RefKey:   "order_id",
    RefValue: "ORD-12345",
    Status:   "completed", // Optional filter
    Limit:    10,
})
if err != nil {
    log.Fatal(err)
}

for _, thread := range threads {
    fmt.Println("Thread:", thread.ID, "Status:", thread.Status)
}
```

### Retrieve Thread Data

**Important:** `GetThread()` returns a **read-only** thread object for querying data. To add steps or modify a thread, resume with `Thread(ctx, threadKey)` or use `Join()` with an internal ID or invitation.

**Recommended:** Use `GetCompleteData()` for efficiency (single query):

```go
// Wait for archival (1-2 seconds)
time.Sleep(2 * time.Second)

// Get thread for READ-ONLY access
thread, err := conn.GetThread(ctx, threadID)
if err != nil {
	log.Fatal(err)
}

// Get everything in one query (recommended)
completeData, err := thread.GetCompleteData(ctx, &threadify.devpleteDataOptions{
	StepHistoryLimit: 50,  // History per step
	ValidationLimit:  10,  // Validation results
})
if err != nil {
	log.Fatal(err)
}

// Access: completeData.Steps, completeData.ValidationResults, etc.
```

**Alternative:** Separate queries (use only if you need partial data):

```go
// Read-only access
thread, err := conn.GetThread(ctx, threadID)
if err != nil {
	log.Fatal(err)
}

// Get steps only
steps, err := thread.Steps(ctx, "order_placed", "", "success")
if err != nil {
	log.Fatal(err)
}

// Get validations only
validations, err := thread.ValidationResults(ctx, 10)
if err != nil {
	log.Fatal(err)
}
```

**To modify a thread:** Use `Join()` instead:

```go
// Join thread to add steps
thread, err := conn.Join(ctx, 
	threadify.WithJoinThreadID(threadID),
	threadify.WithJoinRole("participant"),
)
if err != nil {
	log.Fatal(err)
}

// Now you can record steps
_, err = thread.Step("new_step").Success(ctx)
if err != nil {
	log.Fatal(err)
}
```

### Subscribe to Events
```go
err := conn.Subscribe(ctx, "step.success", "order_placed", func(n *threadify.Notification) {
    fmt.Println("Order placed:", n.StepName)
    
    // Check notification properties
    if n.IsSuccess() {
        fmt.Println("Step succeeded")
    }
    if n.IsCritical() {
        fmt.Println("Critical notification!")
    }
    
    // Always acknowledge
    n.Ack()
})
if err != nil {
    log.Fatal(err)
}

// For contracted threads: Subscribe to rule-check results
err = conn.Subscribe(ctx, "rule.violated", "payment_processed", func(n *threadify.Notification) {
    fmt.Println("Violation:", n.Severity)

    // Check violation status
    if n.IsViolated() && n.IsCritical() {
        // Handle critical violation
    }

    n.Ack()
})
if err != nil {
    log.Fatal(err)
}

// Unsubscribe when done
defer conn.Unsubscribe(ctx, "step.success", "order_placed")
```

### Notification Helper Methods
```go
// Check severity (for contract validation events)
if n.IsCritical() { /* critical severity */ }
if n.IsWarning() { /* warning severity */ }
if n.IsInfo() { /* info severity */ }

// Check step status
if n.IsSuccess() { /* step succeeded */ }
if n.IsFailed() { /* step failed */ }
if n.IsError() { /* step errored */ }

// Check if already acknowledged
if !n.IsAcknowledged() {
    n.Ack()
}

// Convert to string
fmt.Println(n.String()) // "[critical] order_placed: Payment validation failed"
```

### Join Thread
```go
// With token (accessLevel comes from the invitation)
thread, err := conn.Join(ctx, threadify.WithJoinToken(invitationToken))
if err != nil {
    log.Fatal(err)
}

// Direct join (defaults to participant accessLevel)
thread, err := conn.Join(ctx, 
    threadify.WithJoinThreadID(threadID),
    threadify.WithJoinRole("supplier"),
)
if err != nil {
    log.Fatal(err)
}
```

### Invite Parties
```go
// Create invitation for external party (default)
invitation, err := thread.InviteParty(ctx, threadify.InviteOptions{
    Role:        "supplier",
    AccessLevel: threadify.ForExternal,  // Optional: ForExternal (default), ForObserver, ForParticipant
    ExpiresIn:   "48h",                 // Optional, defaults to "24h"
})
if err != nil {
    log.Fatal(err)
}

// Invite as observer (read-only)
invitation, err := thread.InviteParty(ctx, threadify.InviteOptions{
    Role:        "supplier",
    AccessLevel: threadify.ForObserver,
})

// Invite as participant (active)
invitation, err := thread.InviteParty(ctx, threadify.InviteOptions{
    Role:        "inventory-service",
    AccessLevel: threadify.ForParticipant,
})

// Share invitation token
fmt.Println("Token:", invitation.Token)
fmt.Println("Expires at:", invitation.ExpiresAt)
```

### Thread Lifecycle Management
```go
// Complete thread successfully
resp, err := thread.Complete(ctx, "Order fulfilled")
if err != nil {
    log.Fatal(err)
}
fmt.Println("Thread completed at:", resp.EndedAt)

// Close/cancel thread
resp, err := thread.Close(ctx, "User cancelled order")
if err != nil {
    log.Fatal(err)
}

// End thread with custom status
resp, err := thread.End(ctx, "custom_status", "Custom reason")
if err != nil {
    log.Fatal(err)
}
```

### Error Handling
```go
err := processPayment()
if err != nil {
    _, stepErr := thread.Step("process_payment").
        AddContext(map[string]any{"error": err.Error()}).
        Failed(ctx)
    if stepErr != nil {
        log.Fatal(stepErr)
    }
} else {
    _, stepErr := thread.Step("process_payment").Success(ctx)
    if stepErr != nil {
        log.Fatal(stepErr)
    }
}
```

### OpenTelemetry Integration — ONLY for Existing OTel Codebases

**Default to manual instrumentation.** Only use OTel if the user explicitly asks OR the codebase already imports `go.opentelemetry.io/otel`.

Because Go is statically typed, the OTel integration requires its own sub-module:

**Install:**
```bash
go get github.com/ThreadifyDev/go-sdk/otel
```

**Usage (ONLY if codebase already uses OTel):**
```go
import (
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"

    threadify "github.com/ThreadifyDev/go-sdk"
    threadifyotel "github.com/ThreadifyDev/go-sdk/otel"
)

// 1. Connect to Threadify
conn, _ := threadify.Connect(ctx, "api-key")

// 2. Create the Exporter
exporter := threadifyotel.NewSpanExporter(conn, threadifyotel.SpanExporterOptions{
    Refs: []string{"rider.id"},
})

// Tag threads via OTel span attributes
span.SetAttributes(attribute.StringSlice("threadify.tags", []string{"production", "v2.1"}))

// Filter spans by name — exact match or prefix wildcard with *
exporter := threadifyotel.NewSpanExporter(conn, threadifyotel.SpanExporterOptions{
    Refs:    []string{"rider.id"},
    Filters: []string{"invoke_llm", "adk.before*", "llm.*"},
})

// 3. Register with OpenTelemetry
provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
otel.SetTracerProvider(provider)
```

---

## Common Mistakes

### ❌ Wrong: `.Context()`
```go
.Context(map[string]any{"data": "value"})  // Method doesn't exist!
```

### ✅ Correct: `.AddContext()`
```go
.AddContext(map[string]any{"data": "value"})
```

### ❌ Wrong: Nested objects
```go
.AddContext(map[string]any{
    "user": map[string]any{"id": 1, "name": "John"},
})
```

### ✅ Correct: Flat structure
```go
.AddContext(map[string]any{
    "user_id": 1,
    "user_name": "John",
})
```

### ❌ Wrong: Forgetting context.Context
```go
thread.Step("order_placed").Success()  // Missing ctx parameter!
```

### ✅ Correct: Always pass context
```go
thread.Step("order_placed").Success(ctx)
```

### ❌ Wrong: Ignoring errors
```go
thread.Step("order_placed").Success(ctx)  // Error not checked!
```

### ✅ Correct: Check errors
```go
_, err := thread.Step("order_placed").Success(ctx)
if err != nil {
    log.Fatal(err)
}
```

---

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/ThreadifyDev/go-sdk"
)

func main() {
    ctx := context.Background()
    
    conn, _ := threadify.Connect(ctx, "api-key",
        threadify.WithServiceName("checkout-service"),
    )
    defer conn.Close()

    thread, _ := conn.Thread(ctx, "order:ORD-789", threadify.ThreadOptions{Label: "Checkout Process"})

    thread.AddRefs(ctx, map[string]string{
        "customer_id": "123",
        "order_id": "ORD-789",
    })

    thread.Step("validate_cart").
        AddContext(map[string]any{"items": 3, "total": 99.99}).
        Success(ctx)

    payment, err := processPayment()
    if err != nil {
        thread.Step("charge_payment").
            AddContext(map[string]any{"error": err.Error()}).
            Failed(ctx)
        return
    }

    thread.AddRefs(ctx, map[string]string{
        "payment_id": payment.ID,
    })

    thread.Step("charge_payment").
        AddContext(map[string]any{"amount": 99.99, "method": "card"}).
        Success(ctx)
}

func processPayment() (*Payment, error) {
    // Payment processing logic
    return &Payment{ID: "pi_123"}, nil
}

type Payment struct {
    ID string
}
```
