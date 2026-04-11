package handlers

import (
	"net/http"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type APIKeyHandler struct {
	apiKeyService *service.APIKeyService
	userRepo      *repository.UserRepository
}

func NewAPIKeyHandler(apiKeyService *service.APIKeyService, userRepo *repository.UserRepository) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
		userRepo:      userRepo,
	}
}

func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyID, _ := c.Get("companyID")

	var req service.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	response, err := h.apiKeyService.CreateAPIKey(c.Request.Context(), userID.(string), companyID.(string), &req)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		// Log the actual error for debugging
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred.", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, response)
}

// ListAPIKeys returns all API keys for the company
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	companyID, _ := c.Get("companyID")

	keys, err := h.apiKeyService.ListAPIKeys(c.Request.Context(), companyID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch API keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": keys,
	})
}

func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	keyID := c.Param("id")

	if keyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key ID is required"})
		return
	}

	err := h.apiKeyService.RevokeAPIKey(c.Request.Context(), keyID, companyID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revoked successfully",
	})
}
