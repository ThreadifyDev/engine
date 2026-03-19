package handlers

import (
	"errors"
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

// CreateAPIKey generates a new API key
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	authUserID, _ := c.Get("authUserID")
	companyID, _ := c.Get("companyID")

	// Look up user by auth_user_id to get internal ID
	user, err := h.userRepo.FindByAuthUserID(authUserID.(string))
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	var req service.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	response, err := h.apiKeyService.CreateAPIKey(user.ID, companyID.(string), &req)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyNameRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
