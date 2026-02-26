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

type UpdateProfileRequest struct {
	FullName    string `json:"full_name"`
	JobRole     string `json:"job_role"`
	Industry    string `json:"industry"`
	CompanySize string `json:"company_size"`
	UseCase     string `json:"use_case"`
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, companyID, ok := getUserAndCompanyID(c)
	if !ok {
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.FullName == "" || req.JobRole == "" || req.Industry == "" || req.CompanySize == "" || req.UseCase == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "All fields are required"})
		return
	}

	if err := h.userRepo.UpdateProfile(userID, &req.FullName, &req.JobRole, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user profile"})
		return
	}

	if err := h.companyRepo.UpdateDetails(companyID, &req.Industry, &req.CompanySize, &req.UseCase); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update company details"})
		return
	}

	h.respondWithUser(c, userID, "Profile updated successfully")
}

func (h *UserHandler) MarkInstrumentationDone(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	if err := h.userRepo.MarkFirstInstrumentationDone(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update instrumentation status"})
		return
	}

	h.respondWithUser(c, userID, "Instrumentation status updated")
}

// helpers

func getUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return "", false
	}
	return userID.(string), true
}

func getUserAndCompanyID(c *gin.Context) (string, string, bool) {
	userID, ok := getUserID(c)
	if !ok {
		return "", "", false
	}
	companyID, exists := c.Get("companyID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return "", "", false
	}
	return userID, companyID.(string), true
}

func (h *UserHandler) respondWithUser(c *gin.Context, userID, message string) {
	user, err := h.userRepo.FindByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "user": user})
}
