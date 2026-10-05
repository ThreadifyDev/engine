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
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	shareddomain "threadify-go/shared/domain"
	"threadify-go/shared/ingestion"
)

type filterFixture struct {
	filters            []string
	mode               string
	err                error
	evaluated, dropped int
}

func (f *filterFixture) Load(context.Context, string) (ingestion.Settings, error) {
	return ingestion.Settings{Filters: f.filters, Mode: f.mode}, f.err
}
func (f *filterFixture) Save(context.Context, string, string, []string, []string) (ingestion.Settings, error) {
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

// Filtering needs resolved thread context, so the HTTP boundary must pass the
// original batch through even when the general-thread store is unavailable.
func TestOTLPFilteringDelegatesToIngester(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := enginemocks.NewMockAuthService(ctrl)
	plan := enginemocks.NewMockPlanService(ctrl)
	auth.EXPECT().ValidateApiKey("key").Return(&domain.UserInfo{OwnerID: "owner", CompanyID: "company"}, nil)
	plan.EXPECT().CheckBalancePositive(gomock.Any(), "company").Return(&shareddomain.CreditAccount{}, nil)
	calls := 0
	ingester := &fakeOTelTraceIngester{fn: func(ctx context.Context, batch *collecttracepb.ExportTraceServiceRequest, owner, company string) (*collecttracepb.ExportTraceServiceResponse, error) {
		calls++
		require.Equal(t, 3, countOTLPSpans(batch))
		return &collecttracepb.ExportTraceServiceResponse{}, nil
	}}
	store := &filterFixture{err: errors.New("offline")}
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
	require.Equal(t, 200, w.Code)
	require.Equal(t, 1, calls)
	require.Equal(t, "0", w.Header().Get("X-Threadify-Filtered-Spans"))
	require.Zero(t, store.evaluated)
}
func TestOTLPFilterDoesNotRunBeforeAuthentication(t *testing.T) {
	handler := NewOTLPTraceHandler(&fakeOTelTraceIngester{}, nil, nil, zap.NewNop()).WithIngestionRules(&filterFixture{err: errors.New("should not reach policy")})
	router := gin.New()
	router.POST("/v1/traces", handler.HandleTraces)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/traces", http.NoBody))
	require.Equal(t, 401, w.Code)
}
