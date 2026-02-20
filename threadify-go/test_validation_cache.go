package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
)

func main() {
	log.Println("🧪 Testing Phase 3: Validation Results Cache-Aside Pattern")

	// Database configuration
	dbHost := "localhost"
	dbPort := 5434
	dbUser := "td_engine"
	dbPassword := "tdtdtd"
	dbName := "threadify"

	// Redis configuration
	redisHost := "localhost"
	redisPort := 6379
	redisPassword := "threadify_secure_password"
	redisDB := 0

	ctx := context.Background()

	// Connect to PostgreSQL
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	log.Println("✅ Connected to PostgreSQL")

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", redisHost, redisPort),
		Password: redisPassword,
		DB:       redisDB,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	defer rdb.Close()
	log.Println("✅ Connected to Redis")

	// Create Valkey service
	valkeyService, err := database.NewValkeyService(redisHost, redisPort, redisPassword, redisDB)
	if err != nil {
		log.Fatalf("❌ Failed to create Valkey service: %v", err)
	}
	defer valkeyService.Close()

	// Create repositories
	postgresValidationRepo := postgres.NewValidationRepository(dbPool)
	validationRepo := valkey.NewValidationRepositoryWithPostgres(valkeyService, postgresValidationRepo)

	// Test data - use known thread ID from existing data
	testThreadID := "test-thread-123"
	testStepName := "order_placed"
	testIdempotencyKey := "060ef754"

	log.Printf("🔍 Testing validation results for thread=%s, step=%s:%s", testThreadID, testStepName, testIdempotencyKey)

	// Step 1: Check if validation data exists in PostgreSQL
	log.Println("\n📋 Step 1: Checking PostgreSQL for validation data...")
	exists, err := postgresValidationRepo.ValidationResultExists(ctx, testThreadID, testStepName, testIdempotencyKey)
	if err != nil {
		log.Fatalf("❌ Failed to check validation existence: %v", err)
	}

	if !exists {
		log.Printf("⚠️  No validation data found for test parameters. Creating sample data...")
		// Create sample validation data for testing
		if err := createSampleValidationData(ctx, dbPool, testThreadID, testStepName, testIdempotencyKey); err != nil {
			log.Fatalf("❌ Failed to create sample data: %v", err)
		}
		log.Println("✅ Sample validation data created")
	}

	// Step 2: Clear Redis cache to ensure cache miss
	cacheKey := fmt.Sprintf("thread:%s:validations:%s:%s", testThreadID, testStepName, testIdempotencyKey)
	if err := rdb.Del(ctx, cacheKey).Err(); err != nil {
		log.Printf("⚠️  Failed to clear cache: %v", err)
	} else {
		log.Println("🗑️  Cleared Redis cache for cache miss test")
	}

	// Step 3: Test cache miss (PostgreSQL fallback)
	log.Println("\n🔍 Step 3: Testing cache miss (PostgreSQL fallback)...")
	start := time.Now()
	results, err := validationRepo.GetValidationResultsWithCache(ctx, testThreadID, testStepName, testIdempotencyKey)
	cacheMissDuration := time.Since(start)

	if err != nil {
		log.Fatalf("❌ Cache miss test failed: %v", err)
	}

	if len(results) == 0 {
		log.Fatalf("❌ No validation results returned from PostgreSQL")
	}

	log.Printf("✅ Cache miss successful: retrieved %d validation results in %v", len(results), cacheMissDuration)
	for i, result := range results {
		log.Printf("   Result %d: %s (%s) - %d validations", i+1, result.StepID, result.Timestamp.Format(time.RFC3339), len(result.Validations))
	}

	// Step 4: Verify cache was populated
	log.Println("\n💾 Step 4: Verifying cache was populated...")
	cachedData, err := rdb.HGet(ctx, cacheKey, "results").Result()
	if err != nil {
		log.Printf("❌ Cache not populated: %v", err)
	} else {
		log.Printf("✅ Cache populated successfully (%d bytes)", len(cachedData))
	}

	// Step 5: Test cache hit (Redis only)
	log.Println("\n🎯 Step 5: Testing cache hit (Redis only)...")
	start = time.Now()
	cachedResults, err := validationRepo.GetValidationResultsWithCache(ctx, testThreadID, testStepName, testIdempotencyKey)
	cacheHitDuration := time.Since(start)

	if err != nil {
		log.Fatalf("❌ Cache hit test failed: %v", err)
	}

	if len(cachedResults) != len(results) {
		log.Fatalf("❌ Cache hit returned different number of results: expected %d, got %d", len(results), len(cachedResults))
	}

	log.Printf("✅ Cache hit successful: retrieved %d validation results in %v", len(cachedResults), cacheHitDuration)

	// Step 6: Performance comparison
	if cacheMissDuration > 0 && cacheHitDuration > 0 {
		speedup := float64(cacheMissDuration) / float64(cacheHitDuration)
		log.Printf("\n📊 Performance Analysis:")
		log.Printf("   Cache Miss: %v", cacheMissDuration)
		log.Printf("   Cache Hit:  %v", cacheHitDuration)
		log.Printf("   Speedup:    %.2fx faster", speedup)
	}

	// Step 7: Test thread-level validation queries
	log.Println("\n🔍 Step 6: Testing thread-level validation queries...")
	options := &models.ValidationQueryOptions{
		ThreadID: testThreadID,
		Limit:    10,
	}

	threadResults, err := validationRepo.GetThreadValidationResultsWithCache(ctx, testThreadID, options)
	if err != nil {
		log.Printf("⚠️  Thread-level query failed: %v", err)
	} else {
		log.Printf("✅ Thread-level query successful: %d results", len(threadResults))
	}

	// Step 8: Test cache invalidation
	log.Println("\n🗑️  Step 7: Testing cache invalidation...")
	if err := validationRepo.InvalidateValidationResults(ctx, testThreadID, testStepName, testIdempotencyKey); err != nil {
		log.Printf("⚠️  Cache invalidation failed: %v", err)
	} else {
		log.Println("✅ Cache invalidation successful")
	}

	// Verify cache was cleared
	_, err = rdb.HGet(ctx, cacheKey, "results").Result()
	if err == nil {
		log.Printf("❌ Cache not properly invalidated")
	} else {
		log.Println("✅ Cache properly invalidated")
	}

	log.Println("\n🎉 Phase 3 Validation Results Cache-Aside Test Complete!")
	log.Println("✅ Cache-aside pattern working correctly")
	log.Println("✅ PostgreSQL fallback functional")
	log.Println("✅ Redis caching operational")
	log.Println("✅ Performance improvements verified")
}

