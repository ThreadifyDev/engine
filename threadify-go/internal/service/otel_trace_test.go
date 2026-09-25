package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	collecttracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap"
	shderrors "threadify-go/shared/errors"

	"github.com/threadify/engine/internal/domain"
)

type fakeOTelThreadWriter struct {
	mu                     sync.Mutex
	starts                 []*domain.StartThreadCmd
	records                []*domain.RecordEventCmd
	threads                map[string]struct{}
	completions            []time.Time
	completionRecordCounts []int
	completionErr          error
	lookupErr              error
	statuses               map[string]domain.ThreadStatus
	startResponse          *domain.StartThreadResponse
	recordResponse         *domain.RecordEventResponse
}

func newFakeOTelThreadWriter() *fakeOTelThreadWriter {
	return &fakeOTelThreadWriter{threads: make(map[string]struct{}), statuses: make(map[string]domain.ThreadStatus)}
}

func (f *fakeOTelThreadWriter) CompleteTraceForIngestion(_ context.Context, _, _, _, _ string, endedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.completionErr != nil {
		return f.completionErr
	}
	f.completions = append(f.completions, endedAt)
	f.completionRecordCounts = append(f.completionRecordCounts, len(f.records))
	return nil
}

func (f *fakeOTelThreadWriter) StartThreadForIngestion(_ context.Context, req *domain.StartThreadCmd, _, _ string) *domain.StartThreadResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	copyReq := *req
	f.starts = append(f.starts, &copyReq)
	if f.startResponse != nil {
		return f.startResponse
	}
	f.threads[req.ThreadID] = struct{}{}
	return &domain.StartThreadResponse{Status: StepStatusSuccess, ThreadID: req.ThreadID}
}

func (f *fakeOTelThreadWriter) RecordEventForIngestion(_ context.Context, req *domain.RecordEventCmd, _, _ string) *domain.RecordEventResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	copyReq := *req
	f.records = append(f.records, &copyReq)
	if f.recordResponse != nil {
		return f.recordResponse
	}
	return &domain.RecordEventResponse{Status: StepStatusSuccess, ThreadID: req.ThreadID, StepID: "step-id"}
}

func (f *fakeOTelThreadWriter) ValidateThreadForIngestion(_ context.Context, threadID, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.threads[threadID]; !exists {
		return errors.New("thread not found")
	}
	return nil
}

// LookupThreadForIngestion models the persisted contract used by correlation recovery.
func (f *fakeOTelThreadWriter) LookupThreadForIngestion(_ context.Context, id, _, company string) (*domain.Thread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if _, ok := f.threads[id]; !ok {
		return nil, shderrors.ErrThreadNotFound
	}
	t := &domain.Thread{ID: id, CompanyID: company, Status: domain.ThreadStatusActive}
	if status, ok := f.statuses[id]; ok {
		t.Status = status
	}
	for _, start := range f.starts {
		if start.ThreadID == id {
			t.Label, t.Refs, t.Tags = start.Label, start.Refs, start.Tags
			name, version := parseContractIdentifier(start.ContractName)
			t.ContractName = name
			if version > 0 {
				t.ContractVersion = &version
			}
			break
		}
	}
	return t, nil
}

type fakeOTelCorrelationRepository struct {
	mu          sync.Mutex
	threads     map[string]string
	createLocks map[string]string
	completed   map[string]bool
	spans       map[string]string
	err         error
}

func newFakeOTelCorrelationRepository() *fakeOTelCorrelationRepository {
	return &fakeOTelCorrelationRepository{
		threads:     make(map[string]string),
		completed:   make(map[string]bool),
		createLocks: make(map[string]string),
		spans:       make(map[string]string),
	}
}

func (f *fakeOTelCorrelationRepository) traceKey(companyID, traceID string) string {
	return companyID + ":" + traceID
}

func (f *fakeOTelCorrelationRepository) spanKey(companyID, traceID, spanID string) string {
	return companyID + ":" + traceID + ":" + spanID
}

