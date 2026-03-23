package main

import (
	"context"
	"log"
	"time"

	threadify "github.com/ThreadifyDev/go-sdk"
)

const (
	apiKey = "td_5e5VKClYIPMJG1qrAzHYPn-tbJ8skoAjBDHuieUxfnE"
	wsURL  = "ws://localhost:8081/threads"
	gqlURL = "http://localhost:8081/graphql"
)

func main() {
	ctx := context.Background()

	log.Println("── Starting single thread with multiple steps ──")

	conn, err := threadify.Connect(ctx, apiKey,
		threadify.WithServiceName("test-service"),
		threadify.WithWSURL(wsURL),
		threadify.WithGraphQLURL(gqlURL),
	)
	if err != nil {
		log.Fatal("connect:", err)
	}
	defer conn.Close()

	// Give the user time to start the WebSocket script
	// log.Println("Sleeping for 3 seconds...")
	// time.Sleep(3 * time.Second)

	thread, err := conn.Start(ctx)
	if err != nil {
		log.Fatal("start thread:", err)
	}
	log.Println("Thread ID:", thread.ThreadID)
	log.Println("⏳ Waiting 30 seconds for you to start the WebSocket subscription...")
	log.Println("   Run: node test-graphql-subscription.js", thread.ThreadID)
	time.Sleep(30 * time.Second)

	// Attach business refs
	err = thread.AddRefs(ctx, map[string]string{
		"orderId":    "ORD-20260309-001",
		"customerId": "CUST-88821",
		"region":     "eu-west-1",
	})
	if err != nil {
		log.Fatal("add refs:", err)
	}

	// Step 1: order_placed
	_, err = thread.Step("order_placed").
		AddContext(map[string]any{
			"items":        []string{"SKU-A", "SKU-B", "SKU-C"},
			"total_amount": 149.99,
			"currency":     "EUR",
		}).
		Success(ctx)
	if err != nil {
		log.Fatal("order_placed step:", err)
	}
	log.Println("✓ order_placed")
	time.Sleep(1 * time.Second)

	// Step 2: payment_authorised
	_, err = thread.Step("payment_authorised").
		AddContext(map[string]any{
			"payment_method":     "card",
			"authorisation_code": "AUTH-XK99",
		}).
		Success(ctx)
	if err != nil {
		log.Fatal("payment_authorised step:", err)
	}
	log.Println("✓ payment_authorised")
	time.Sleep(1 * time.Second)

	// Step 3: items_picked
	_, err = thread.Step("items_picked").
		AddContext(map[string]any{
			"picker_id": "EMP-4421",
			"items":     []string{"SKU-A", "SKU-B", "SKU-C"},
			"picked_at": time.Now().Format(time.RFC3339),
		}).
		Success(ctx)
	if err != nil {
		log.Fatal("items_picked step:", err)
	}
	log.Println("✓ items_picked")
	time.Sleep(1 * time.Second)

	// Step 4: order_packed
	_, err = thread.Step("order_packed").
		AddContext(map[string]any{
			"box_id":    "BOX-7812",
			"weight_kg": 1.4,
			"packed_at": time.Now().Format(time.RFC3339),
		}).
		Success(ctx)
	if err != nil {
		log.Fatal("order_packed step:", err)
	}
	log.Println("✓ order_packed")
	time.Sleep(1 * time.Second)

	// Step 5: shipment_collected
	_, err = thread.Step("shipment_collected").
		AddContext(map[string]any{
			"courier_id":         "DHL-EXP",
			"tracking_code":      "1Z99911234567890",
			"collected_at":       time.Now().Format(time.RFC3339),
			"estimated_delivery": time.Now().Add(48 * time.Hour).Format(time.RFC3339),
		}).
		Success(ctx)
	if err != nil {
		log.Fatal("shipment_collected step:", err)
	}
	log.Println("✓ shipment_collected")
	time.Sleep(1 * time.Second)

	// Step 6: out_for_delivery
	_, err = thread.Step("out_for_delivery").
		AddContext(map[string]any{
			"driver_id":   "DRV-9087",
			"vehicle":     "VAN-LU-441",
			"departed_at": time.Now().Format(time.RFC3339),
		}).
		Success(ctx)
	if err != nil {
		log.Fatal("out_for_delivery step:", err)
	}
	log.Println("✓ out_for_delivery")
	time.Sleep(1 * time.Second)

	// Complete thread
	log.Println("\n── Completing thread ──")
	_, err = thread.Complete(ctx, "order fulfilled and dispatched")
	if err != nil {
		log.Fatal("complete thread:", err)
	}
	log.Println("✓ thread completed")

	// Query thread data
	log.Println("\n── Querying via GraphQL ──")
	time.Sleep(2 * time.Second) // allow archival

	queriedThread, err := conn.GetThread(ctx, thread.ThreadID)
	if err != nil {
		log.Fatal("get thread:", err)
	}

	completeData, err := queriedThread.GetCompleteData(ctx, &threadify.CompleteDataOptions{
		StepHistoryLimit: 20,
		ValidationLimit:  10,
	})
	if err != nil {
		log.Fatal("get complete data:", err)
	}

	log.Printf("Thread data: %+v", completeData)
	log.Printf("\nRefs: %v", completeData["refs"])
}
