package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	shareddomain "threadify-go/shared/domain"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	collecttracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

type fakeOTelTraceIngester struct {
	fn func(context.Context, *collecttracepb.ExportTraceServiceRequest, string, string) (*collecttracepb.ExportTraceServiceResponse, error)
}

func (f *fakeOTelTraceIngester) Ingest(
	ctx context.Context,
	req *collecttracepb.ExportTraceServiceRequest,
	ownerID, companyID string,
) (*collecttracepb.ExportTraceServiceResponse, error) {
	return f.fn(ctx, req, ownerID, companyID)
}

func TestOTLPTraceHandlerAcceptsProtobufAndGzip(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip"} {
		t.Run(encoding, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			auth := enginemocks.NewMockAuthService(ctrl)
			plan := enginemocks.NewMockPlanService(ctrl)
			auth.EXPECT().ValidateApiKey("key-1").Return(&domain.UserInfo{OwnerID: "owner-1", CompanyID: "company-1"}, nil)
			plan.EXPECT().CheckBalancePositive(gomock.Any(), "company-1").Return(&shareddomain.CreditAccount{}, nil)

			ingester := &fakeOTelTraceIngester{fn: func(
				_ context.Context,
				req *collecttracepb.ExportTraceServiceRequest,
				ownerID, companyID string,
			) (*collecttracepb.ExportTraceServiceResponse, error) {
				require.NotNil(t, req)
				require.Equal(t, "owner-1", ownerID)
				require.Equal(t, "company-1", companyID)
				return &collecttracepb.ExportTraceServiceResponse{}, nil
			}}
			handler := NewOTLPTraceHandler(ingester, auth, plan, zap.NewNop())
			router := gin.New()
			router.POST("/v1/traces", handler.HandleTraces)

			payload, err := proto.Marshal(&collecttracepb.ExportTraceServiceRequest{})
			require.NoError(t, err)
			if encoding == "gzip" {
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				_, err = writer.Write(payload)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				payload = compressed.Bytes()
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(payload))
			req.Header.Set("Content-Type", otlpProtobufContentType)
			req.Header.Set("X-API-Key", "key-1")
			if encoding == "gzip" {
				req.Header.Set("Content-Encoding", "gzip")
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			require.Equal(t, http.StatusOK, resp.Code)
			require.Equal(t, otlpProtobufContentType, resp.Header().Get("Content-Type"))
			decoded := &collecttracepb.ExportTraceServiceResponse{}
			require.NoError(t, proto.Unmarshal(resp.Body.Bytes(), decoded))
		})
	}
}

func TestOTLPTraceHandlerReturnsProtobufAuthenticationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := enginemocks.NewMockAuthService(ctrl)
	plan := enginemocks.NewMockPlanService(ctrl)
	handler := NewOTLPTraceHandler(&fakeOTelTraceIngester{}, auth, plan, zap.NewNop())
	router := gin.New()
	router.POST("/v1/traces", handler.HandleTraces)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", http.NoBody)
	req.Header.Set("Content-Type", otlpProtobufContentType)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusUnauthorized, resp.Code)
	decoded := &statuspb.Status{}
	require.NoError(t, proto.Unmarshal(resp.Body.Bytes(), decoded))
	require.Contains(t, decoded.GetMessage(), "X-API-Key")
}

func TestOTLPTraceHandlerRejectsMalformedPayload(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := enginemocks.NewMockAuthService(ctrl)
	plan := enginemocks.NewMockPlanService(ctrl)
	auth.EXPECT().ValidateApiKey("key-1").Return(&domain.UserInfo{OwnerID: "owner-1", CompanyID: "company-1"}, nil)
	plan.EXPECT().CheckBalancePositive(gomock.Any(), "company-1").Return(&shareddomain.CreditAccount{}, nil)
	handler := NewOTLPTraceHandler(&fakeOTelTraceIngester{}, auth, plan, zap.NewNop())
	router := gin.New()
	router.POST("/v1/traces", handler.HandleTraces)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewBuffer([]byte{0xff, 0xff}))
	req.Header.Set("Content-Type", otlpProtobufContentType)
	req.Header.Set("X-API-Key", "key-1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusBadRequest, resp.Code)
	decoded := &statuspb.Status{}
	require.NoError(t, proto.Unmarshal(resp.Body.Bytes(), decoded))
	require.Contains(t, decoded.GetMessage(), "invalid OTLP")
}

