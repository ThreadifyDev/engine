package handlers

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	collecttracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.uber.org/zap"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
)

const (
	maxOTLPRequestBytes     = 64 << 20
	maxOTLPSpansPerRequest  = 10_000
	otlpProtobufContentType = "application/x-protobuf"
)

var (
	errOTLPPayloadTooLarge     = errors.New("OTLP request payload is too large")
	errOTLPUnsupportedEncoding = errors.New("unsupported OTLP Content-Encoding")
)

type otelTraceIngester interface {
	Ingest(context.Context, *collecttracepb.ExportTraceServiceRequest, string, string) (*collecttracepb.ExportTraceServiceResponse, error)
}

// OTLPTraceHandler implements the standard OTLP/HTTP trace endpoint.
type OTLPTraceHandler struct {
	ingester otelTraceIngester
	auth     domain.AuthService
	plan     domain.PlanService
	logger   *zap.Logger
}

func NewOTLPTraceHandler(
	ingester otelTraceIngester,
	auth domain.AuthService,
	plan domain.PlanService,
	logger *zap.Logger,
) *OTLPTraceHandler {
	return &OTLPTraceHandler{ingester: ingester, auth: auth, plan: plan, logger: logger}
}

func (h *OTLPTraceHandler) HandleTraces(c *gin.Context) {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		h.writeStatus(c, http.StatusUnauthorized, codes.Unauthenticated, "X-API-Key header required")
		return
	}
	userInfo, err := h.auth.ValidateApiKey(apiKey)
	if err != nil || userInfo == nil || userInfo.OwnerID == "" || userInfo.CompanyID == "" {
		h.writeStatus(c, http.StatusUnauthorized, codes.Unauthenticated, "invalid API key")
		return
	}
	account, err := h.plan.CheckBalancePositive(c.Request.Context(), userInfo.CompanyID)
	if err != nil {
		status := http.StatusServiceUnavailable
		code := codes.Unavailable
		if errors.Is(err, service.ErrNoAccount) || errors.Is(err, service.ErrInsufficientCredit) {
			status = http.StatusPaymentRequired
			code = codes.ResourceExhausted
		}
		h.writeStatus(c, status, code, "trace ingestion is unavailable: "+err.Error())
		return
	}
	if account == nil {
		h.writeStatus(c, http.StatusServiceUnavailable, codes.Unavailable, "trace ingestion is unavailable: billing account details are unavailable")
		return
	}

	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != otlpProtobufContentType {
		h.writeStatus(c, http.StatusUnsupportedMediaType, codes.InvalidArgument, "Content-Type must be application/x-protobuf")
		return
	}

	body, err := readOTLPBody(c.Writer, c.Request)
	if err != nil {
		if errors.Is(err, errOTLPUnsupportedEncoding) {
			h.writeStatus(c, http.StatusUnsupportedMediaType, codes.InvalidArgument, err.Error())
			return
		}
		if errors.Is(err, errOTLPPayloadTooLarge) {
			h.writeStatus(c, http.StatusRequestEntityTooLarge, codes.ResourceExhausted, err.Error())
			return
		}
		h.writeStatus(c, http.StatusBadRequest, codes.InvalidArgument, err.Error())
		return
	}

	req := &collecttracepb.ExportTraceServiceRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		h.writeStatus(c, http.StatusBadRequest, codes.InvalidArgument, "invalid OTLP protobuf payload")
		return
	}
	if countOTLPSpans(req) > maxOTLPSpansPerRequest {
		h.writeStatus(c, http.StatusRequestEntityTooLarge, codes.ResourceExhausted, "OTLP request contains too many spans")
		return
	}

	resp, err := h.ingester.Ingest(c.Request.Context(), req, userInfo.OwnerID, userInfo.CompanyID)
	if err != nil {
		h.logger.Error("OTLP trace ingestion failed", zap.String("company_id", userInfo.CompanyID), zap.Error(err))
		c.Header("Retry-After", "1")
		h.writeStatus(c, http.StatusServiceUnavailable, codes.Unavailable, "trace ingestion temporarily unavailable")
		return
	}
	h.writeProto(c, http.StatusOK, resp)
}

func readOTLPBody(writer http.ResponseWriter, req *http.Request) ([]byte, error) {
	compressed := http.MaxBytesReader(writer, req.Body, maxOTLPRequestBytes)
	defer compressed.Close()

	var reader io.Reader = compressed
	switch strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		gzipReader, err := gzip.NewReader(compressed)
		if err != nil {
			return nil, fmt.Errorf("invalid gzip OTLP payload: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	default:
		return nil, errOTLPUnsupportedEncoding
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxOTLPRequestBytes+1))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errOTLPPayloadTooLarge
		}
		return nil, fmt.Errorf("read OTLP request: %w", err)
	}
	if len(body) > maxOTLPRequestBytes {
		return nil, errOTLPPayloadTooLarge
	}
	return body, nil
}

func countOTLPSpans(req *collecttracepb.ExportTraceServiceRequest) int {
	count := 0
	for _, resourceSpans := range req.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			count += len(scopeSpans.GetSpans())
		}
	}
	return count
}

func (h *OTLPTraceHandler) writeStatus(c *gin.Context, httpStatus int, code codes.Code, message string) {
	h.writeProto(c, httpStatus, &statuspb.Status{Code: int32(code), Message: message})
}

func (h *OTLPTraceHandler) writeProto(c *gin.Context, httpStatus int, message proto.Message) {
	payload, err := proto.Marshal(message)
	if err != nil {
		h.logger.Error("failed to encode OTLP response", zap.Error(err))
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(httpStatus, otlpProtobufContentType, payload)
}
