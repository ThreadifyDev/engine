package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"threadify-go/api/internal/service"
	"threadify-go/shared/registry"
	"time"

	"github.com/gin-gonic/gin"
)

// safeErrorMessage maps HTTP status codes to generic, user-facing messages.
func safeErrorMessage(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "Unauthorized"
	case status == http.StatusForbidden:
		return "Access denied"
	case status == http.StatusNotFound:
		return "Not found"
	case status == http.StatusBadRequest:
		return "Invalid request"
	case status >= 500:
		return "An internal error occurred"
	default:
		return "Request failed"
	}
}

const entityProfileProxyTimeout = 15 * time.Second

type EntityProfileProxyHandler struct {
	engineGraphQLURL string
	httpClient       *http.Client
}

func NewEntityProfileProxyHandler(engineGraphQLURL string) *EntityProfileProxyHandler {
	return &EntityProfileProxyHandler{
		engineGraphQLURL: engineGraphQLURL,
		httpClient:       &http.Client{Timeout: entityProfileProxyTimeout},
	}
}

func (h *EntityProfileProxyHandler) graphqlRequest(c *gin.Context, query string, variables map[string]interface{}) (json.RawMessage, int, error) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		return nil, http.StatusUnauthorized, errors.New("authorization header required")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.engineGraphQLURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to create request")
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, service.ContentTypeJSON)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	// Authenticate the internal hop so Engine does not count API-metered traffic twice.
	// Production startup always installs a runtime; isolated handler tests may omit it.
	if runtime := registry.Default(); runtime != nil {
		if err := runtime.SignRequest(req); err != nil {
			return nil, http.StatusServiceUnavailable, errors.New("license verification unavailable")
		}
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, errors.New("engine unavailable")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to read response")
	}

	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, errors.New("engine error")
	}

	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, http.StatusInternalServerError, errors.New("invalid engine response")
	}

	return gqlResp.Data, http.StatusOK, nil
}

func (h *EntityProfileProxyHandler) GetEntityProfile(c *gin.Context) {
	refKey := c.Query("refKey")
	typeName := c.Query("type")

	if refKey == "" || typeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refKey and type query params are required"})
		return
	}

	query := `query EntityProfile($refKey: String!, $type: String!) {
		entityProfile(refKey: $refKey, type: $type) {
			id refKey companyId profileTypeId name createdAt lastActiveAt
			metrics {
				entityProfileId totalDeliveries completedSuccessfully
				validationViolations deliveryHealthScore healthTrendSlope
				averageDeliveryTimeMs lastCalculatedAt
			}
		}
	}`

	data, status, err := h.graphqlRequest(c, query, map[string]interface{}{
		"refKey": refKey,
		"type":   typeName,
	})
	if err != nil {
		c.JSON(status, gin.H{"error": safeErrorMessage(status)})
		return
	}

	var result struct {
		EntityProfile json.RawMessage `json:"entityProfile"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse response"})
		return
	}

	if string(result.EntityProfile) == "null" {
		c.JSON(http.StatusNotFound, gin.H{"error": "entity profile not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile fetched successfully.",
		"data":    json.RawMessage(result.EntityProfile),
	})
}

func (h *EntityProfileProxyHandler) ListEntityProfileTypes(c *gin.Context) {
	query := `query {
		entityProfileTypes {
			id companyId name type description createdAt updatedAt
		}
	}`

	data, status, err := h.graphqlRequest(c, query, nil)
	if err != nil {
		c.JSON(status, gin.H{"error": safeErrorMessage(status)})
		return
	}

	var result struct {
		EntityProfileTypes json.RawMessage `json:"entityProfileTypes"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse response"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile types fetched successfully.",
		"data":    json.RawMessage(result.EntityProfileTypes),
	})
}
