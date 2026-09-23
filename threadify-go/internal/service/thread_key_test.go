package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

func TestThreadKeyCreationAndContractResume(t *testing.T) {
	ctx := context.Background()
	writer := newFakeOTelThreadWriter()
	repo := newFakeOTelCorrelationRepository()
	svc := NewOTelTraceService(writer, repo, zap.NewNop())
	first := svc.startSDKThread(ctx, &domain.StartThreadCmd{Action: "thread", ThreadKey: " session-1 ", Label: "Original", ContractName: "agent:3", Refs: map[string]string{"customer": "customer-1"}, Tags: []string{"agent"}}, "owner", "company")
	require.Equal(t, StepStatusSuccess, first.Status, first.Message)
	require.Equal(t, "session-1", first.ThreadKey)
	require.Equal(t, "agent", first.ContractName)
	require.Equal(t, 3, *first.ContractVersion)
	for _, contract := range []string{"", "agent", "agent:3"} {
		// Recreate the service with an empty correlation cache, as after a restart.
		repo.threads = map[string]string{}
		svc = NewOTelTraceService(writer, repo, zap.NewNop())
		resumed := svc.startSDKThread(ctx, &domain.StartThreadCmd{Action: "thread", ThreadKey: "session-1", Label: "Ignored", ContractName: contract, Refs: map[string]string{"customer": "ignored"}}, "owner", "company")
		require.Equal(t, StepStatusSuccess, resumed.Status, resumed.Message)
		require.Equal(t, first.ThreadID, resumed.ThreadID)
		require.Equal(t, first.ContractVersion, resumed.ContractVersion)
		require.Equal(t, "Original", resumed.Label)
		require.Equal(t, "customer-1", resumed.Refs["customer"])
		require.Equal(t, []string{"agent"}, resumed.Tags)
	}
	for _, contract := range []string{"other", "agent:4"} {
		rejected := svc.startSDKThread(ctx, &domain.StartThreadCmd{ThreadKey: "session-1", ContractName: contract}, "owner", "company")
		require.Equal(t, StepStatusError, rejected.Status)
		require.Contains(t, rejected.Message, "contract conflicts")
	}
	require.Len(t, writer.starts, 1)
}

func TestThreadKeyFreeFormCreationDoesNotAcquireContractOnResume(t *testing.T) {
	svc := NewOTelTraceService(newFakeOTelThreadWriter(), newFakeOTelCorrelationRepository(), zap.NewNop())
	first := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{Action: "thread", ThreadKey: "free"}, "owner", "company")
	require.Equal(t, StepStatusSuccess, first.Status, first.Message)
	require.Empty(t, first.ContractName)
	require.Nil(t, first.ContractVersion)
	rejected := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{ThreadKey: "free", ContractName: "agent"}, "owner", "company")
	require.Equal(t, StepStatusError, rejected.Status)
	require.Contains(t, rejected.Message, "contract conflicts")
}

func TestThreadKeySDKAndOTLPAreOneIdentity(t *testing.T) {
	for _, sdkFirst := range []bool{true, false} {
		writer := newFakeOTelThreadWriter()
		svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
		sdk := func() {
			r := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{Action: "thread", ThreadKey: "session"}, "owner", "company")
			require.Equal(t, StepStatusSuccess, r.Status, r.Message)
		}
		if sdkFirst {
			sdk()
		}
		for i, key := range []string{"threadify.thread_key", "workflow.run_id"} {
			r, err := svc.Ingest(context.Background(), correlatedExport(byte(i+1), key, "session"), "owner", "company")
			require.NoError(t, err)
			require.Nil(t, r.PartialSuccess)
		}
		if !sdkFirst {
			sdk()
		}
		require.Len(t, writer.starts, 1)
		require.Len(t, writer.records, 2)
		require.Empty(t, writer.completions)
		for _, event := range writer.records {
			require.Equal(t, writer.starts[0].ThreadID, event.ThreadID)
		}
	}
}

func TestThreadKeyClosedThreadsCannotResumeOrReceiveSpans(t *testing.T) {
	for _, status := range []domain.ThreadStatus{domain.ThreadStatusCompleted, domain.ThreadStatusCancelled, domain.ThreadStatusClosed, domain.ThreadStatusFailed} {
		for _, cacheLoss := range []bool{false, true} {
			writer := newFakeOTelThreadWriter()
			repo := newFakeOTelCorrelationRepository()
			svc := NewOTelTraceService(writer, repo, zap.NewNop())
			first := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{ThreadKey: "session"}, "owner", "company")
			require.Equal(t, StepStatusSuccess, first.Status, first.Message)
			writer.statuses[first.ThreadID] = status
			if cacheLoss {
				repo.threads = map[string]string{}
			}
			resumed := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{ThreadKey: "session"}, "owner", "company")
			require.Equal(t, StepStatusError, resumed.Status)
			require.Contains(t, resumed.Message, string(status)+" thread")
			r, err := svc.Ingest(context.Background(), correlatedExport(1, "threadify.thread_key", "session"), "owner", "company")
			require.NoError(t, err)
			require.EqualValues(t, 1, r.GetPartialSuccess().GetRejectedSpans())
			require.Empty(t, writer.records)
			require.Len(t, writer.starts, 1)
		}
	}
}

