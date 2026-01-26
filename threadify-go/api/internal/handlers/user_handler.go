package handlers

import (
	"net/http"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userRepo      *repository.UserRepository
	companyRepo   *repository.CompanyRepository
	apiKeyService *service.APIKeyService
}

func NewUserHandler(userRepo *repository.UserRepository, companyRepo *repository.CompanyRepository, apiKeyService *service.APIKeyService) *UserHandler {
	return &UserHandler{
		userRepo:      userRepo,
		companyRepo:   companyRepo,
		apiKeyService: apiKeyService,
	}
}

type UpdateProfileRequest struct {
	FullName    string `json:"full_name"`
	JobRole     string `json:"job_role"`
	Industry    string `json:"industry"`
	CompanySize string `json:"company_size"`
	UseCase     string `json:"use_case"`
}

// UpdateProfile handles user profile updates (for onboarding)
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	// Get user ID from JWT claims (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	companyID, exists := c.Get("companyID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate required fields
	if req.FullName == "" || req.JobRole == "" || req.Industry == "" || req.CompanySize == "" || req.UseCase == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "All fields are required"})
		return
	}

	// Update user profile
	var fullName, jobRole *string
	if req.FullName != "" {
		fullName = &req.FullName
	}
	if req.JobRole != "" {
		jobRole = &req.JobRole
	}

	err := h.userRepo.UpdateProfile(userID.(string), fullName, jobRole, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user profile"})
		return
	}

	// Update company details
	var industry, size, useCase *string
	if req.Industry != "" {
		industry = &req.Industry
	}
	if req.CompanySize != "" {
		size = &req.CompanySize
	}
	if req.UseCase != "" {
		useCase = &req.UseCase
	}

	err = h.companyRepo.UpdateDetails(companyID.(string), industry, size, useCase)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to update company details",
			"details": err.Error(),
		})
		return
	}

	// Get updated user
	user, err := h.userRepo.FindByID(userID.(string))
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user":    user,
	})
}

func (h *UserHandler) MarkInstrumentationDone(c *gin.Context) {
	// Get user ID from JWT claims
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Mark first instrumentation as done
	err := h.userRepo.MarkFirstInstrumentationDone(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update instrumentation status"})
		return
	}

	// Get updated user
	user, err := h.userRepo.FindByID(userID.(string))
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Instrumentation status updated",
		"user":    user,
	})
}
