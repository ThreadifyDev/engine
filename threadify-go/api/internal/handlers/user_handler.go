package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	iface "threadify-go/api/internal/interfaces"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/utils"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type UserHandler struct {
	userRepo      repository.UserRepository
	companyRepo   repository.CompanyRepository
	apiKeyService iface.APIKeyService
	userRoleRepo  repository.UserRoleRepository
	authClient    sharedauth.AuthClient
	outboxRepo    repository.OutboxRepository
	outboxTrigger service.OutboxWorkerTrigger
	encryptionKey []byte
	logger        *zap.Logger
}

func NewUserHandler(
	userRepo repository.UserRepository,
	companyRepo repository.CompanyRepository,
	apiKeyService iface.APIKeyService,
	userRoleRepo repository.UserRoleRepository,
	authClient sharedauth.AuthClient,
	outboxRepo repository.OutboxRepository,
	outboxTrigger service.OutboxWorkerTrigger,
	encryptionKey []byte,
	logger *zap.Logger,
) *UserHandler {
	return &UserHandler{
		userRepo:      userRepo,
		companyRepo:   companyRepo,
		apiKeyService: apiKeyService,
		userRoleRepo:  userRoleRepo,
		authClient:    authClient,
		outboxRepo:    outboxRepo,
		outboxTrigger: outboxTrigger,
		encryptionKey: encryptionKey,
		logger:        logger,
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

	user, err := h.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	// If companyID from JWT is empty, use user's company_id
	if companyID == "" && user != nil {
		companyID = user.CompanyID
	}

	company, err := h.companyRepo.FindByID(c.Request.Context(), companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
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

	user, err := h.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	// If companyID from JWT is empty, use user's company_id
	if companyID == "" && user != nil {
		companyID = user.CompanyID
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
	company, err := h.companyRepo.FindByID(c.Request.Context(), companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
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
	if err := h.userRepo.UpdateProfile(c.Request.Context(), user.ID, &req.FullName, &req.JobRole, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user profile"})
		return
	}

	// Update company details if provided and company doesn't exist yet
	if companyProvided && !companyExists {
		if err := h.companyRepo.UpdateDetails(c.Request.Context(), companyID, &req.Industry, &req.CompanySize, &req.UseCase); err != nil {
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
	user, err := h.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	if err := h.userRepo.MarkFirstInstrumentationDone(c.Request.Context(), user.ID); err != nil {
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

	users, err := h.userRepo.ListByCompanyID(c.Request.Context(), companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve team members"})
		return
	}

	// Filter to only return necessary fields
	filteredUsers := make([]gin.H, len(users))
	for i, user := range users {
		roles, err := h.userRoleRepo.GetUserRoles(c.Request.Context(), user.ID)
		var highestRole string
		if err == nil && len(roles) > 0 {
			highestRole = roles[0]
		} else {
			highestRole = "member"
		}

		filteredUsers[i] = gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"full_name":  user.FullName,
			"job_role":   user.JobRole,
			"role":       highestRole,
			"created_at": user.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"members": filteredUsers})
}

func (h *UserHandler) RemoveTeamMember(c *gin.Context) {
	_, companyID, ok := getUserAndCompanyID(c)
	if !ok {
		return
	}

	targetUserID := c.Param("id")
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	// 1. Prevent users from removing themselves (enforce frontend rule at backend level too)
	currentUserID, _ := getUserID(c)
	if targetUserID == currentUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot remove yourself from the team"})
		return
	}

	// 2. Fetch target user and verify they belong to same company
	user, err := h.userRepo.FindByID(c.Request.Context(), targetUserID)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to look up user"})
		return
	}

	if user.CompanyID != companyID {
		c.JSON(http.StatusForbidden, gin.H{"error": "User does not belong to your company"})
		return
	}

	// 3. Prevent removing admins (optional, but good for safety unless we want to allow it)
	// Actually, the RBAC middleware would handle this if we set permissions correctly,
	// but let's check roles here too to be safe.
	roles, err := h.userRoleRepo.GetUserRoles(c.Request.Context(), targetUserID)
	if err == nil {
		for _, r := range roles {
			if r == "admin" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Cannot remove an administrator"})
				return
			}
		}
	}

	// 4. Perform archival (Deactivation)
	// We remove roles to revoke access, and update the email/company_id
	// to preserve records while allowing the original email to be reused.
	pool := h.userRepo.Pool()
	tx, err := pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(c.Request.Context()) //nolint:errcheck

	// Remove all roles
	_, err = tx.Exec(c.Request.Context(), `DELETE FROM user_roles WHERE principal_id = $1 AND principal_type = 'user'`, targetUserID)
	if err != nil {
		h.logger.Error("failed to remove user roles", zap.String("user_id", targetUserID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove user roles"})
		return
	}

	// Archive user record:
	// 1. Prefix email with archived metadata to free the original email
	// 2. Clear company_id, password_hash, and auth_user_id for security and mobility
	archivedEmail := fmt.Sprintf("archived-%s-%s", uuid.New().String(), user.Email)

	// We use tx.Exec directly to update multiple fields in one go
	_, err = tx.Exec(c.Request.Context(),
		`UPDATE users SET email = $1, company_id = NULL, password_hash = NULL, auth_user_id = NULL, updated_at = NOW() WHERE id = $2`,
		archivedEmail, targetUserID)

	if err != nil {
		h.logger.Error("failed to archive user record", zap.String("user_id", targetUserID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to archive user record"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit removal"})
		return
	}

	// 5. Queue Supabase user email update via outbox
	if user.AuthUserID != nil && *user.AuthUserID != "" {
		if err := h.queueSupabaseEmailUpdate(c.Request.Context(), *user.AuthUserID, archivedEmail, targetUserID); err != nil {
			h.logger.Warn("failed to queue Supabase email update", zap.Error(err))
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Member removed successfully"})
}

// queueSupabaseEmailUpdate creates an outbox event to update a Supabase user's email
func (h *UserHandler) queueSupabaseEmailUpdate(ctx context.Context, authUserID, newEmail, referenceID string) error {
	payload, err := json.Marshal(map[string]string{
		"auth_user_id": authUserID,
		"new_email":    newEmail,
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	encrypted, err := utils.Encrypt(payload, h.encryptionKey)
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return fmt.Errorf("encrypt payload: %w", err)
	}

	for i := range payload {
		payload[i] = 0
	}

	if err := h.outboxRepo.Create(ctx, &models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeUpdateAuthUserEmail,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  models.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: referenceID,
	}); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	if h.outboxTrigger != nil {
		h.outboxTrigger.Trigger()
	}

	return nil
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
	userID, exists := c.Get(sharedauth.CtxUserID)
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
	user, err := h.userRepo.FindByID(c.Request.Context(), userID)
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
