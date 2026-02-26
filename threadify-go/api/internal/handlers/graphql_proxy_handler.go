package handlers

import (
	"bytes"
	"io"
	"net/http"
	"threadify-go/api/internal/service"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	graphQLTimeout = 30 * time.Second
)

type GraphQLProxyHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
	logger             *zap.Logger
}

func NewGraphQLProxyHandler(threadifyEngineURL string, logger *zap.Logger) *GraphQLProxyHandler {
	return &GraphQLProxyHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: graphQLTimeout},
		logger:             logger,
	}
}

// ProxyGraphQL forwards GraphQL requests to ThreadifyEngine
func (h *GraphQLProxyHandler) ProxyGraphQL(c *gin.Context) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.threadifyEngineURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		h.logger.Error("failed to create request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, service.ContentTypeJSON)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.Error("failed to connect to engine", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "Service temporarily unavailable"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("failed to read engine response", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if resp.StatusCode >= 400 {
		h.logger.Warn("engine returned error status",
			zap.Int("status", resp.StatusCode),
			zap.String("path", c.Request.URL.Path),
			zap.Int("response_size", len(respBody)),
			zap.ByteString("request_body", bodyBytes),
			zap.ByteString("response_body", respBody),
		)
	}

	contentType := resp.Header.Get(service.HeaderContentType)
	if contentType == "" {
		contentType = service.ContentTypeJSON
	}
	c.Data(resp.StatusCode, contentType, respBody)
}
