package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/repository/valkey"
)

func main() {
	// Initialize database connections
	pgURL := "postgres://td_engine:tdtdtd@localhost:5434/threadify?sslmode=disable"
	maxConns := 25
	db, err := database.NewPostgresDB(pgURL, maxConns)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer db.Close()

	redisHost := "localhost"
	redisPort := 6379
	redisPassword := "threadify_secure_password"
	redisDB := 0
	valkeyService, err := database.NewValkeyService(redisHost, redisPort, redisPassword, redisDB)
	if err != nil {
		log.Fatal("Failed to connect to Valkey:", err)
	}
	defer valkeyService.Close()

	// Create repository
	threadRepo := valkey.NewThreadRepository(valkeyService, 3600)

	// Get the thread
	threadID := "d46e195f-8c31-43ac-8ca3-91ab03acf1a1"

	fmt.Printf("Checking thread: %s\n", threadID)

	thread, err := threadRepo.Get(context.Background(), threadID)
	if err != nil {
		log.Fatal("Failed to get thread:", err)
	}

	fmt.Printf("Thread ID: %s\n", thread.ID)
	fmt.Printf("Owner ID: %s\n", thread.OwnerID)
	fmt.Printf("Status: %s\n", thread.Status)
	fmt.Printf("Last Hash: '%s'\n", thread.LastHash)
	fmt.Printf("Started At: %s\n", thread.StartedAt.Format(time.RFC3339Nano)) // Show nanosecond precision

	fmt.Printf("Steps (%d):\n", len(thread.Steps))
	for stepName, step := range thread.Steps {
		fmt.Printf("  - %s: %s (completed: %t, updated: %s)\n", stepName, step.Status, step.IsCompleted, step.UpdatedAt.Format(time.RFC3339Nano))
	}

	// Now check for individual step events
	fmt.Printf("\nChecking individual step events in Redis:\n")

	// Get all keys for this thread's events
	ctx := context.Background()
	pattern := fmt.Sprintf("thread:events:%s:*", threadID)

	keys, err := valkeyService.Keys(ctx, pattern)
	if err != nil {
		log.Printf("Error getting keys: %v", err)
		return
	}

	fmt.Printf("Found %d event keys:\n", len(keys))

	// Parse and display each event to check hash chain
	for i, key := range keys {
		eventData, err := valkeyService.Get(ctx, key)
		if err != nil {
			log.Printf("Error getting event data for key %s: %v", key, err)
			continue
		}

		// Parse the event JSON
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			log.Printf("Error parsing event JSON: %v", err)
			continue
		}

		stepName := event["step_name"].(string)
		hash := event["hash"].(string)
		prevHash := event["prev_hash"].(string)
		timestamp := event["created_at"].(string)

		fmt.Printf("  [%d] Step: %s\n", i+1, stepName)
		fmt.Printf("      Hash: %s\n", hash[:16]+"...")
		fmt.Printf("      PrevHash: %s\n", func() string {
			if prevHash == "" {
				return "[GENESIS]"
			}
			return prevHash[:16] + "..."
		}())
		fmt.Printf("      Created: %s\n", timestamp)
		fmt.Printf("\n")
	}

	fmt.Printf("📝 Hash Chain Analysis:\n")
	fmt.Printf("• Each step should reference the previous step's hash\n")
	fmt.Printf("• First step should have empty prev_hash (genesis)\n")
	fmt.Printf("• Subsequent steps should have non-empty prev_hash\n")
	fmt.Printf("• Thread LastHash should be the hash of the last step\n")
}
