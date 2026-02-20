package handlers

import (
	"bytes"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type GraphQLProxyHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
}

func NewGraphQLProxyHandler(threadifyEngineURL string) *GraphQLProxyHandler {
	return &GraphQLProxyHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{},
	}
}

// ProxyGraphQL forwards GraphQL requests to ThreadifyEngine
func (h *GraphQLProxyHandler) ProxyGraphQL(c *gin.Context) {
	// Get JWT token from incoming request (already validated by JWT middleware)
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	// Read request body (GraphQL query)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("[GRAPHQL-PROXY-ERROR] Failed to read request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Create request to ThreadifyEngine GraphQL endpoint
	// Note: threadifyEngineURL already includes the full GraphQL endpoint path
	url := h.threadifyEngineURL
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("[GRAPHQL-PROXY-ERROR] Failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Forward authorization header (JWT token)
	// Note: ThreadifyEngine should accept JWT tokens for GraphQL queries from admin users
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Threadify-WebAPI/1.0")

	// Execute request to ThreadifyEngine
	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("[GRAPHQL-PROXY-ERROR] Failed to connect to engine: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Service temporarily unavailable"})
		return
	}
	defer resp.Body.Close()

	// Read response from ThreadifyEngine
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[GRAPHQL-PROXY-ERROR] Failed to read engine response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Log non-2xx responses for debugging but still forward them
	if resp.StatusCode >= 400 {
		log.Printf("[GRAPHQL-PROXY-ERROR] Engine returned error status %d: %s", resp.StatusCode, string(respBody))
	}

	// Forward response to client with same status code and content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)
}
