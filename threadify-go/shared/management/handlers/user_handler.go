package handlers

import (
	"net/http"

	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/dto"
	"threadify-go/shared/management/ports"
	"threadify-go/shared/management/validation"

	"errors"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService ports.UserService
}

func NewUserHandler(userService ports.UserService) *UserHandler {
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
	c.JSON(http.StatusOK, mapUserProfileToDTO(result, minimal))
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userCtx := getUserAndCompanyID(c)
	if userCtx == nil {
		return
	}

	var req dto.UpdateProfileRequest
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.Industry != "" || req.CompanySize != "" || req.UseCase != "" {
		raw, _ := c.Get(sharedauth.CtxRoles)
		roles, _ := raw.([]string)
		admin := false
		for _, role := range roles {
			if role == "admin" {
				admin = true
			}
		}
		if !admin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Company settings require an administrator"})
			return
		}
	}
	user, err := h.userService.UpdateProfile(c.Request.Context(), userCtx.UserID, userCtx.CompanyID, &domain.UpdateProfileCmd{
		FullName:    &req.FullName,
		JobRole:     &req.JobRole,
		Industry:    &req.Industry,
		CompanySize: &req.CompanySize,
		UseCase:     &req.UseCase,
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
		"message": "Profile updated successfully",
		"user":    mapUserToDTO(user),
	})
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

	c.JSON(http.StatusOK, gin.H{
		"message": "Instrumentation status updated",
		"user":    mapUserToDTO(user),
	})
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

	dtos := make([]*dto.TeamMember, len(members))
	for i, m := range members {
		dtos[i] = &dto.TeamMember{
			ID:        m.ID,
			Email:     m.Email,
			FullName:  stringPtrToString(m.FullName),
			JobRole:   stringPtrToString(m.JobRole),
			Role:      m.Role,
			CreatedAt: m.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"members": dtos})
}

func mapUserProfileToDTO(result *domain.UserProfile, minimal bool) gin.H {
	user := mapUserToDTO(result.User)
	company := result.Company

	detailsCompleted := company.Industry != nil || company.Size != nil || company.UseCase != nil

	if minimal {
		return gin.H{
			"user": user,
			"company": gin.H{
				"details_completed": detailsCompleted,
			},
		}
	}

	return gin.H{
		"user": user,
		"company": gin.H{
			"details_completed": detailsCompleted,
			"name":              company.Name,
			"industry":          stringPtrToString(company.Industry),
			"company_size":      stringPtrToString(company.Size),
			"use_case":          stringPtrToString(company.UseCase),
		},
	}
}

func mapUserToDTO(user *domain.User) *dto.User {
	if user == nil {
		return nil
	}
	return &dto.User{
		ID:                       user.ID,
		Email:                    user.Email,
		FullName:                 user.FullName,
		JobRole:                  user.JobRole,
		EmailVerified:            user.EmailVerified,
		OnboardingCompleted:      user.OnboardingCompleted,
		FirstInstrumentationDone: user.FirstInstrumentationDone,
		LastLoginAt:              user.LastLoginAt,
	}
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
