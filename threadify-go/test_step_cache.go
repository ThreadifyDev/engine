package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
)

func main() {
	fmt.Println("🧪 Testing Phase 2 Step State Caching...")

	// Use same config as server
	fmt.Println("🔗 Connecting to databases using server config...")

	db, err := database.NewPostgresDB("postgres://td_engine:tdtdtd@localhost:5434/threadify?sslmode=disable", 10)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer db.Pool.Close()

	valkeyClient, err := database.NewValkeyService("localhost", 6379, "threadify_secure_password", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if err != nil {
		log.Fatalf("Failed to connect to Valkey: %v", err)
	}
	defer valkeyClient.Close()

	// Create repositories with cache-aside pattern
	postgresStepRepo := postgres.NewStepStateRepository(db.Pool)
	stepStateRepo := valkey.NewStepStateRepositoryWithPostgres(valkeyClient, postgresStepRepo)

	// Test with real data from thread_activities
	threadID := "016abee5-e137-42a0-9620-55e3e03366ca"
	stepName := "order_placed"
	idempotencyKey := "test-1767698362695"

	fmt.Printf("📊 Testing step: %s:%s for thread %s\n", stepName, idempotencyKey, threadID)

	ctx := context.Background()

	// First query - should be cache miss and hit PostgreSQL
	fmt.Println("\n🔍 First query (expecting cache miss)...")
	start := time.Now()
	stepState, err := stepStateRepo.GetStepStateWithCache(ctx, threadID, stepName, idempotencyKey)
	firstQueryDuration := time.Since(start)

	if err != nil {
		log.Fatalf("First query failed: %v", err)
	}

	if stepState == nil {
		log.Fatalf("Step state not found in PostgreSQL")
	}

	fmt.Printf("✅ First query completed in %v\n", firstQueryDuration)
	fmt.Printf("   Status: %s\n", stepState.Status)
	fmt.Printf("   RetryCount: %d\n", stepState.RetryCount)
	fmt.Printf("   FirstSeenAt: %s\n", stepState.FirstSeenAt.Format(time.RFC3339))
	fmt.Printf("   LastUpdatedAt: %s\n", stepState.LastUpdatedAt.Format(time.RFC3339))

	// Second query - should be cache hit from Redis
	fmt.Println("\n🎯 Second query (expecting cache hit)...")
	start = time.Now()
	stepState2, err := stepStateRepo.GetStepStateWithCache(ctx, threadID, stepName, idempotencyKey)
	secondQueryDuration := time.Since(start)

	if err != nil {
		log.Fatalf("Second query failed: %v", err)
	}

	if stepState2 == nil {
		log.Fatalf("Step state not found on second query")
	}

	fmt.Printf("✅ Second query completed in %v\n", secondQueryDuration)
	fmt.Printf("   Status: %s\n", stepState2.Status)
	fmt.Printf("   RetryCount: %d\n", stepState2.RetryCount)

	// Verify results are consistent
	if stepState.Status != stepState2.Status || stepState.RetryCount != stepState2.RetryCount {
		log.Fatalf("❌ Results inconsistent between queries!")
	}

	// Calculate performance improvement
	speedup := float64(firstQueryDuration) / float64(secondQueryDuration)
	fmt.Printf("\n🚀 Performance improvement: %.2fx faster on cache hit\n", speedup)

	if speedup > 2.0 {
		fmt.Println("✅ Cache-aside pattern working correctly!")
	} else {
		fmt.Println("⚠️  Cache performance improvement minimal")
	}

	fmt.Println("\n🎉 Phase 2 Step State Caching test completed successfully!")
}
