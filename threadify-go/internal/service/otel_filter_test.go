package service

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"threadify-go/shared/ingestion"
)

type traceFilterStore struct {
	settings                  ingestion.Settings
	err                       error
	loads, evaluated, dropped int
}

func (f *traceFilterStore) Load(context.Context, string) (ingestion.Settings, error) {
	f.loads++
	return f.settings, f.err
}
func (f *traceFilterStore) Save(context.Context, string, string, []string, []string) (ingestion.Settings, error) {
	panic("unused")
}
func (f *traceFilterStore) Record(_ context.Context, _ string, evaluated, dropped int) error {
	f.evaluated += evaluated
	f.dropped += dropped
	return nil
}

func TestOTelGeneralThreadFilters(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		filters    []string
		kept       int
		err        error
	}{
		{"partial", ingestion.ModeInclude, []string{"healthcheck", "internal.*"}, 2, nil},
		{"all", ingestion.ModeInclude, []string{"*"}, 3, nil},
		{"empty", ingestion.ModeInclude, nil, 0, nil},
		{"legacy", ingestion.ModeExcludeLegacy, []string{"healthcheck", "internal.*"}, 1, nil},
		{"offline", ingestion.ModeInclude, nil, 0, errors.New("offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := newFakeOTelThreadWriter()
			svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
			batch := otelRequest(validOTelSpan(), nil)
			scope := batch.ResourceSpans[0].ScopeSpans[0]
			scope.Spans = nil
			for i, name := range []string{"healthcheck", "internal.cache", "refund"} {
				span := validOTelSpan()
				span.Name = name
				span.SpanId[0] = byte(i + 1)
				span.Attributes = []*commonpb.KeyValue{otelKV("threadify.step_name", otelString("mapped"))}
				scope.Spans = append(scope.Spans, span)
			}
			before := proto.Clone(batch)
			store := &traceFilterStore{settings: ingestion.Settings{Mode: tc.mode, Filters: tc.filters}, err: tc.err}
			ctx, policy := WithOTelIngestionRules(context.Background(), store)
			resp, err := svc.Ingest(ctx, batch, "owner", "company")
			if tc.err != nil {
				require.Error(t, err)
				require.Zero(t, store.evaluated)
			} else {
				require.NoError(t, err)
				require.Nil(t, resp.PartialSuccess)
				require.Equal(t, 3, store.evaluated)
				require.Equal(t, 3-tc.kept, store.dropped)
				require.Equal(t, store.dropped, policy.Dropped)
			}
			require.Len(t, writer.records, tc.kept)
			if tc.kept == 0 {
				require.Empty(t, writer.starts)
			}
			require.True(t, proto.Equal(before, batch), "filter must not mutate the export")
		})
	}
}

func TestOTelContractThreadsBypassGeneralFilters(t *testing.T) {
	for _, target := range []string{"new contract", "explicit ID", "trace binding", "expired binding", "thread key", "workflow run"} {
		t.Run(target, func(t *testing.T) {
			writer := newFakeOTelThreadWriter()
			correlations := newFakeOTelCorrelationRepository()
			svc := NewOTelTraceService(writer, correlations, zap.NewNop())
			span := validOTelSpan()
			traceID := hex.EncodeToString(span.TraceId)
			id := correlatedThreadID("company", traceID)
			attrs := []*commonpb.KeyValue{}
			switch target {
			case "new contract":
				attrs = append(attrs, otelKV("threadify.contract", otelString("orders:1")))
			case "explicit ID":
				id = "existing"
				attrs = append(attrs, otelKV("threadify.thread_id", otelString(id)))
			case "trace binding":
				id = "bound"
				_, err := correlations.SetThreadIDIfAbsent(context.Background(), "company", traceID, id)
				require.NoError(t, err)
			case "thread key":
				id = correlatedThreadID("company", threadKeyCorrelationID("order-1"))
				attrs = append(attrs, otelKV("threadify.thread_key", otelString("order-1")))
			case "workflow run":
				id = correlatedThreadID("company", threadKeyCorrelationID("run-1"))
				attrs = append(attrs, otelKV("workflow.run_id", otelString("run-1")))
			}
			if target != "new contract" {
				writer.threads[id] = struct{}{}
				writer.starts = append(writer.starts, &domain.StartThreadCmd{ThreadID: id, ContractName: "orders:1"})
			}
			store := &traceFilterStore{err: errors.New("general settings unavailable")}
			ctx, policy := WithOTelIngestionRules(context.Background(), store)
			resp, err := svc.Ingest(ctx, otelRequest(span, attrs), "owner", "company")
			require.NoError(t, err)
			require.Nil(t, resp.PartialSuccess)
			require.Len(t, writer.records, 1)
			require.Equal(t, id, writer.records[0].ThreadID)
			require.Zero(t, store.loads)
			require.Zero(t, policy.Dropped)
			require.Zero(t, store.evaluated)
		})
	}
}

