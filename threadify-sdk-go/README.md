# Threadify Go SDK

The official Go SDK for connecting to the Threadify Engine.

## Installation

```bash
go get github.com/threadify/threadify-sdk-go
```

## Quick Start

### 1. Connect to the Engine

Use `threadify.Connect` to establish a connection. You can configure the connection using Functional Options.

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/threadify/threadify-sdk-go"
)

func main() {
    ctx := context.Background()

    // Connect with options
    conn, err := threadify.Connect(ctx, "your-api-key",
        threadify.WithServiceName("my-service"),
        threadify.WithDebug(true),
        threadify.WithConnectTimeout(5*time.Second),
    )
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
}
```

### 2. Start a New Thread

Start a thread, optionally associating it with a contract.

```go
// Start a generic thread
thread, err := conn.Start(ctx)

// Start a thread for a specific contract
thread, err := conn.Start(ctx, 
    threadify.WithContract("order_processing"),
)
```

### 3. Join an Existing Thread

You can join a thread by its ID or using a secure token.

```go
// Join by ID
thread, err := conn.Join(ctx, 
    threadify.WithJoinThreadID("thread-123"), 
    threadify.WithJoinRole("logistics"),
)

// Join by Token
thread, err := conn.Join(ctx, 
    threadify.WithJoinToken("ey..."),
)
```

### 4. Record Steps

Record steps in a thread's lifecycle. You can add context, references, and sub-steps.

```go
step, _ := thread.Step("order_shipped")

_, err = step.
    AddContext(map[string]any{
        "trackingNumber": "TRK123456",
        "carrier":        "FedEx",
    }).
    AddRefs(map[string]string{
        "orderId": "ORD-999",
    }).
    Success(ctx, "Order has been shipped successfully")
```

## Event Subscriptions

Listen for real-time events from the engine.

```go
// Subscribe to 'step.success' events for the 'order_placed' step
err := conn.Subscribe(ctx, "step.success", "order_placed", func(n *threadify.Notification) {
    log.Printf("Order placed: %s", n.ThreadID)
    
    // Acknowledge receipt
    n.Ack()
})

// Unsubscribe when done
defer conn.Unsubscribe(ctx, "step.success", "order_placed")
```

### Event Patterns

| Pattern | Description |
| :--- | :--- |
| `step.success` | Step completed successfully |
| `step.failed` | Step failed |
| `step.*` | Any step execution event |
| `rule.violated` | Validation rule violated |
| `rule.passed` | Validation rule passed |
| `*` | All events |

## Functional Options

The SDK uses the Functional Option pattern for configuration.

### Connection Options

-   `WithServiceName(string)`: Set the service name (default: "default").
-   `WithWSURL(string)`: Set the WebSocket URL.
-   `WithDebug(bool)`: Enable debug logging.
-   `WithConnectTimeout(time.Duration)`: Set the connection timeout.
-   `WithMaxInFlight(int)`: Set the maximum number of concurrent requests.

### Join Options

-   `WithJoinThreadID(string)`: Join by Thread ID.
-   `WithJoinRole(string)`: Set the role when joining by ID.
-   `WithJoinToken(string)`: Join using a secure invitation token.
