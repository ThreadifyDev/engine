package graphql

import (
	"context"
	"fmt"

	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/perf"
)

type ThreadQueryOptions struct {
	Limit  *int
	Offset *int
}

func NormalizePagination(opts *ThreadQueryOptions) (limit, offset int) {
	limit = 50
	if opts.Limit != nil && *opts.Limit > 0 {
		limit = *opts.Limit
		if limit > MaxThreadsPerQuery {
			limit = MaxThreadsPerQuery
		}
	}
	if opts.Offset != nil {
		offset = *opts.Offset
	}
	return limit, offset
}

func (r *queryResolver) BatchLoadThreadData(ctx context.Context, threads []*models.Thread) error {
	if len(threads) == 0 {
		return nil
	}

	selectionStart := perf.Now()
	selections := ExtractFieldSelections(ctx)
	perf.Log("[PERF] batchLoad.extractSelections: %v\n", perf.Since(selectionStart))

	threadIDs := make([]string, len(threads))
	for i, thread := range threads {
		threadIDs[i] = thread.ID
	}

	batchLoadStart := perf.Now()

	if selections.Has("refs") {
		refsStart := perf.Now()
		refsMap, err := r.refsRepo.GetRefsBatch(ctx, threadIDs)
		if err != nil {
			return fmt.Errorf("batch load thread refs: %w", err)
		}
		for _, thread := range threads {
			if refs, ok := refsMap[thread.ID]; ok {
				thread.Refs = refs
			}
		}
		perf.Log("[PERF] batchLoad.refs: %v (%d threads)\n", perf.Since(refsStart), len(threadIDs))
	}

	if selections.Has("steps") {
		stepsStart := perf.Now()
		stepsMap, err := r.stepStatePostgres.GetStepsBatch(ctx, threadIDs)
		if err != nil {
			return fmt.Errorf("batch load thread steps: %w", err)
		}
		stepsCache := make(map[string]interface{}, len(stepsMap))
		for threadID, steps := range stepsMap {
			stepsCache[threadID] = steps
		}
		ctx = cacheSteps(ctx, stepsCache)
		perf.Log("[PERF] batchLoad.steps: %v (%d threads)\n", perf.Since(stepsStart), len(threadIDs))
	}

	perf.Log("[PERF] batchLoad.total: %v\n", perf.Since(batchLoadStart))
	return nil
}

func (r *queryResolver) FilterThreadsByAccess(ctx context.Context, threads []*models.Thread, ownerID string) ([]*models.Thread, error) {
	if len(threads) == 0 {
		return []*models.Thread{}, nil
	}

	accessCheckStart := perf.Now()

	accessMap, err := r.threadAccessService.BatchCheckThreadAccess(ctx, threads, ownerID, "read")
	if err != nil {
		return nil, fmt.Errorf("batch check thread access: %w", err)
	}

	accessible := make([]*models.Thread, 0, len(threads))
	for _, thread := range threads {
		if hasAccess, ok := accessMap[thread.ID]; ok && hasAccess {
			accessible = append(accessible, thread)
			ctx = cacheAccessCheck(ctx, thread.ID, true)
		}
	}

	perf.Log("[PERF] accessCheck.total: %v (checked %d, accessible %d)\n",
		perf.Since(accessCheckStart), len(threads), len(accessible))

	return accessible, nil
}

func (r *queryResolver) ProcessThreadQuery(ctx context.Context, threads []*models.Thread, ownerID string) ([]*models.Thread, error) {
	if len(threads) == 0 {
		return []*models.Thread{}, nil
	}
	if err := r.BatchLoadThreadData(ctx, threads); err != nil {
		return nil, err
	}
	return r.FilterThreadsByAccess(ctx, threads, ownerID)
}