func (f *fakeOTelCorrelationRepository) GetThreadID(_ context.Context, companyID, traceID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return f.threads[f.traceKey(companyID, traceID)], nil
}

func (f *fakeOTelCorrelationRepository) SetThreadIDIfAbsent(_ context.Context, companyID, traceID, threadID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	key := f.traceKey(companyID, traceID)
	if _, exists := f.threads[key]; exists {
		return false, nil
	}
	f.threads[key] = threadID
	return true, nil
}

func (f *fakeOTelCorrelationRepository) IsTraceCompleted(_ context.Context, companyID, traceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	return f.completed[f.traceKey(companyID, traceID)], nil
}

func (f *fakeOTelCorrelationRepository) MarkTraceCompleted(_ context.Context, companyID, traceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.completed[f.traceKey(companyID, traceID)] = true
	return nil
}

func (f *fakeOTelCorrelationRepository) AcquireCreationLock(_ context.Context, companyID, traceID, token string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	key := f.traceKey(companyID, traceID)
	if _, exists := f.createLocks[key]; exists {
		return false, nil
	}
	f.createLocks[key] = token
	return true, nil
}

func (f *fakeOTelCorrelationRepository) ReleaseCreationLock(_ context.Context, companyID, traceID, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := f.traceKey(companyID, traceID)
	if f.createLocks[key] == token {
		delete(f.createLocks, key)
	}
	return nil
}

func (f *fakeOTelCorrelationRepository) GetSpanState(_ context.Context, companyID, traceID, spanID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return f.spans[f.spanKey(companyID, traceID, spanID)], nil
}

func (f *fakeOTelCorrelationRepository) ClaimSpan(_ context.Context, companyID, traceID, spanID, token string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	key := f.spanKey(companyID, traceID, spanID)
	if _, exists := f.spans[key]; exists {
		return false, nil
	}
	f.spans[key] = token
	return true, nil
}

func (f *fakeOTelCorrelationRepository) CompleteSpan(_ context.Context, companyID, traceID, spanID, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := f.spanKey(companyID, traceID, spanID)
	if f.spans[key] != token {
		return errors.New("claim lost")
	}
	f.spans[key] = "done"
	return nil
}

func (f *fakeOTelCorrelationRepository) ReleaseSpan(_ context.Context, companyID, traceID, spanID, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := f.spanKey(companyID, traceID, spanID)
	if f.spans[key] == token {
		delete(f.spans, key)
	}
	return nil
}

