package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ContractProxyHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
}

func NewContractProxyHandler(threadifyEngineURL string) *ContractProxyHandler {
	return &ContractProxyHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{},
	}
}

// proxyRequest forwards the request to ThreadifyEngine with JWT passthrough
func (h *ContractProxyHandler) proxyRequest(c *gin.Context, method, path string, body interface{}) {
	// Get JWT token from incoming request
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	// Prepare request body
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare request"})
			return
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// Create request to ThreadifyEngine
	url := fmt.Sprintf("%s%s", h.threadifyEngineURL, path)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Forward JWT token from incoming request to Engine
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to ThreadifyEngine"})
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	// Forward response
	c.Data(resp.StatusCode, "application/json", respBody)
}

// GetAllContracts - GET /api/contracts
func (h *ContractProxyHandler) GetAllContracts(c *gin.Context) {
	h.proxyRequest(c, "GET", "/v1/contracts", nil)
}

// CreateContract - POST /api/contracts
func (h *ContractProxyHandler) CreateContract(c *gin.Context) {
	// Get JWT token from incoming request
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	// Read raw body (YAML)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Create request to ThreadifyEngine
	url := fmt.Sprintf("%s/v1/contracts", h.threadifyEngineURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Forward headers
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", c.GetHeader("Content-Type")) // Forward original content type

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to ThreadifyEngine"})
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	// Forward response
	c.Data(resp.StatusCode, "application/json", respBody)
}

// PreviewContract - POST /api/contracts/preview
func (h *ContractProxyHandler) PreviewContract(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	h.proxyRequest(c, "POST", "/v1/contracts/preview", body)
}

// GetContract - GET /api/contracts/:id
func (h *ContractProxyHandler) GetContract(c *gin.Context) {
	id := c.Param("id")
	path := fmt.Sprintf("/v1/contracts/%s", id)
	h.proxyRequest(c, "GET", path, nil)
}

// UpdateContract - PUT /api/contracts/:id
func (h *ContractProxyHandler) UpdateContract(c *gin.Context) {
	id := c.Param("id")

	// Get JWT token from incoming request
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	// Read raw body (YAML)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to read request body: %v", err)})
		return
	}

	// Log the request for debugging
	fmt.Printf("[PROXY] UpdateContract - ID: %s, Body length: %d bytes, Content-Type: %s\n",
		id, len(bodyBytes), c.GetHeader("Content-Type"))

	// Create request to ThreadifyEngine
	url := fmt.Sprintf("%s/v1/contracts/%s", h.threadifyEngineURL, id)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create request: %v", err)})
		return
	}

	// Forward headers
	req.Header.Set("Authorization", authHeader)
	contentType := c.GetHeader("Content-Type")
	if contentType == "" {
		contentType = "text/plain" // Default for YAML
	}
	req.Header.Set("Content-Type", contentType)

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to connect to ThreadifyEngine: %v", err)})
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read response: %v", err)})
		return
	}

	// Log the response for debugging
	fmt.Printf("[PROXY] UpdateContract - Response status: %d, Body: %s\n", resp.StatusCode, string(respBody))

	// Forward response with exact status code and body from Engine
	c.Data(resp.StatusCode, "application/json", respBody)
}

// DeleteContract - DELETE /api/contracts/:id
func (h *ContractProxyHandler) DeleteContract(c *gin.Context) {
	id := c.Param("id")
	path := fmt.Sprintf("/v1/contracts/%s", id)
	h.proxyRequest(c, "DELETE", path, nil)
}

// GetAllContractVersions - GET /api/contracts/:id/versions
func (h *ContractProxyHandler) GetAllContractVersions(c *gin.Context) {
	id := c.Param("id")
	path := fmt.Sprintf("/v1/contracts/%s/versions", id)
	h.proxyRequest(c, "GET", path, nil)
}

// GetContractVersion - GET /api/contracts/:id/versions/:version
func (h *ContractProxyHandler) GetContractVersion(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")
	path := fmt.Sprintf("/v1/contracts/%s/versions/%s", id, version)
	h.proxyRequest(c, "GET", path, nil)
}

// DeleteContractVersion - DELETE /api/contracts/:id/versions/:version
func (h *ContractProxyHandler) DeleteContractVersion(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")
	path := fmt.Sprintf("/v1/contracts/%s/versions/%s", id, version)
	h.proxyRequest(c, "DELETE", path, nil)
}
