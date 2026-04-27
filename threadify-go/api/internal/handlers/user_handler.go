package handlers

import (
	"net/http"

	iface "threadify-go/api/internal/interfaces"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"errors"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService iface.UserService
}

func NewUserHandler(userService iface.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userCtx := getUserAndCompanyID(c)
	if userCtx == nil {
		return
	}

	result, err := h.userService.GetProfile(c.Request.Context(), userCtx.UserID, userCtx.CompanyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	minimal := c.Query("minimal") == "true"

	var companyData gin.H
	company := result.Company
	if minimal {
		companyData = gin.H{
			"details_completed": company.Industry != nil || company.Size != nil || company.UseCase != nil,
		}
	} else {
		companyData = gin.H{
			"details_completed": company.Industry != nil || company.Size != nil || company.UseCase != nil,
			"industry":          stringPtrToString(company.Industry),
			"company_size":      stringPtrToString(company.Size),
			"use_case":          stringPtrToString(company.UseCase),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user":    result.User,
		"company": companyData,
	})
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userCtx := getUserAndCompanyID(c)
	if userCtx == nil {
		return
	}

	var req models.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := validation.ValidateUpdateProfileRequest(&req); err != nil {
		var vErr *validation.RequestValidationError
		if errors.As(err, &vErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    vErr.FirstMessage(),
				"problems": vErr.Problems(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.UpdateProfile(c.Request.Context(), userCtx.UserID, userCtx.CompanyID, &req)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully", "user": user})
}

func (h *UserHandler) MarkInstrumentationDone(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	user, err := h.userService.MarkInstrumentationDone(c.Request.Context(), userID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Instrumentation status updated", "user": user})
}

func (h *UserHandler) ListTeamMembers(c *gin.Context) {
	userCtx := getUserAndCompanyID(c)
	if userCtx == nil {
		return
	}

	members, err := h.userService.ListTeamMembers(c.Request.Context(), userCtx.CompanyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve team members"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (h *UserHandler) RemoveTeamMember(c *gin.Context) {
	userCtx := getUserAndCompanyID(c)
	if userCtx == nil {
		return
	}

	targetUserID := c.Param("id")
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	if err := h.userService.RemoveTeamMember(c.Request.Context(), userCtx.UserID, userCtx.CompanyID, targetUserID); err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Member removed successfully"})
}

func stringPtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func getUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return "", false
	}
	return userID.(string), true
}

type userContext struct {
	UserID    string
	CompanyID string
}

func getUserAndCompanyID(c *gin.Context) *userContext {
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return nil
	}
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return nil
	}
	return &userContext{UserID: userID.(string), CompanyID: companyID.(string)}
}
