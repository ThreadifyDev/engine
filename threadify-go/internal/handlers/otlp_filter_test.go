package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	collecttracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	shareddomain "threadify-go/shared/domain"
	"threadify-go/shared/ingestion"
)

type filterFixture struct {
	filters            []string
	err                error
	evaluated, dropped int
}

func (f *filterFixture) Load(context.Context, string) (ingestion.Settings, error) {
	return ingestion.Settings{Filters: f.filters}, f.err
}
func (f *filterFixture) Save(context.Context, string, string, []string) (ingestion.Settings, error) {
	panic("not used")
}
func (f *filterFixture) Record(_ context.Context, _ string, evaluated, dropped int) error {
	f.evaluated += evaluated
	f.dropped += dropped
	return nil
}
func filterBatch(names ...string) *collecttracepb.ExportTraceServiceRequest {
	spans := make([]*tracepb.Span, 0, len(names))
	for _, name := range names {
		spans = append(spans, &tracepb.Span{Name: name})
	}
	return &collecttracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}}}
}
func TestOTLPFilteringPrecedesIngestion(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		filters                   []string
		loadErr                   error
		wantStatus, kept, dropped int
	}{
		{"partial", []string{"healthcheck", "internal.*"}, nil, 200, 1, 2},
		{"all", []string{"*"}, nil, 200, 0, 3},
		{"disabled", nil, nil, 200, 3, 0},
		{"storage unavailable", nil, errors.New("offline"), 503, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			auth := enginemocks.NewMockAuthService(ctrl)
			plan := enginemocks.NewMockPlanService(ctrl)
			auth.EXPECT().ValidateApiKey("key").Return(&domain.UserInfo{OwnerID: "owner", CompanyID: "company"}, nil)
			plan.EXPECT().CheckBalancePositive(gomock.Any(), "company").Return(&shareddomain.CreditAccount{}, nil)
			calls := 0
			ingester := &fakeOTelTraceIngester{fn: func(_ context.Context, batch *collecttracepb.ExportTraceServiceRequest, owner, company string) (*collecttracepb.ExportTraceServiceResponse, error) {
				calls++
				require.Equal(t, tc.kept, countOTLPSpans(batch))
				return &collecttracepb.ExportTraceServiceResponse{}, nil
			}}
			store := &filterFixture{filters: tc.filters, err: tc.loadErr}
			handler := NewOTLPTraceHandler(ingester, auth, plan, zap.NewNop()).WithIngestionRules(store)
			router := gin.New()
			router.POST("/v1/traces", handler.HandleTraces)
			body, err := proto.Marshal(filterBatch("healthcheck", "internal.cache", "refund"))
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/v1/traces", bytes.NewReader(body))
			req.Header.Set("X-API-Key", "key")
			req.Header.Set("Content-Type", otlpProtobufContentType)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.wantStatus, w.Code)
			if tc.kept == 0 {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.Equal(t, tc.dropped, store.dropped)
			if tc.wantStatus == 200 {
				require.Equal(t, 3, store.evaluated)
				response := &collecttracepb.ExportTraceServiceResponse{}
				require.NoError(t, proto.Unmarshal(w.Body.Bytes(), response))
				require.Nil(t, response.PartialSuccess)
			} else {
				require.Equal(t, "1", w.Header().Get("Retry-After"))
				require.Zero(t, store.evaluated)
			}
		})
	}
}
func TestOTLPFiltersOriginalNamesAndPreservesRetainedSpans(t *testing.T) {
	batch := filterBatch("internal.parent", "refund", "Refund")
	spans := batch.ResourceSpans[0].ScopeSpans[0].Spans
	spans[1].Attributes = []*commonpb.KeyValue{{Key: "threadify.step_name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "internal.mapped"}}}}
	spans[1].ParentSpanId = []byte{1, 2, 3}
	spans[1].Events = []*tracepb.Span_Event{{Name: "receipt"}}
	expected := proto.Clone(spans[1])
	require.Equal(t, 1, filterOTLPSpans(batch, []string{"internal.*"}))
	require.Len(t, batch.ResourceSpans[0].ScopeSpans[0].Spans, 2)
	require.True(t, proto.Equal(expected, batch.ResourceSpans[0].ScopeSpans[0].Spans[0]))
	require.Equal(t, 1, filterOTLPSpans(batch, []string{"refund"}))
	require.Equal(t, "Refund", batch.ResourceSpans[0].ScopeSpans[0].Spans[0].Name)
	require.Equal(t, 1, filterOTLPSpans(batch, []string{"*"}))
	require.Empty(t, batch.ResourceSpans)
}
func TestOTLPFilterDoesNotRunBeforeAuthentication(t *testing.T) {
	handler := NewOTLPTraceHandler(&fakeOTelTraceIngester{}, nil, nil, zap.NewNop()).WithIngestionRules(&filterFixture{err: errors.New("should not reach policy")})
	router := gin.New()
	router.POST("/v1/traces", handler.HandleTraces)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/traces", http.NoBody))
	require.Equal(t, 401, w.Code)
}
