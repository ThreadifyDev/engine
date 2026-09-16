package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	collecttracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"go.uber.org/zap"
)

// correlatedExport gives every invocation a distinct wire trace while retaining the caller reference.
func correlatedExport(index byte, key, value string) *collecttracepb.ExportTraceServiceRequest {
	span := validOTelSpan()
	span.TraceId[0] = index
	if key != "" {
		span.Attributes = append(span.Attributes, otelKV(key, otelString(value)))
	}
	return otelRequest(span, nil)
}

func TestOTelExternalReferenceResolution(t *testing.T) {
	for _, key := range []string{"threadify.external_ref", "workflow.run_id"} {
		t.Run(key, func(t *testing.T) {
			writer := newFakeOTelThreadWriter()
			repo := newFakeOTelCorrelationRepository()
			svc := NewOTelTraceService(writer, repo, zap.NewNop())
			for i := byte(1); i <= 3; i++ {
				response, err := svc.Ingest(context.Background(), correlatedExport(i, key, "payment-123"), "owner", "company")
				require.NoError(t, err)
				require.Nil(t, response.PartialSuccess)
			}
			require.Len(t, writer.starts, 1)
			require.Len(t, writer.records, 3)
			require.Empty(t, writer.completions)
			for _, r := range writer.records {
				require.Equal(t, writer.starts[0].ThreadID, r.ThreadID)
				require.NotContains(t, r.Refs, "otel_trace_id")
			}
			// Cache loss must recover the original persisted thread, not reset it.
			repo.threads = map[string]string{}
			response, err := svc.Ingest(context.Background(), correlatedExport(4, key, "payment-123"), "owner", "company")
			require.NoError(t, err)
			require.Nil(t, response.PartialSuccess)
			require.Len(t, writer.starts, 1)
		})
	}
}

func TestOTelWorkflowOptOutAndExternalPrecedence(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		writer := newFakeOTelThreadWriter()
		svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
		for i := byte(1); i <= 2; i++ {
			_, err := svc.Ingest(WithOTelWorkflowRunID(context.Background(), enabled), correlatedExport(i, "workflow.run_id", "run"), "owner", "company")
			require.NoError(t, err)
		}
		if enabled {
			require.Len(t, writer.starts, 1)
		} else {
			require.Len(t, writer.starts, 2)
		}
	}
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	for i := byte(1); i <= 2; i++ {
		req := correlatedExport(i, "threadify.external_ref", "explicit")
		span := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
		span.Attributes = append(span.Attributes, otelKV("workflow.run_id", otelString(fmt.Sprint(i))))
		_, err := svc.Ingest(WithOTelWorkflowRunID(context.Background(), false), req, "owner", "company")
		require.NoError(t, err)
	}
	require.Len(t, writer.starts, 1)
}

func TestOTelExternalReferenceRejectsConflictBeforeRecording(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	first := correlatedExport(1, "threadify.external_ref", "run")
	first.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = append(first.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes, otelKV("threadify.contract", otelString("payment")))
	_, err := svc.Ingest(context.Background(), first, "owner", "company")
	require.NoError(t, err)
	second := correlatedExport(2, "threadify.external_ref", "run")
	second.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = append(second.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes, otelKV("threadify.contract", otelString("shipping")))
	response, err := svc.Ingest(context.Background(), second, "owner", "company")
	require.NoError(t, err)
	require.EqualValues(t, 1, response.GetPartialSuccess().GetRejectedSpans())
	require.Len(t, writer.records, 1)
	// A trace already accepted under one identity cannot be remapped by later batches.
	response, err = svc.Ingest(context.Background(), correlatedExport(1, "threadify.external_ref", "other"), "owner", "company")
	require.NoError(t, err)
	require.EqualValues(t, 1, response.GetPartialSuccess().GetRejectedSpans())
	require.Len(t, writer.starts, 1)
}

