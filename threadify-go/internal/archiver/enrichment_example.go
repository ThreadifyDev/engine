package archiver

import (
	"context"
	"time"

	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

// EnrichmentService demonstrates how to use the enrichment worker pool
// in the archiver for async data enrichment tasks
type EnrichmentService struct {
	pool   *workerpool.Pool
	logger *zap.Logger
}

// NewEnrichmentService creates a new enrichment service
func NewEnrichmentService(pool *workerpool.Pool, logger *zap.Logger) *EnrichmentService {
	return &EnrichmentService{
		pool:   pool,
		logger: logger,
	}
}

// EnrichThreadMetadata enriches thread metadata asynchronously
// This is a non-blocking operation that submits a job to the worker pool
func (s *EnrichmentService) EnrichThreadMetadata(threadID string, metadata map[string]interface{}) {
	submitted := s.pool.Submit(func(ctx context.Context) {
		s.logger.Debug("enriching thread metadata",
			zap.String("thread_id", threadID),
		)

		// Example: Add derived fields, calculate metrics, etc.
		// This runs asynchronously in the worker pool
		enrichedData := s.calculateDerivedMetrics(metadata)

		// Store enriched data (pseudo-code)
		if err := s.saveEnrichedMetadata(ctx, threadID, enrichedData); err != nil {
			s.logger.Error("failed to save enriched metadata",
				zap.String("thread_id", threadID),
				zap.Error(err),
			)
		}
	})

	if !submitted {
		s.logger.Warn("enrichment job dropped (queue full)",
			zap.String("thread_id", threadID),
		)
	}
}

// EnrichStepData enriches step data with external information
func (s *EnrichmentService) EnrichStepData(threadID, stepID string, stepData map[string]interface{}) {
	s.pool.Submit(func(ctx context.Context) {
		s.logger.Debug("enriching step data",
			zap.String("thread_id", threadID),
			zap.String("step_id", stepID),
		)

		// Example: Fetch entity profile, external API data, etc.
		// Add timeout for external calls
		enrichCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		enrichedData, err := s.fetchExternalEnrichment(enrichCtx, stepData)
		if err != nil {
			s.logger.Warn("external enrichment failed",
				zap.String("thread_id", threadID),
				zap.String("step_id", stepID),
				zap.Error(err),
			)
			return
		}

		// Save enriched data
		if err := s.saveEnrichedStepData(ctx, threadID, stepID, enrichedData); err != nil {
			s.logger.Error("failed to save enriched step data",
				zap.String("thread_id", threadID),
				zap.String("step_id", stepID),
				zap.Error(err),
			)
		}
	})
}

// EnrichWithEntityProfile enriches data with entity profile information
// This is a critical enrichment that blocks until queued
func (s *EnrichmentService) EnrichWithEntityProfile(ctx context.Context, threadID, entityID string) error {
	enrichCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.pool.SubmitWait(enrichCtx, func(jobCtx context.Context) {
		s.logger.Debug("enriching with entity profile",
			zap.String("thread_id", threadID),
			zap.String("entity_id", entityID),
		)

		// Fetch entity profile
		profile, err := s.fetchEntityProfile(jobCtx, entityID)
		if err != nil {
			s.logger.Error("failed to fetch entity profile",
				zap.String("entity_id", entityID),
				zap.Error(err),
			)
			return
		}

		// Attach profile to thread
		if err := s.attachProfileToThread(jobCtx, threadID, profile); err != nil {
			s.logger.Error("failed to attach profile",
				zap.String("thread_id", threadID),
				zap.Error(err),
			)
		}
	})
}

// Helper methods (pseudo-code examples)

func (s *EnrichmentService) calculateDerivedMetrics(metadata map[string]interface{}) map[string]interface{} {
	// Example: Calculate thread duration, step counts, etc.
	enriched := make(map[string]interface{})
	// ... calculation logic ...
	return enriched
}

func (s *EnrichmentService) saveEnrichedMetadata(ctx context.Context, threadID string, data map[string]interface{}) error {
	// Save to database
	return nil
}

func (s *EnrichmentService) fetchExternalEnrichment(ctx context.Context, stepData map[string]interface{}) (map[string]interface{}, error) {
	// Call external API
	return nil, nil
}

func (s *EnrichmentService) saveEnrichedStepData(ctx context.Context, threadID, stepID string, data map[string]interface{}) error {
	// Save to database
	return nil
}

func (s *EnrichmentService) fetchEntityProfile(ctx context.Context, entityID string) (map[string]interface{}, error) {
	// Fetch from entity profile repository
	return nil, nil
}

func (s *EnrichmentService) attachProfileToThread(ctx context.Context, threadID string, profile map[string]interface{}) error {
	// Update thread with profile data
	return nil
}

// Example integration in a consumer:
//
// type StepStateConsumer struct {
//     enrichmentService *EnrichmentService
//     // ... other fields ...
// }
//
// func (c *StepStateConsumer) processStepState(stepState StepState) {
//     // Archive step state to database (blocking)
//     if err := c.archiveStepState(stepState); err != nil {
//         return err
//     }
//
//     // Enrich step data asynchronously (non-blocking)
//     c.enrichmentService.EnrichStepData(
//         stepState.ThreadID,
//         stepState.StepID,
//         stepState.Data,
//     )
// }