func TestOTelTraceServiceMapsThreadifyAttributes(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	correlations := newFakeOTelCorrelationRepository()
	svc := NewOTelTraceService(writer, correlations, zap.NewNop())

	span := validOTelSpan()
	span.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "payment failed"}
	span.Attributes = []*commonpb.KeyValue{
		otelKV("threadify.step_name", otelString("charge_card")),
		otelKV("threadify.invocation_id", otelString("test-invocation")),
		otelKV("threadify.ref.order_id", otelString("ORD-123")),
		otelKV("threadify.context.region", otelString("eu-west-2")),
		otelKV("attempt", otelInt(2)),
	}
	span.Events = []*tracepb.Span_Event{{
		Name:         "provider_response",
		TimeUnixNano: uint64(time.Date(2026, 8, 27, 12, 0, 1, 123, time.UTC).UnixNano()),
		Attributes:   []*commonpb.KeyValue{otelKV("code", otelString("declined"))},
	}}
	req := otelRequest(span, []*commonpb.KeyValue{
		otelKV("service.name", otelString("checkout-service")),
		otelKV("threadify.tags", otelStringArray("production", " checkout ", "production", "")),
		otelKV("threadify.label", otelString("Checkout trace")),
	})

	resp, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
	require.NoError(t, err)
	require.Nil(t, resp.GetPartialSuccess())
	require.Len(t, writer.starts, 1)
	require.Equal(t, "Checkout trace", writer.starts[0].Label)
	require.Equal(t, "checkout-service", writer.starts[0].ServiceName)
	require.Equal(t, otelTimestamp(span.GetStartTimeUnixNano()), writer.starts[0].StartedAt)
	require.Equal(t, []string{"production", "checkout"}, writer.starts[0].Tags)
	require.Equal(t, "0102030405060708090a0b0c0d0e0f10", writer.starts[0].Refs["otel_trace_id"])

	require.Len(t, writer.records, 1)
	record := writer.records[0]
	require.Equal(t, "charge_card", record.StepName)
	require.Equal(t, "test-invocation", record.InvocationID)
	require.NotContains(t, record.Context, "threadify.invocation_id")
	require.Equal(t, StepStatusFailed, record.Status)
	require.Equal(t, "checkout-service", record.ServiceName)
	require.Equal(t, "ORD-123", record.Refs["order_id"])
	require.Equal(t, "eu-west-2", record.Context["region"])
	require.Equal(t, "2", record.Context["attempt"])
	require.Equal(t, "0203040506070809", record.Context["otel.span_id"])
	require.Equal(t, "payment failed", record.ThreadifyMetadata["message"])
	require.Equal(t, otelTimestamp(span.GetStartTimeUnixNano()), record.StartedAt)
	require.Equal(t, otelTimestamp(span.GetEndTimeUnixNano()), record.FinishedAt)
	require.Len(t, record.SubSteps, 1)
	require.Equal(t, "provider_response", record.SubSteps[0].Name)
	require.Equal(t, "declined", record.SubSteps[0].Payload["code"])
}

func TestOTelTraceServiceUsesEarliestSpanTimeForThreadStart(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())

	root := validOTelSpan()
	root.StartTimeUnixNano = uint64(time.Date(2026, 8, 27, 12, 0, 2, 0, time.UTC).UnixNano())
	root.EndTimeUnixNano = uint64(time.Date(2026, 8, 27, 12, 0, 3, 0, time.UTC).UnixNano())
	child := validOTelSpan()
	child.SpanId = []byte{3, 4, 5, 6, 7, 8, 9, 10}
	child.ParentSpanId = root.SpanId
	child.StartTimeUnixNano = uint64(time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC).UnixNano())
	child.EndTimeUnixNano = uint64(time.Date(2026, 8, 27, 12, 0, 1, 0, time.UTC).UnixNano())
	req := otelRequest(root, nil)
	req.ResourceSpans[0].ScopeSpans[0].Spans = append(req.ResourceSpans[0].ScopeSpans[0].Spans, child)

	resp, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
	require.NoError(t, err)
	require.Nil(t, resp.GetPartialSuccess())
	require.Len(t, writer.starts, 1)
	require.Equal(t, otelTimestamp(child.GetStartTimeUnixNano()), writer.starts[0].StartedAt)
}

func TestRecordEventTimestampUsesProducerFinishedAt(t *testing.T) {
	want := time.Date(2026, 8, 27, 12, 0, 1, 123456789, time.UTC)
	req := &domain.RecordEventCmd{FinishedAt: want.Format(time.RFC3339Nano)}

	require.Equal(t, want, recordEventTimestamp(req))
	require.Equal(t, want, buildStepStateSnapshot(
		"step-1", "thread-1", "pay", "attempt-1", StepStatusSuccess, "owner-1",
		req, 0, "", "",
	).LastUpdatedAt)
}

func TestOTelTraceServiceReplayIsAcceptedWithoutDuplicateWrites(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	correlations := newFakeOTelCorrelationRepository()
	svc := NewOTelTraceService(writer, correlations, zap.NewNop())
	req := otelRequest(validOTelSpan(), nil)

	for range 2 {
		resp, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
		require.NoError(t, err)
		require.Nil(t, resp.GetPartialSuccess())
	}
	require.Len(t, writer.starts, 1)
	require.Len(t, writer.records, 1)
}

