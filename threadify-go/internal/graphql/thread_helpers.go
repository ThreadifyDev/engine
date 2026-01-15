package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/models"
)

// ThreadQueryOptions contains common options for thread queries
type ThreadQueryOptions struct {
	Limit  *int
	Offset *int
}

// NormalizePagination normalizes and validates pagination parameters
func NormalizePagination(opts *ThreadQueryOptions) (limit, offset int) {
	limit = 50 // default
	offset = 0 // default

	if opts.Limit != nil {
		limit = *opts.Limit
		// Cap at 500 to prevent abuse
		if limit > 500 {
			limit = 500
		}
	}

	if opts.Offset != nil {
		offset = *opts.Offset
	}

	return limit, offset
}

// BatchLoadThreadData loads refs and steps for threads based on GraphQL selections
// This is the core reusable batch loading logic
func (r *queryResolver) BatchLoadThreadData(ctx context.Context, threads []*models.Thread) error {
	if len(threads) == 0 {
		return nil
	}

	// Extract GraphQL field selections
	selectionStart := time.Now()
	selections := ExtractFieldSelections(ctx)
	fmt.Printf("[PERF] batchLoad.extractSelections: %v\n", time.Since(selectionStart))

	// Collect thread IDs for batch loading
	threadIDs := make([]string, len(threads))
	for i, thread := range threads {
		threadIDs[i] = thread.ID
	}

	batchLoadStart := time.Now()

	// Conditionally batch load refs (only if requested in GraphQL query)
	if selections.Has("refs") {
		refsStart := time.Now()
		refsMap, err := r.refsRepo.GetRefsBatch(ctx, threadIDs)
		fmt.Printf("[PERF] batchLoad.refs: %v (loaded for %d threads)\n", time.Since(refsStart), len(threadIDs))
		if err != nil {
			fmt.Printf("Warning: failed to batch load refs: %v\n", err)
		} else {
			// Assign refs to threads
			for _, thread := range threads {
				if refs, ok := refsMap[thread.ID]; ok {
					thread.Refs = refs
				}
			}
		}
	} else {
		fmt.Printf("[PERF] batchLoad.refs: SKIPPED (not requested)\n")
	}

	// Note: Steps are loaded lazily via the Thread.steps field resolver
	// This provides automatic GraphQL-aware loading

	fmt.Printf("[PERF] batchLoad.total: %v\n", time.Since(batchLoadStart))
	return nil
}

// FilterThreadsByAccess filters threads based on user access permissions
// This is the core reusable access control logic
func (r *queryResolver) FilterThreadsByAccess(ctx context.Context, threads []*models.Thread, ownerID string) ([]*models.Thread, error) {
	if len(threads) == 0 {
		return []*models.Thread{}, nil
	}

	accessCheckStart := time.Now()
	fmt.Printf("[PERF] accessCheck: Starting for %d threads\n", len(threads))

	accessibleThreads := make([]*models.Thread, 0, len(threads))
	for i, thread := range threads {
		threadCheckStart := time.Now()
		// Check if user has read permission
		hasAccess, err := r.threadAccessService.CheckThreadAccess(thread.ID, ownerID, "read", thread)
		fmt.Printf("[PERF] accessCheck[%d]: %v\n", i, time.Since(threadCheckStart))
		if err != nil || !hasAccess {
			// Skip threads user doesn't have access to
			continue
		}

		accessibleThreads = append(accessibleThreads, thread)
	}

	fmt.Printf("[PERF] accessCheck.total: %v (checked %d, accessible %d)\n",
		time.Since(accessCheckStart), len(threads), len(accessibleThreads))

	return accessibleThreads, nil
}

// ProcessThreadQuery is a high-level helper that combines all common thread query logic:
// 1. Batch load related data (refs, etc.)
// 2. Filter by access permissions
// This eliminates duplication across all thread query resolvers
func (r *queryResolver) ProcessThreadQuery(ctx context.Context, threads []*models.Thread, ownerID string) ([]*models.Thread, error) {
	if len(threads) == 0 {
		return []*models.Thread{}, nil
	}

	// Step 1: Batch load related data based on GraphQL selections
	if err := r.BatchLoadThreadData(ctx, threads); err != nil {
		return nil, err
	}

	// Step 2: Filter by access permissions
	return r.FilterThreadsByAccess(ctx, threads, ownerID)
}