func TestOTLPTraceHandlerRejectsUnsupportedContentEncoding(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := enginemocks.NewMockAuthService(ctrl)
	plan := enginemocks.NewMockPlanService(ctrl)
	auth.EXPECT().ValidateApiKey("key-1").Return(&domain.UserInfo{OwnerID: "owner-1", CompanyID: "company-1"}, nil)
	plan.EXPECT().CheckBalancePositive(gomock.Any(), "company-1").Return(&shareddomain.CreditAccount{}, nil)
	handler := NewOTLPTraceHandler(&fakeOTelTraceIngester{}, auth, plan, zap.NewNop())
	router := gin.New()
	router.POST("/v1/traces", handler.HandleTraces)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", http.NoBody)
	req.Header.Set("Content-Type", otlpProtobufContentType)
	req.Header.Set("Content-Encoding", "br")
	req.Header.Set("X-API-Key", "key-1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusUnsupportedMediaType, resp.Code)
	decoded := &statuspb.Status{}
	require.NoError(t, proto.Unmarshal(resp.Body.Bytes(), decoded))
	require.Contains(t, decoded.GetMessage(), "Content-Encoding")
}

func TestOTLPTraceHandlerReturnsRetryableServiceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := enginemocks.NewMockAuthService(ctrl)
	plan := enginemocks.NewMockPlanService(ctrl)
	auth.EXPECT().ValidateApiKey("key-1").Return(&domain.UserInfo{OwnerID: "owner-1", CompanyID: "company-1"}, nil)
	plan.EXPECT().CheckBalancePositive(gomock.Any(), "company-1").Return(&shareddomain.CreditAccount{}, nil)
	ingester := &fakeOTelTraceIngester{fn: func(context.Context, *collecttracepb.ExportTraceServiceRequest, string, string) (*collecttracepb.ExportTraceServiceResponse, error) {
		return nil, errors.New("valkey unavailable")
	}}
	handler := NewOTLPTraceHandler(ingester, auth, plan, zap.NewNop())
	router := gin.New()
	router.POST("/v1/traces", handler.HandleTraces)
	payload, err := proto.Marshal(&collecttracepb.ExportTraceServiceRequest{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(payload))
	req.Header.Set("Content-Type", otlpProtobufContentType)
	req.Header.Set("X-API-Key", "key-1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusServiceUnavailable, resp.Code)
	require.Equal(t, "1", resp.Header().Get("Retry-After"))
}

// Query options are strict so a typo cannot silently merge traces.
func TestOTLPTraceHandlerWorkflowOption(t *testing.T) {
	for _, query := range []string{"", "?use_workflow_run_id=true", "?use_workflow_run_id=false", "?use_workflow_run_id=maybe", "?use_workflow_run_id=false&use_workflow_run_id=true"} {
		t.Run(query, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			auth := enginemocks.NewMockAuthService(ctrl)
			plan := enginemocks.NewMockPlanService(ctrl)
			valid := query == "" || query == "?use_workflow_run_id=true" || query == "?use_workflow_run_id=false"
			if valid {
				auth.EXPECT().ValidateApiKey("key").Return(&domain.UserInfo{OwnerID: "owner", CompanyID: "company"}, nil)
				plan.EXPECT().CheckBalancePositive(gomock.Any(), "company").Return(&shareddomain.CreditAccount{}, nil)
			}
			ingester := &fakeOTelTraceIngester{fn: func(ctx context.Context, _ *collecttracepb.ExportTraceServiceRequest, _, _ string) (*collecttracepb.ExportTraceServiceResponse, error) {
				require.Equal(t, query != "?use_workflow_run_id=false", service.OTelUseWorkflowRunID(ctx))
				return &collecttracepb.ExportTraceServiceResponse{}, nil
			}}
			router := gin.New()
			router.POST("/v1/traces", NewOTLPTraceHandler(ingester, auth, plan, zap.NewNop()).HandleTraces)
			req := httptest.NewRequest("POST", "/v1/traces"+query, bytes.NewReader(nil))
			req.Header.Set("X-API-Key", "key")
			req.Header.Set("Content-Type", otlpProtobufContentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if valid {
				require.Equal(t, 200, response.Code)
			} else {
				require.Equal(t, 400, response.Code)
			}
		})
	}
}
