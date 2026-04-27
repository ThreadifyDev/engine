package handlers

import (
	"net/http"
	iface "threadify-go/api/internal/interfaces"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type APIKeyHandler struct {
	apiKeyService iface.APIKeyService
}

func NewAPIKeyHandler(apiKeyService iface.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
	}
}

func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	userID, exists := ctxString(c, sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req models.CreateAPIKeyRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateCreateAPIKeyRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	response, err := h.apiKeyService.CreateAPIKey(c.Request.Context(), userID, companyID, &req)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred.", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, response)
}

// ListAPIKeys returns all API keys for the company
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	keys, err := h.apiKeyService.ListAPIKeys(c.Request.Context(), companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch API keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": keys,
	})
}

func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	keyID := c.Param("id")

	if keyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key ID is required"})
		return
	}

	err := h.apiKeyService.RevokeAPIKey(c.Request.Context(), keyID, companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revoked successfully",
	})
}