func TestThreadKeyConcurrentSDKAndOTLPAndTenantIsolation(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	repo := newFakeOTelCorrelationRepository()
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := NewOTelTraceService(writer, repo, zap.NewNop())
			if i%2 == 0 {
				r := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{ThreadKey: "session"}, "owner", "company")
				if r.Status != StepStatusSuccess {
					t.Errorf("SDK: %s", r.Message)
				}
			} else {
				r, err := svc.Ingest(context.Background(), correlatedExport(byte(i), "threadify.thread_key", "session"), "owner", "company")
				if err != nil || r.GetPartialSuccess().GetRejectedSpans() != 0 {
					t.Errorf("OTLP: %v %v", r, err)
				}
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, writer.starts, 1)
	svc := NewOTelTraceService(writer, repo, zap.NewNop())
	other := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{ThreadKey: "session"}, "owner", "other-company")
	require.Equal(t, StepStatusSuccess, other.Status, other.Message)
	require.Len(t, writer.starts, 2)
	require.NotEqual(t, writer.starts[0].ThreadID, other.ThreadID)
}

func TestThreadKeyValidationAndCanonicalPrecedence(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	for _, key := range []string{"", "  ", strings.Repeat("é", 513)} {
		r := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{Action: "thread", ThreadKey: key}, "owner", "company")
		require.Equal(t, StepStatusError, r.Status)
	}
	require.Empty(t, writer.starts)
	req := correlatedExport(1, "threadify.thread_key", "canonical")
	span := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Attributes = append(span.Attributes, otelKV("workflow.run_id", otelString("workflow")))
	r, err := svc.Ingest(context.Background(), req, "owner", "company")
	require.NoError(t, err)
	require.Nil(t, r.PartialSuccess)
	require.Equal(t, "canonical", writer.starts[0].Refs["threadify.thread_key"])
	require.Equal(t, correlatedThreadID("company", threadKeyCorrelationID("canonical")), writer.starts[0].ThreadID)
}

func TestThreadKeyCompletionWaitsForEveryTraceInBatch(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	batch := correlatedExport(1, "threadify.thread_key", "session")
	marked := batch.ResourceSpans[0].ScopeSpans[0].Spans[0]
	marked.Attributes = append(marked.Attributes, otelKV("threadify.run.complete", otelString("true")))
	other := correlatedExport(2, "threadify.thread_key", "session")
	batch.ResourceSpans = append(batch.ResourceSpans, other.ResourceSpans...)
	r, err := svc.Ingest(context.Background(), batch, "owner", "company")
	require.NoError(t, err)
	require.Nil(t, r.PartialSuccess)
	require.Len(t, writer.records, 2)
	require.Equal(t, []int{2}, writer.completionRecordCounts)
}

func TestThreadKeyCannotBeReassignedThroughBusinessRefs(t *testing.T) {
	thread := &domain.Thread{Refs: map[string]string{"threadify.thread_key": "session"}}
	require.NoError(t, validateThreadKeyRefs(thread, map[string]string{"customer": "other"}))
	require.NoError(t, validateThreadKeyRefs(thread, map[string]string{"threadify.thread_key": "session"}))
	require.ErrorContains(t, validateThreadKeyRefs(thread, map[string]string{"threadify.thread_key": "other"}), "cannot be changed")
	require.Error(t, validateThreadKeyRefs(thread, map[string]string{"threadify.thread_key": ""}))
}

func TestThreadKeyExplicitInternalTargetDoesNotComplete(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	created := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{Action: "thread", ThreadKey: "session"}, "owner", "company")
	require.Equal(t, StepStatusSuccess, created.Status)
	batch := correlatedExport(1, "threadify.thread_id", created.ThreadID)
	span := batch.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Attributes = append(span.Attributes, otelKV("threadify.run.complete", otelString("true")))
	response, err := svc.Ingest(context.Background(), batch, "owner", "company")
	require.NoError(t, err)
	require.Nil(t, response.PartialSuccess)
	require.Len(t, writer.records, 1)
	require.Empty(t, writer.completions, "explicit targets leave lifecycle control to the caller")
}

func TestTraceOnlyRecoveryPreservesDurableLifecycle(t *testing.T) {
	for _, status := range []domain.ThreadStatus{domain.ThreadStatusActive, domain.ThreadStatusCompleted, domain.ThreadStatusCancelled, domain.ThreadStatusClosed, domain.ThreadStatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			writer := newFakeOTelThreadWriter()
			svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
			batch := correlatedExport(1, "", "")
			_, err := svc.Ingest(context.Background(), batch, "owner", "company")
			require.NoError(t, err)
			require.Len(t, writer.starts, 1)
			id := writer.starts[0].ThreadID
			writer.statuses[id] = status
			// Both the resolver and its cache are replaced, while durable thread state remains.
			svc = NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
			response, err := svc.Ingest(context.Background(), batch, "owner", "company")
			require.NoError(t, err)
			if status == domain.ThreadStatusActive {
				require.Nil(t, response.PartialSuccess)
			} else {
				require.EqualValues(t, 1, response.GetPartialSuccess().GetRejectedSpans())
				require.Contains(t, response.GetPartialSuccess().GetErrorMessage(), string(status))
			}
			require.Len(t, writer.starts, 1, "durable threads must never be recreated")
		})
	}
}
