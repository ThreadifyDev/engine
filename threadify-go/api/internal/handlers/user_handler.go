package handlers

import (
	"errors"
	"net/http"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userRepo      *repository.UserRepository
	companyRepo   *repository.CompanyRepository
	apiKeyService *service.APIKeyService
}

func NewUserHandler(
	userRepo *repository.UserRepository,
	companyRepo *repository.CompanyRepository,
	apiKeyService *service.APIKeyService,
) *UserHandler {
	return &UserHandler{
		userRepo:      userRepo,
		companyRepo:   companyRepo,
		apiKeyService: apiKeyService,
	}
}

// Helper function to safely convert string pointer to string
func stringPtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, companyID, ok := getUserAndCompanyID(c)
	if !ok {
		return
	}

	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	company, err := h.companyRepo.FindByID(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve company information"})
		return
	}

	// Check if minimal response requested (for onboarding)
	minimal := c.Query("minimal") == "true"

	var companyData gin.H
	if minimal {
		// Return only details_completed for onboarding
		companyData = gin.H{
			"details_completed": company.Industry != nil || company.Size != nil || company.UseCase != nil,
		}
	} else {
		// Return full company data for settings page
		companyData = gin.H{
			"details_completed": company.Industry != nil || company.Size != nil || company.UseCase != nil,
			"industry":          stringPtrToString(company.Industry),
			"company_size":      stringPtrToString(company.Size),
			"use_case":          stringPtrToString(company.UseCase),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user":    user,
		"company": companyData,
	})
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, companyID, ok := getUserAndCompanyID(c)
	if !ok {
		return
	}

	// Look up user by internal ID
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
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

	// Get company to check its current state
	company, err := h.companyRepo.FindByID(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve company information"})
		return
	}

	// Check if company details exist
	companyExists := company.Industry != nil || company.Size != nil || company.UseCase != nil
	companyProvided := req.Industry != "" || req.CompanySize != "" || req.UseCase != ""

	// Security check: If company doesn't exist and no company data provided, reject
	if !companyExists && !companyProvided {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company details are required for first-time setup"})
		return
	}

	// Security check: If company exists and user tries to update it, reject
	if companyExists && companyProvided {
		c.JSON(http.StatusForbidden, gin.H{"error": "Company details cannot be modified after initial setup"})
		return
	}

	// Update user profile (always allowed) - use internal database ID
	if err := h.userRepo.UpdateProfile(user.ID, &req.FullName, &req.JobRole, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user profile"})
		return
	}

	// Update company details if provided and company doesn't exist yet
	if companyProvided && !companyExists {
		if err := h.companyRepo.UpdateDetails(companyID, &req.Industry, &req.CompanySize, &req.UseCase); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update company details"})
			return
		}
	}

	h.respondWithUser(c, user.ID, "Profile updated successfully")
}

func (h *UserHandler) MarkInstrumentationDone(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// Look up user by internal ID
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	if err := h.userRepo.MarkFirstInstrumentationDone(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update instrumentation status"})
		return
	}

	h.respondWithUser(c, user.ID, "Instrumentation status updated")
}

func (h *UserHandler) ListTeamMembers(c *gin.Context) {
	_, companyID, ok := getUserAndCompanyID(c)
	if !ok {
		return
	}

	users, err := h.userRepo.ListByCompanyID(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve team members"})
		return
	}

	// Filter to only return necessary fields
	filteredUsers := make([]gin.H, len(users))
	for i, user := range users {
		filteredUsers[i] = gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"full_name":  user.FullName,
			"job_role":   user.JobRole,
			"created_at": user.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"members": filteredUsers})
}

// helpers

func getUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return "", false
	}
	return userID.(string), true
}

func getUserAndCompanyID(c *gin.Context) (string, string, bool) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return "", "", false
	}
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return "", "", false
	}
	return userID.(string), companyID.(string), true
}

func (h *UserHandler) respondWithUser(c *gin.Context, userID, message string) {
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "user": user})
}
