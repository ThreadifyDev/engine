package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
)

// Only engine-created, contract-free telemetry threads participate in automatic
// trace completion. Explicit workflow targets retain
// their existing lifecycle and terminal-write protections.
func isOwnedOTelThread(thread *domain.Thread, companyID, traceID string) bool {
	if traceID == "" || thread.CompanyID != companyID || thread.ContractName != "" || (thread.ContractID != nil && *thread.ContractID != "") {
		return false
	}
	// Shared work remains open until the exporter explicitly marks the run complete.
	ref := thread.Refs["threadify.thread_key"]
	if ref != "" {
		return thread.ID == correlatedThreadID(companyID, threadKeyCorrelationID(ref))
	}
	return thread.Refs["otel_trace_id"] == traceID && thread.ID == uuid.NewSHA1(uuid.NameSpaceOID, []byte(companyID+":"+traceID)).String()
}

// CompleteTraceForIngestion records execution completion, not business success.
// The completed root/marked span remains the hashed audit evidence. Publishing
// a full snapshot makes persistence safe even when initial archival is delayed.
// Repeating the snapshot is harmless and repairs interrupted publication/cache
// updates; a terminal cache entry must not suppress a persistence retry.
func (s *ThreadService) CompleteTraceForIngestion(ctx context.Context, threadID, ownerID, companyID, traceID string, endedAt time.Time) error {
	if err := s.ValidateThreadForIngestion(ctx, threadID, ownerID, companyID); err != nil {
		return err
	}
	thread, err := s.repo.Get(ctx, threadID)
	if err != nil {
		return err
	}
	if !isOwnedOTelThread(thread, companyID, traceID) ||
		(thread.Status != domain.ThreadStatusActive && thread.Status != domain.ThreadStatusCompleted) {
		return nil
	}
	if s.natsArchivalPublisher == nil {
		return fmt.Errorf("OTEL completion persistence is unavailable")
	}
	if thread.CompletedAt != nil {
		endedAt = *thread.CompletedAt
	}
	if err := s.natsArchivalPublisher.PublishThreadMetadata(ctx, map[string]interface{}{
		"action":   "otel_completed",
		"threadId": threadID, "companyId": companyID, "ownerId": thread.OwnerID,
		"label": thread.Label, "contractVersion": "0", "error": thread.Error,
		"status":      ThreadStatusCompleted,
		"startedAt":   thread.StartedAt.Format(time.RFC3339Nano),
		"completedAt": endedAt.Format(time.RFC3339Nano),
	}); err != nil {
		return fmt.Errorf("persist OTEL completion: %w", err)
	}
	if err := s.repo.UpdateThreadStatus(ctx, threadID, ThreadStatusCompleted, endedAt); err != nil {
		return err
	}
	if s.otelTrace != nil {
		if err := s.otelTrace.correlations.MarkTraceCompleted(ctx, companyID, traceID); err != nil {
			return err
		}
	}
	s.cacheManager.ClearThreadCache(threadID)
	return nil
}
