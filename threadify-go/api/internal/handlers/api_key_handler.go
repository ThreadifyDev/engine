package handlers

import (
	"net/http"
	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/dto"
	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type APIKeyHandler struct {
	apiKeyService ports.APIKeyService
}

func NewAPIKeyHandler(apiKeyService ports.APIKeyService) *APIKeyHandler {
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

	var req dto.CreateAPIKeyRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateCreateAPIKeyRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	response, err := h.apiKeyService.CreateAPIKey(c.Request.Context(), userID, companyID, &domain.CreateAPIKeyCmd{
		Name:                 req.Name,
		ExpiresIn:            req.ExpiresIn,
		ServiceAccountID:     req.ServiceAccountID,
		CreateServiceAccount: req.CreateServiceAccount,
		ServiceAccountRole:   req.ServiceAccountRole,
	})
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusCreated, &dto.CreateAPIKeyResponse{
		Key:       response.Key,
		KeyPrefix: response.KeyPrefix,
		APIKey:    mapAPIKeyToDTO(response.APIKey),
	})
}

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

	dtos := make([]*dto.APIKeyInfo, len(keys))
	for i, k := range keys {
		dtos[i] = mapAPIKeyToDTO(k)
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": dtos,
	})
}

func mapAPIKeyToDTO(k *domain.APIKey) *dto.APIKeyInfo {
	if k == nil {
		return nil
	}
	return &dto.APIKeyInfo{
		ID:               k.ID,
		CompanyID:        k.CompanyID,
		KeyPrefix:        k.KeyPrefix,
		Name:             k.Name,
		UserID:           k.UserID,
		ServiceAccountID: k.ServiceAccountID,
		LastUsedAt:       k.LastUsedAt,
		ExpiresAt:        k.ExpiresAt,
		RevokedAt:        k.RevokedAt,
	}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revoked successfully",
	})
}
