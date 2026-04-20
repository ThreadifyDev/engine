package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"threadify-go/api/internal/service"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	contractProxyTimeout      = 30 * time.Second
	contractProxyPath         = "/v1/contracts"
	contentTypeTextPlain      = "text/plain"
	contractProxyPreviewPath  = "/v1/contracts/preview"
	contractProxyVersionsPath = "/v1/contracts/%s/versions"
	contractProxyVersionPath  = "/v1/contracts/%s/versions/%s"
)

type ContractProxyHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
}

func NewContractProxyHandler(threadifyEngineURL string) *ContractProxyHandler {
	return &ContractProxyHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: contractProxyTimeout},
	}
}

func (h *ContractProxyHandler) proxyRequest(c *gin.Context, method, path, contentType string, body interface{}) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	var reqBody io.Reader
	if body != nil {
		switch v := body.(type) {
		case []byte:
			reqBody = bytes.NewBuffer(v)
		default:
			jsonData, err := json.Marshal(v)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare request"})
				return
			}
			reqBody = bytes.NewBuffer(jsonData)
		}
	}

	if contentType == "" {
		contentType = service.ContentTypeJSON
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), method, h.threadifyEngineURL+path, reqBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, contentType)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to ThreadifyEngine"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	respContentType := resp.Header.Get(service.HeaderContentType)
	if respContentType == "" {
		respContentType = service.ContentTypeJSON
	}
	c.Data(resp.StatusCode, respContentType, respBody)
}

// proxyRawBody reads the raw request body and proxies it, preserving the incoming Content-Type.
func (h *ContractProxyHandler) proxyRawBody(c *gin.Context, method, path, defaultContentType string) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	contentType := c.GetHeader(service.HeaderContentType)
	if contentType == "" {
		contentType = defaultContentType
	}

	h.proxyRequest(c, method, path, contentType, bodyBytes)
}

func (h *ContractProxyHandler) GetAllContracts(c *gin.Context) {
	h.proxyRequest(c, http.MethodGet, contractProxyPath, "", nil)
}

func (h *ContractProxyHandler) CreateContract(c *gin.Context) {
	h.proxyRawBody(c, http.MethodPost, contractProxyPath, contentTypeTextPlain)
}

func (h *ContractProxyHandler) PreviewContract(c *gin.Context) {
	if ct := c.GetHeader(service.HeaderContentType); !strings.HasPrefix(ct, "application/json") {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Content-Type must be application/json"})
		return
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	h.proxyRequest(c, http.MethodPost, contractProxyPreviewPath, "", body)
}

func (h *ContractProxyHandler) GetContract(c *gin.Context) {
	h.proxyRequest(c, http.MethodGet, fmt.Sprintf(contractProxyPath+"/%s", c.Param("id")), "", nil)
}

func (h *ContractProxyHandler) UpdateContract(c *gin.Context) {
	h.proxyRawBody(c, http.MethodPut, fmt.Sprintf(contractProxyPath+"/%s", c.Param("id")), contentTypeTextPlain)
}

func (h *ContractProxyHandler) DeleteContract(c *gin.Context) {
	h.proxyRequest(c, http.MethodDelete, fmt.Sprintf("/v1/contracts/%s", c.Param("id")), "", nil)
}

func (h *ContractProxyHandler) GetAllContractVersions(c *gin.Context) {
	h.proxyRequest(c, http.MethodGet, fmt.Sprintf("/v1/contracts/%s/versions", c.Param("id")), "", nil)
}

func (h *ContractProxyHandler) GetContractVersion(c *gin.Context) {
	h.proxyRequest(c, http.MethodGet, fmt.Sprintf(contractProxyVersionPath, c.Param("id"), c.Param("version")), "", nil)
}

func (h *ContractProxyHandler) DeleteContractVersion(c *gin.Context) {
	h.proxyRequest(c, http.MethodDelete, fmt.Sprintf(contractProxyVersionPath, c.Param("id"), c.Param("version")), "", nil)
}