func TestOTelTraceServiceConcurrentReplayWritesOnce(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	correlations := newFakeOTelCorrelationRepository()
	svc := NewOTelTraceService(writer, correlations, zap.NewNop())
	req := otelRequest(validOTelSpan(), nil)

	const requests = 8
	var wg sync.WaitGroup
	errs := make(chan error, requests)
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	require.Len(t, writer.starts, 1)
	require.Len(t, writer.records, 1)
}

func TestOTelTraceServiceTreatsTagOrderAsEquivalent(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	first := validOTelSpan()
	second := validOTelSpan()
	second.SpanId = []byte{3, 4, 5, 6, 7, 8, 9, 10}
	second.Attributes = []*commonpb.KeyValue{otelKV("threadify.tags", otelStringArray("beta", "alpha"))}
	req := otelRequest(first, []*commonpb.KeyValue{otelKV("threadify.tags", otelStringArray("alpha", "beta"))})
	req.ResourceSpans[0].ScopeSpans[0].Spans = append(req.ResourceSpans[0].ScopeSpans[0].Spans, second)

	resp, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
	require.NoError(t, err)
	require.Nil(t, resp.GetPartialSuccess())
	require.Len(t, writer.records, 2)
}

func TestOTelTraceServiceRejectsInvalidTagType(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	req := otelRequest(validOTelSpan(), []*commonpb.KeyValue{otelKV("threadify.tags", otelInt(1))})

	resp, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.GetPartialSuccess().GetRejectedSpans())
	require.Contains(t, resp.GetPartialSuccess().GetErrorMessage(), "threadify.tags")
	require.Empty(t, writer.starts)
}

func TestOTelTraceServiceReturnsPartialSuccessForInvalidSpan(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	invalid := validOTelSpan()
	invalid.SpanId = make([]byte, 8)
	req := otelRequest(validOTelSpan(), nil)
	req.ResourceSpans[0].ScopeSpans[0].Spans = append(req.ResourceSpans[0].ScopeSpans[0].Spans, invalid)

	resp, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.GetPartialSuccess().GetRejectedSpans())
	require.Contains(t, resp.GetPartialSuccess().GetErrorMessage(), "invalid span_id")
	require.Len(t, writer.records, 1)
}

func TestOTelTraceServiceReturnsRetryableErrorForCorrelationFailure(t *testing.T) {
	correlations := newFakeOTelCorrelationRepository()
	correlations.err = errors.New("valkey unavailable")
	svc := NewOTelTraceService(newFakeOTelThreadWriter(), correlations, zap.NewNop())

	resp, err := svc.Ingest(context.Background(), otelRequest(validOTelSpan(), nil), "owner-1", "company-1")
	require.ErrorContains(t, err, "valkey unavailable")
	require.Nil(t, resp)
}

func TestOTelTraceServiceReturnsRetryableErrorForBackendWriteFailure(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	writer.recordResponse = &domain.RecordEventResponse{Status: StepStatusError, Message: "failed to process step event"}
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())

	resp, err := svc.Ingest(context.Background(), otelRequest(validOTelSpan(), nil), "owner-1", "company-1")
	require.ErrorContains(t, err, "failed to process step event")
	require.Nil(t, resp)
}

func TestOTelTraceServiceReturnsPartialSuccessForPermanentWriteRejection(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	writer.recordResponse = &domain.RecordEventResponse{Status: StepStatusError, Message: "Step 'pay' not found in contract 'checkout'"}
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())

	resp, err := svc.Ingest(context.Background(), otelRequest(validOTelSpan(), nil), "owner-1", "company-1")
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.GetPartialSuccess().GetRejectedSpans())
	require.Contains(t, resp.GetPartialSuccess().GetErrorMessage(), "not found in contract")
}

func TestOTelTraceServiceScopesCorrelationByCompany(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	req := otelRequest(validOTelSpan(), nil)

	_, err := svc.Ingest(context.Background(), req, "owner-1", "company-1")
	require.NoError(t, err)
	_, err = svc.Ingest(context.Background(), req, "owner-2", "company-2")
	require.NoError(t, err)
	require.Len(t, writer.starts, 2)
	require.NotEqual(t, writer.starts[0].ThreadID, writer.starts[1].ThreadID)
}