func TestOTelExternalReferenceConcurrentReplicas(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	repo := newFakeOTelCorrelationRepository()
	a := NewOTelTraceService(writer, repo, zap.NewNop())
	b := NewOTelTraceService(writer, repo, zap.NewNop())
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := byte(1); i <= 32; i++ {
		wg.Add(1)
		go func(i byte) {
			defer wg.Done()
			svc := a
			if i%2 == 0 {
				svc = b
			}
			r, err := svc.Ingest(context.Background(), correlatedExport(i, "threadify.external_ref", "shared"), "owner", "company")
			if err == nil && r.GetPartialSuccess().GetRejectedSpans() > 0 {
				err = fmt.Errorf("rejected: %s", r.GetPartialSuccess().GetErrorMessage())
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, writer.starts, 1)
	require.Len(t, writer.records, 32)
}

func TestOTelExternalReferenceValidationAndCompletion(t *testing.T) {
	for _, value := range []*commonpb.AnyValue{otelInt(3), otelString(strings.Repeat("x", 1025))} {
		writer := newFakeOTelThreadWriter()
		svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
		req := correlatedExport(1, "", "")
		req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = []*commonpb.KeyValue{otelKV("threadify.external_ref", value)}
		r, err := svc.Ingest(context.Background(), req, "owner", "company")
		require.NoError(t, err)
		require.EqualValues(t, 1, r.GetPartialSuccess().GetRejectedSpans())
		require.Empty(t, writer.starts)
	}
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	req := correlatedExport(1, "threadify.external_ref", "run")
	req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = append(req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes, otelKV("threadify.run.complete", &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}))
	_, err := svc.Ingest(context.Background(), req, "owner", "company")
	require.NoError(t, err)
	require.Len(t, writer.completions, 1)
}

func TestOTelSDKAndHTTPShareExternalIdentity(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	response := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{Label: "SDK", Refs: map[string]string{"threadify.external_ref": "run", "otel_trace_id": "ff02030405060708090a0b0c0d0e0f10"}}, "owner", "company")
	require.Equal(t, StepStatusSuccess, response.Status, response.Message)
	r, err := svc.Ingest(context.Background(), correlatedExport(1, "workflow.run_id", "run"), "owner", "company")
	require.NoError(t, err)
	require.Nil(t, r.PartialSuccess)
	require.Len(t, writer.starts, 1)
	require.Equal(t, response.ThreadID, writer.records[0].ThreadID)
}

func TestOTelCorrelationLookupFailureDoesNotRecreate(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	writer.lookupErr = fmt.Errorf("database unavailable")
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	_, err := svc.Ingest(context.Background(), correlatedExport(1, "workflow.run_id", "run"), "owner", "company")
	require.ErrorContains(t, err, "database unavailable")
	require.Empty(t, writer.starts)
	require.Empty(t, writer.records)
}

func TestOTelCorrelationContractVersionAndCompanyScope(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	for i, company := range []string{"a", "b"} {
		req := correlatedExport(byte(i+1), "workflow.run_id", "run")
		req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = append(req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes, otelKV("threadify.contract", otelString("payment:1")))
		r, err := svc.Ingest(context.Background(), req, "owner", company)
		require.NoError(t, err)
		require.Nil(t, r.PartialSuccess)
	}
	require.Len(t, writer.starts, 2)
	require.NotEqual(t, writer.starts[0].ThreadID, writer.starts[1].ThreadID)
	req := correlatedExport(3, "workflow.run_id", "run")
	req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = append(req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes, otelKV("threadify.contract", otelString("payment:2")))
	r, err := svc.Ingest(context.Background(), req, "owner", "a")
	require.NoError(t, err)
	require.EqualValues(t, 1, r.GetPartialSuccess().GetRejectedSpans())
	require.Len(t, writer.records, 2)
}

func TestOTelLaterSpanWithoutReferenceKeepsSharedThreadOpen(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	_, err := svc.Ingest(context.Background(), correlatedExport(1, "workflow.run_id", "run"), "owner", "company")
	require.NoError(t, err)
	req := correlatedExport(1, "", "")
	req.ResourceSpans[0].ScopeSpans[0].Spans[0].SpanId[0] = 99
	r, err := svc.Ingest(context.Background(), req, "owner", "company")
	require.NoError(t, err)
	require.Nil(t, r.PartialSuccess)
	require.Len(t, writer.starts, 1)
	require.Len(t, writer.records, 2)
	require.Empty(t, writer.completions)
	// The SDK's trace-only fallback must use the existing Engine binding too.
	traceID := fmt.Sprintf("%x", req.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
	response := svc.startSDKThread(context.Background(), &domain.StartThreadCmd{Refs: map[string]string{"otel_trace_id": traceID}}, "owner", "company")
	require.Equal(t, StepStatusSuccess, response.Status, response.Message)
	require.Equal(t, writer.starts[0].ThreadID, response.ThreadID)
}
