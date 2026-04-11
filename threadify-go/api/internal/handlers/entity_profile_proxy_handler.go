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
	"go.uber.org/zap"
)

const entityProfileProxyTimeout = 15 * time.Second

type EntityProfileProxyHandler struct {
	engineGraphQLURL string
	httpClient       *http.Client
	logger           *zap.Logger
}

func NewEntityProfileProxyHandler(engineGraphQLURL string, logger *zap.Logger) *EntityProfileProxyHandler {
	return &EntityProfileProxyHandler{
		engineGraphQLURL: engineGraphQLURL,
		httpClient:       &http.Client{Timeout: entityProfileProxyTimeout},
		logger:           logger,
	}
}

func (h *EntityProfileProxyHandler) graphqlRequest(c *gin.Context, query string, variables map[string]interface{}) (json.RawMessage, int, error) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		return nil, http.StatusUnauthorized, fmt.Errorf("authorization header required")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.engineGraphQLURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, service.ContentTypeJSON)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("engine unavailable: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		h.logger.Warn("engine returned error", zap.Int("status", resp.StatusCode), zap.ByteString("body", respBody))
		return nil, resp.StatusCode, fmt.Errorf("engine error")
	}

	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("invalid engine response: %w", err)
	}

	if len(gqlResp.Errors) > 0 && string(gqlResp.Errors) != "null" {
		h.logger.Warn("graphql errors", zap.ByteString("errors", gqlResp.Errors))
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
		c.JSON(status, gin.H{"error": err.Error()})
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
		c.JSON(status, gin.H{"error": err.Error()})
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