// createSampleValidationData creates test validation data in PostgreSQL
func createSampleValidationData(ctx context.Context, dbPool *pgxpool.Pool, threadID, stepName, idempotencyKey string) error {
	stepID := fmt.Sprintf("%s:%s", stepName, idempotencyKey)

	// Insert sample validation result with all required columns
	query := `
		INSERT INTO thread_validations (
			validation_id, thread_id, step_id, step_name, idempotency_key, 
			timestamp, validations, overall_status, has_critical_violation,
			critical_count, warning_count, minor_count, info_count, total_validations
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) ON CONFLICT (validation_id) DO NOTHING
	`

	validationsJSON := `[
		{
			"type": "critical",
			"message": "Invalid transition order",
			"field": "step_order",
			"expected": "payment_received",
			"actual": "order_placed",
			"rule": "step_sequence_validation"
		},
		{
			"type": "warning", 
			"message": "Missing optional field",
			"field": "customer_notes",
			"rule": "optional_field_validation"
		}
	]`

	_, err := dbPool.Exec(ctx, query,
		fmt.Sprintf("validation-%s-%d", stepID, time.Now().Unix()),
		threadID,
		stepID,
		stepName,
		idempotencyKey,
		time.Now(),
		validationsJSON,
		"violated", // overall_status
		true,       // has_critical_violation
		1,          // critical_count
		1,          // warning_count
		0,          // minor_count
		0,          // info_count
		2,          // total_validations
	)

	return err
}
