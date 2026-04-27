package handlers

import (
	"bytes"
	"io"
	"net/http"
	"threadify-go/api/internal/service"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	contentType := resp.Header.Get(service.HeaderContentType)
	if contentType == "" {
		contentType = service.ContentTypeJSON
	}
	c.Data(resp.StatusCode, contentType, respBody)
}
