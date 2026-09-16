package handlers

import (
	"net/http"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/dto"
	"threadify-go/shared/management/ports"
	"threadify-go/shared/rbac"

	"github.com/gin-gonic/gin"
)

type ServiceAccountHandler struct {
	serviceAccountService ports.ServiceAccountService
	rbacLoader            *rbac.Loader
}

func NewServiceAccountHandler(serviceAccountService ports.ServiceAccountService, rbacLoader *rbac.Loader) *ServiceAccountHandler {
	return &ServiceAccountHandler{
		serviceAccountService: serviceAccountService,
		rbacLoader:            rbacLoader,
	}
}

// CreateServiceAccount - POST /api/service-accounts
func (h *ServiceAccountHandler) CreateServiceAccount(c *gin.Context) {
	companyID := c.GetString("companyID")
	userID := c.GetString("userID")

	var req dto.CreateServiceAccountRequest
	if !bindJSON(c, &req) {
		return
	}

	sa, err := h.serviceAccountService.CreateServiceAccount(c.Request.Context(), companyID, userID, &domain.CreateServiceAccountCmd{
		Name:        req.Name,
		Description: req.Description,
		Role:        req.Role,
	})
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"service_account": mapServiceAccountToDTO(sa),
		"message":         "Service account created successfully",
	})
}

// ListServiceAccounts - GET /api/service-accounts
func (h *ServiceAccountHandler) ListServiceAccounts(c *gin.Context) {
	companyID := c.GetString("companyID")

	sas, err := h.serviceAccountService.ListServiceAccounts(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	dtos := make([]*dto.ServiceAccount, len(sas))
	for i, sa := range sas {
		dtos[i] = mapServiceAccountToDTO(sa)
	}

	c.JSON(http.StatusOK, gin.H{
		"service_accounts": dtos,
	})
}

// GetServiceAccount - GET /api/service-accounts/:id
func (h *ServiceAccountHandler) GetServiceAccount(c *gin.Context) {
	companyID := c.GetString("companyID")
	id := c.Param("id")

	sa, err := h.serviceAccountService.GetServiceAccount(c.Request.Context(), id, companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"service_account": mapServiceAccountToDTO(sa),
	})
}

// UpdateServiceAccount - PUT /api/service-accounts/:id
func (h *ServiceAccountHandler) UpdateServiceAccount(c *gin.Context) {
	companyID := c.GetString("companyID")
	id := c.Param("id")

	var req dto.UpdateServiceAccountRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	sa, err := h.serviceAccountService.UpdateServiceAccount(c.Request.Context(), id, companyID, &domain.UpdateServiceAccountCmd{
		Name:        req.Name,
		Description: req.Description,
		IsActive:    req.IsActive,
	})
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"service_account": mapServiceAccountToDTO(sa),
		"message":         "Service account updated successfully",
	})
}

// DeleteServiceAccount - DELETE /api/service-accounts/:id
func (h *ServiceAccountHandler) DeleteServiceAccount(c *gin.Context) {
	companyID := c.GetString("companyID")
	id := c.Param("id")

	if err := h.serviceAccountService.DeleteServiceAccount(c.Request.Context(), id, companyID); err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Service account deleted successfully",
	})
}

// GetPermissions - GET /api/service-accounts/scopes/:scope/permissions
// Returns available roles for service accounts from api_level
func (h *ServiceAccountHandler) GetPermissions(c *gin.Context) {
	scope := c.Param("scope")

	// Service accounts should use api_level roles
	if scope != "api_level" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Service accounts must use 'api_level' scope",
		})
		return
	}

	// Get roles from api_level
	roles := h.rbacLoader.GetRolesByLevel("api_level")

	c.JSON(http.StatusOK, gin.H{
		"scope": scope,
		"roles": roles,
	})
}

func mapServiceAccountToDTO(sa *domain.ServiceAccount) *dto.ServiceAccount {
	if sa == nil {
		return nil
	}
	return &dto.ServiceAccount{
		ID:          sa.ID,
		CompanyID:   sa.CompanyID,
		Name:        sa.Name,
		Description: sa.Description,
		IsActive:    sa.IsActive,
		CreatedBy:   sa.CreatedBy,
		LastUsedAt:  sa.LastUsedAt,
		CreatedAt:   sa.CreatedAt,
		UpdatedAt:   sa.UpdatedAt,
	}
}