func TestOTelMixedBatchKeepsContractInputsAndFiltersOnlyGeneralSpans(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	root := validOTelSpan()
	root.Name = "root"
	child := validOTelSpan()
	child.SpanId[0]++
	child.ParentSpanId = root.SpanId
	child.Attributes = []*commonpb.KeyValue{otelKV("threadify.contract", otelString("orders:1"))}
	batch := otelRequest(root, nil)
	batch.ResourceSpans[0].ScopeSpans[0].Spans = append(batch.ResourceSpans[0].ScopeSpans[0].Spans, child)
	general := validOTelSpan()
	general.TraceId[0]++
	batch.ResourceSpans = append(batch.ResourceSpans, otelRequest(general, nil).ResourceSpans...)
	store := &traceFilterStore{settings: ingestion.Settings{Mode: ingestion.ModeInclude}}
	ctx, policy := WithOTelIngestionRules(context.Background(), store)
	response, err := svc.Ingest(ctx, batch, "owner", "company")
	require.NoError(t, err)
	require.Nil(t, response.PartialSuccess)
	require.Len(t, writer.starts, 1)
	require.Equal(t, "orders:1", writer.starts[0].ContractName)
	require.Len(t, writer.records, 2)
	require.Equal(t, 1, policy.Dropped)
	require.Equal(t, 1, store.evaluated)
	require.Equal(t, 1, store.loads)
}

func TestOTelDropSpanFilteringBeforeThreadCreation(t *testing.T) {
	for _, tc := range []struct {
		name, spanName, contract string
		keep                     bool
	}{
		{"matching span", "POST /graphql", "", false},
		{"matching prefix", "POST /graphql/admin", "", false},
		{"different span", "checkout", "", true},
		{"contract bypass", "POST /graphql", "orders:1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := newFakeOTelThreadWriter()
			svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
			span := validOTelSpan()
			span.Name = tc.spanName
			// Display overrides and URL attributes must not affect original-name matching.
			span.Attributes = append(span.Attributes, otelKV("threadify.step_name", otelString("POST /graphql")), otelKV("url.path", otelString("/graphql")))
			var attrs []*commonpb.KeyValue
			if tc.contract != "" {
				attrs = append(attrs, otelKV("threadify.contract", otelString(tc.contract)))
			}
			store := &traceFilterStore{settings: ingestion.Settings{Mode: ingestion.ModeInclude, Filters: []string{"*"}, Exclude: []string{"POST /graphql*"}}}
			ctx, policy := WithOTelIngestionRules(context.Background(), store)
			batch := otelRequest(span, attrs)
			original := proto.Clone(batch)
			response, err := svc.Ingest(ctx, batch, "owner", "company")
			require.NoError(t, err)
			require.Nil(t, response.PartialSuccess)
			require.True(t, proto.Equal(original, batch))
			if tc.keep {
				require.Len(t, writer.records, 1)
				require.Zero(t, policy.Dropped)
			} else {
				require.Empty(t, writer.starts)
				require.Empty(t, writer.completions)
				require.Equal(t, 1, policy.Dropped)
			}
			if tc.contract != "" {
				require.Zero(t, store.loads)
				require.Zero(t, store.evaluated)
			} else {
				require.Equal(t, 1, store.evaluated)
			}
		})
	}
}

func TestOTelRegexFiltering(t *testing.T) {
	writer := newFakeOTelThreadWriter()
	svc := NewOTelTraceService(writer, newFakeOTelCorrelationRepository(), zap.NewNop())
	span := validOTelSpan()
	span.Name = "post /GraphQL"
	store := &traceFilterStore{settings: ingestion.Settings{Mode: ingestion.ModeInclude, Filters: []string{`regex:(?i)^POST /`}, Exclude: []string{`regex:(?i)graphql`}}}
	ctx, policy := WithOTelIngestionRules(context.Background(), store)
	response, err := svc.Ingest(ctx, otelRequest(span, nil), "owner", "company")
	require.NoError(t, err)
	require.Nil(t, response.PartialSuccess)
	require.Empty(t, writer.starts)
	require.Empty(t, writer.records)
	require.Equal(t, 1, policy.Dropped)
	require.Equal(t, 1, store.loads)
	// Invalid persisted expressions cannot silently admit a batch.
	store.settings.Exclude = []string{"regex:["}
	ctx, policy = WithOTelIngestionRules(context.Background(), store)
	_, err = svc.Ingest(ctx, otelRequest(span, nil), "owner", "company")
	require.ErrorContains(t, err, "invalid regex")
	require.Zero(t, policy.evaluated)
	require.Empty(t, writer.starts)
}