func validOTelSpan() *tracepb.Span {
	start := time.Date(2026, 8, 27, 12, 0, 0, 123, time.UTC)
	return &tracepb.Span{
		TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanId:            []byte{2, 3, 4, 5, 6, 7, 8, 9},
		Name:              "process_payment",
		StartTimeUnixNano: uint64(start.UnixNano()),
		EndTimeUnixNano:   uint64(start.Add(time.Second).UnixNano()),
	}
}

func otelRequest(span *tracepb.Span, resourceAttrs []*commonpb.KeyValue) *collecttracepb.ExportTraceServiceRequest {
	return &collecttracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: resourceAttrs},
		ScopeSpans: []*tracepb.ScopeSpans{{
			Scope: &commonpb.InstrumentationScope{Name: "test-instrumentation", Version: "1.0.0"},
			Spans: []*tracepb.Span{span},
		}},
	}}}
}

func otelKV(key string, value *commonpb.AnyValue) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: value}
}

func otelString(value string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
}

func otelInt(value int64) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}
}

func otelStringArray(values ...string) *commonpb.AnyValue {
	items := make([]*commonpb.AnyValue, 0, len(values))
	for _, value := range values {
		items = append(items, otelString(value))
	}
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: items}}}
}

func TestOTelCompletionWaitsForRootAndRetriesAfterSpanDedup(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	root := validOTelSpan()
	child := validOTelSpan()
	child.SpanId = []byte{9, 8, 7, 6, 5, 4, 3, 2}
	child.ParentSpanId = root.SpanId
	_, err := svc.Ingest(context.Background(), otelRequest(child, nil), "owner", "company")
	require.NoError(t, err)
	require.Empty(t, writer.completions)
	writer.completionErr = errors.New("publication interrupted")
	_, err = svc.Ingest(context.Background(), otelRequest(root, nil), "owner", "company")
	require.ErrorContains(t, err, "publication interrupted")
	require.Len(t, writer.records, 2)
	writer.completionErr = nil
	_, err = svc.Ingest(context.Background(), otelRequest(root, nil), "owner", "company")
	require.NoError(t, err)
	require.Len(t, writer.records, 2, "root retry must not write the hashed step again")
	require.Equal(t, []time.Time{time.Unix(0, int64(root.EndTimeUnixNano)).UTC()}, writer.completions)
}

func TestOTelMarkedInvocationCanCompleteWithUpstreamParent(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	span := validOTelSpan()
	span.ParentSpanId = []byte{9, 8, 7, 6, 5, 4, 3, 2}
	span.Attributes = []*commonpb.KeyValue{otelKV("threadify.run.complete", &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}})}
	span.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR}
	_, err := svc.Ingest(context.Background(), otelRequest(span, nil), "owner", "company")
	require.NoError(t, err)
	require.Len(t, writer.completions, 1, "a failed execution still ends; its span preserves failure")
	require.Equal(t, StepStatusFailed, writer.records[0].Status)
}

func TestOTelRejectedRootDoesNotComplete(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	writer.recordResponse = &domain.RecordEventResponse{Status: StepStatusError, Message: "Access denied"}
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	response, err := svc.Ingest(context.Background(), otelRequest(validOTelSpan(), nil), "owner", "company")
	require.NoError(t, err)
	require.EqualValues(t, 1, response.GetPartialSuccess().GetRejectedSpans())
	require.Empty(t, writer.completions)
}

func TestOTelResourceMarkerCannotCompleteEveryChild(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	child := validOTelSpan()
	child.ParentSpanId = []byte{9, 8, 7, 6, 5, 4, 3, 2}
	_, err := svc.Ingest(context.Background(), otelRequest(child, []*commonpb.KeyValue{otelKV("threadify.run.complete", otelString("true"))}), "owner", "company")
	require.NoError(t, err)
	require.Empty(t, writer.completions)
}
