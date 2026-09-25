# Go SDK fallback

Bundled from the Threadify Go SDK and documentation. Confirm exact signatures
with `get_developer_reference` when it is available.

```sh
go get github.com/ThreadifyDev/go-sdk
```

```go
package main

import (
    "context"
    "os"

    threadify "github.com/ThreadifyDev/go-sdk"
)

func main() {
    ctx := context.Background()
    conn, err := threadify.Connect(ctx, os.Getenv("THREADIFY_API_KEY"),
        threadify.WithServiceName("orders-service"),
        threadify.WithEngineURL("https://threadify.example.com"))
    if err != nil { panic(err) }
    defer conn.Close()

    thread, err := conn.Thread(ctx, "order:ORD-123", threadify.ThreadOptions{
        Label: "Order ORD-123",
        Contract: "order_processing",
        Refs: map[string]string{"order_id": "ORD-123"},
    })
    if err != nil { panic(err) }
    _, err = thread.Step("order_received").
        AddContext(map[string]any{"order_id": "ORD-123"}).
        Success(ctx, "Order accepted")
    if err != nil { panic(err) }
}
```

Use the application's stable key to resume the same active thread. Go SDK 0.4
and later adds `WaitFor` and `WaitForValidation` for contract waits.
