package handlers

import (
	"bytes"
	"io"
	"net/http"
	"threadify-go/api/internal/service"
	"threadify-go/shared/registry"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	graphQLTimeout = 30 * time.Second
)

type GraphQLProxyHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
}

func NewGraphQLProxyHandler(threadifyEngineURL string) *GraphQLProxyHandler {
	return &GraphQLProxyHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: graphQLTimeout},
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.threadifyEngineURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, service.ContentTypeJSON)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	// Authenticate the internal hop so Engine does not count API-metered traffic twice.
	// Production startup always installs a runtime; isolated handler tests may omit it.
	if runtime := registry.Default(); runtime != nil {
		if err := runtime.SignRequest(req); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "License verification unavailable"})
			return
		}
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Service temporarily unavailable"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if resp.StatusCode >= 400 {
		// Preserve the engine's status code but return a generic message.
		// Do not forward the raw engine response body to avoid leaking internals.
		c.JSON(resp.StatusCode, gin.H{"error": "Request could not be completed"})
		return
	}

	contentType := resp.Header.Get(service.HeaderContentType)
	if contentType == "" {
		contentType = service.ContentTypeJSON
	}
	c.Data(resp.StatusCode, contentType, respBody)
}
