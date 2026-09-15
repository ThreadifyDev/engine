package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (h *ContractProxyHandler) proxyRequest(c *gin.Context, method, path, contentType string, body interface{}) (*http.Response, []byte, error) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return nil, nil, fmt.Errorf("unauthorized")
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
				return nil, nil, err
			}
			reqBody = bytes.NewBuffer(jsonData)
		}
	}

	if contentType == "" {
		contentType = service.ContentTypeJSON
	}

	url := fmt.Sprintf("%s%s", h.threadifyEngineURL, path)

	req, err := http.NewRequestWithContext(c.Request.Context(), method, url, reqBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return nil, nil, err
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, contentType)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to ThreadifyEngine"})
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return resp, nil, err
	}

	return resp, respBody, nil
}

func (h *ContractProxyHandler) proxyRawBody(c *gin.Context, method, path, defaultContentType string) (*http.Response, []byte, error) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return nil, nil, err
	}

	contentType := c.GetHeader(service.HeaderContentType)
	if contentType == "" {
		contentType = defaultContentType
	}

	return h.proxyRequest(c, method, path, contentType, bodyBytes)
}



func (h *ContractProxyHandler) GetAllContracts(c *gin.Context) {
	path := contractProxyPath
	if c.Request.URL.RawQuery != "" {
		path = fmt.Sprintf("%s?%s", contractProxyPath, c.Request.URL.RawQuery)
	}
	resp, body, err := h.proxyRequest(c, http.MethodGet, path, "", nil)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(http.StatusOK, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) CreateContract(c *gin.Context) {
	resp, body, err := h.proxyRawBody(c, http.MethodPost, contractProxyPath, contentTypeTextPlain)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) PreviewContract(c *gin.Context) {
	resp, body, err := h.proxyRawBody(c, http.MethodPost, contractProxyPreviewPath, contentTypeTextPlain)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) GetContract(c *gin.Context) {
	resp, body, err := h.proxyRequest(c, http.MethodGet, fmt.Sprintf(contractProxyPath+"/%s", c.Param("id")), "", nil)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) UpdateContract(c *gin.Context) {
	resp, body, err := h.proxyRawBody(c, http.MethodPut, fmt.Sprintf(contractProxyPath+"/%s", c.Param("id")), contentTypeTextPlain)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) DeleteContract(c *gin.Context) {
	resp, body, err := h.proxyRequest(c, http.MethodDelete, fmt.Sprintf("/v1/contracts/%s", c.Param("id")), "", nil)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) GetAllContractVersions(c *gin.Context) {
	resp, body, err := h.proxyRequest(c, http.MethodGet, fmt.Sprintf("/v1/contracts/%s/versions", c.Param("id")), "", nil)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) GetContractVersion(c *gin.Context) {
	resp, body, err := h.proxyRequest(c, http.MethodGet, fmt.Sprintf(contractProxyVersionPath, c.Param("id"), c.Param("version")), "", nil)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}

func (h *ContractProxyHandler) DeleteContractVersion(c *gin.Context) {
	resp, body, err := h.proxyRequest(c, http.MethodDelete, fmt.Sprintf(contractProxyVersionPath, c.Param("id"), c.Param("version")), "", nil)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			c.Data(resp.StatusCode, service.ContentTypeJSON, body)
		}
		return
	}
	c.Data(resp.StatusCode, service.ContentTypeJSON, body)
}
