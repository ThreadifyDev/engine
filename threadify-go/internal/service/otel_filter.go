package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	shderrors "threadify-go/shared/errors"
	"threadify-go/shared/ingestion"
)

type otelIngestionPolicyKey struct{}

// OTelIngestionPolicy is request-scoped. Settings apply only to general threads;
// contract inputs are governed by the thread's contract version instead.
type OTelIngestionPolicy struct {
	store     ingestion.Store
	settings  *ingestion.Settings
	evaluated int
	Dropped   int
}

func WithOTelIngestionRules(ctx context.Context, store ingestion.Store) (context.Context, *OTelIngestionPolicy) {
	policy := &OTelIngestionPolicy{store: store}
	return context.WithValue(ctx, otelIngestionPolicyKey{}, policy), policy
}

func (s *OTelTraceService) filterGeneralTrace(ctx context.Context, owner, company, traceID, threadKey string, descriptor otelSpanEnvelope, spans []otelSpanEnvelope) ([]otelSpanEnvelope, error) {
	policy, _ := ctx.Value(otelIngestionPolicyKey{}).(*OTelIngestionPolicy)
	if policy == nil || policy.store == nil {
		return spans, nil
	}
	attrs := descriptor.resourceAttrs
	if attributeString(attrs, "threadify.contract") != "" {
		return spans, nil
	}

	// Inspect the existing binding without creating a thread. This covers explicit
	// IDs, later batches without directives, workflow references, and cache expiry.
	id := attributeString(attrs, "threadify.thread_id")
	if id == "" {
		var err error
		id, err = s.correlations.GetThreadID(ctx, company, traceID)
		if err != nil {
			return nil, err
		}
		if id == "" {
			correlationID := traceID
			if threadKey != "" {
				correlationID = threadKeyCorrelationID(threadKey)
			}
			id = correlatedThreadID(company, correlationID)
		}
	}
	thread, err := s.threads.LookupThreadForIngestion(ctx, id, owner, company)
	if err != nil && !errors.Is(err, shderrors.ErrThreadNotFound) {
		return nil, err
	}
	if thread != nil && thread.ContractName != "" {
		return spans, nil
	}

	if policy.settings == nil {
		settings, err := policy.store.Load(ctx, company)
		if err != nil {
			return nil, fmt.Errorf("trace ingestion rules unavailable: %w", err)
		}
		policy.settings = &settings
	}
	kept := make([]otelSpanEnvelope, 0, len(spans))
	for _, span := range spans {
		policy.evaluated++
		if ingestion.ShouldDrop(policy.settings.Mode, policy.settings.Filters, span.span.GetName()) {
			policy.Dropped++
		} else {
			kept = append(kept, span)
		}
	}
	return kept, nil
}

func (s *OTelTraceService) recordFilterCounts(ctx context.Context, company string) {
	policy, _ := ctx.Value(otelIngestionPolicyKey{}).(*OTelIngestionPolicy)
	if policy == nil || policy.store == nil || policy.evaluated == 0 {
		return
	}
	if err := policy.store.Record(ctx, company, policy.evaluated, policy.Dropped); err != nil {
		s.logger.Warn("record OTLP filter counts", zap.Error(err))
	}
}
