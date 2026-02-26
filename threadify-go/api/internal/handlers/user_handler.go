package handlers

import (
	"errors"
	"net/http"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/validation"
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

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, companyID, ok := getUserAndCompanyID(c)
	if !ok {
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
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
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
