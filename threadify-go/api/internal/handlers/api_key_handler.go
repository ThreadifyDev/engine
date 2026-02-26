package handlers

import (
	"errors"
	"net/http"
	"threadify-go/api/internal/service"

	"github.com/gin-gonic/gin"
)

type APIKeyHandler struct {
	apiKeyService *service.APIKeyService
}

func NewAPIKeyHandler(apiKeyService *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
	}
}

// CreateAPIKey generates a new API key
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyID, _ := c.Get("companyID")

	var req service.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	response, err := h.apiKeyService.CreateAPIKey(userID.(string), companyID.(string), &req)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyNameRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusCreated, response)
}

// ListAPIKeys returns all API keys for the company
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	companyID, _ := c.Get("companyID")

	keys, err := h.apiKeyService.ListAPIKeys(companyID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch API keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": keys,
	})
}

// RevokeAPIKey revokes an API key
func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	keyID := c.Param("id")

	if keyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key ID is required"})
		return
	}

	err := h.apiKeyService.RevokeAPIKey(keyID, companyID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revoked successfully",
	})
}
