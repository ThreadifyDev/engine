package handlers

import (
	"net/http"
	"time"

	iface "threadify-go/api/internal/interfaces"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/validation"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type TeamInvitationHandler struct {
	invitationSvc iface.TeamInvitationService
	companyRepo   repository.CompanyRepository
	logger        *zap.Logger
}

func NewTeamInvitationHandler(
	invitationSvc iface.TeamInvitationService,
	companyRepo repository.CompanyRepository,
	logger *zap.Logger,
) *TeamInvitationHandler {
	return &TeamInvitationHandler{
		invitationSvc: invitationSvc,
		companyRepo:   companyRepo,
		logger:        logger,
	}
}

type SendInvitationRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required,oneof=admin member viewer"`
}

type SendInvitationResponse struct {
	Success      bool   `json:"success"`
	InvitationID string `json:"invitationId,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	Message      string `json:"message,omitempty"`
	Error        string `json:"error,omitempty"`
}

// SendInvitation handles POST /api/team/invitations
func (h *TeamInvitationHandler) SendInvitation(c *gin.Context) {
	var req SendInvitationRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateSendInvitationRequest(req.Email, req.Role); err != nil {
		respondValidationError(c, err)
		return
	}

	// Get user from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		h.logger.Warn("unauthorized team invitation request")
		c.JSON(http.StatusUnauthorized, SendInvitationResponse{
			Success: false,
			Error:   "Unauthorized",
		})
		return
	}

	companyID, exists := c.Get("companyID")
	if !exists {
		h.logger.Warn("companyID not found in context")
		c.JSON(http.StatusUnauthorized, SendInvitationResponse{
			Success: false,
			Error:   "Unauthorized",
		})
		return
	}

	// Send invitation (7 day expiry)
	invitation, err := h.invitationSvc.SendInvitation(
		c.Request.Context(),
		companyID.(string),
		req.Email,
		req.Role,
		userID.(string),
		7*24*time.Hour,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, SendInvitationResponse{
				Success: false,
				Error:   de.Message,
			})
			return
		}
		h.logger.Error("failed to send invitation",
			zap.String("email", req.Email),
			zap.String("company_id", companyID.(string)),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, SendInvitationResponse{
			Success: false,
			Error:   "Failed to send invitation",
		})
		return
	}

	h.logger.Info("invitation sent",
		zap.String("email", req.Email),
		zap.String("role", req.Role),
		zap.String("company_id", companyID.(string)),
	)

	c.JSON(http.StatusCreated, SendInvitationResponse{
		Success:      true,
		InvitationID: invitation.ID,
		ExpiresAt:    invitation.ExpiresAt.Unix(),
		Message:      "Invitation sent successfully",
	})
}

type ValidateInvitationRequest struct {
	Token string `json:"token" binding:"required"`
}

type ValidateInvitationResponse struct {
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
}

type InvitationListItem struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	InvitedBy string `json:"invited_by"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
}

type ListInvitationsResponse struct {
	Invitations []InvitationListItem `json:"invitations"`
}

// ValidateInvitation handles POST /api/team/invitation/validate
func (h *TeamInvitationHandler) ValidateInvitation(c *gin.Context) {
	var req ValidateInvitationRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateValidateInvitationRequest(req.Token); err != nil {
		respondValidationError(c, err)
		return
	}

	// Validate token and get invitation
	invitation, err := h.invitationSvc.ValidateToken(c.Request.Context(), req.Token)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		h.logger.Warn("invalid invitation token",
			zap.String("token", req.Token),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get company name
	company, err := h.companyRepo.FindByID(c.Request.Context(), invitation.CompanyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		h.logger.Error("failed to get company",
			zap.String("company_id", invitation.CompanyID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve company information"})
		return
	}

	c.JSON(http.StatusOK, ValidateInvitationResponse{
		CompanyName: company.Name,
		Email:       invitation.Email,
	})
}

// ListInvitations handles GET /api/team/invitations
// Requires member.view permission
func (h *TeamInvitationHandler) ListInvitations(c *gin.Context) {
	companyID, exists := c.Get("companyID")
	if !exists {
		h.logger.Warn("companyID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get invitations for company
	invitations, err := h.invitationSvc.ListByCompany(c.Request.Context(), companyID.(string))
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		h.logger.Error("failed to list invitations",
			zap.String("company_id", companyID.(string)),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invitations"})
		return
	}

	// Convert to response format
	response := ListInvitationsResponse{
		Invitations: make([]InvitationListItem, 0, len(invitations)),
	}

	for _, inv := range invitations {
		response.Invitations = append(response.Invitations, InvitationListItem{
			ID:        inv.ID,
			Email:     inv.Email,
			Role:      inv.Role,
			Status:    inv.Status,
			InvitedBy: inv.InvitedBy,
			Token:     inv.Token,
			ExpiresAt: inv.ExpiresAt.Unix(),
			CreatedAt: inv.CreatedAt.Unix(),
		})
	}

	c.JSON(http.StatusOK, response)
}

// ResendInvitation handles POST /api/team/invitations/:id/resend
// Requires member.invite permission
func (h *TeamInvitationHandler) ResendInvitation(c *gin.Context) {
	invitationID := c.Param("id")
	if invitationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation ID is required"})
		return
	}

	companyID, exists := c.Get("companyID")
	if !exists {
		h.logger.Warn("companyID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get existing invitation
	invitation, err := h.invitationSvc.GetByID(c.Request.Context(), invitationID)
	if err != nil || invitation == nil {
		h.logger.Error("invitation not found",
			zap.String("invitation_id", invitationID),
			zap.Error(err),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}

	// Verify invitation belongs to user's company
	if invitation.CompanyID != companyID.(string) {
		h.logger.Warn("unauthorized access to invitation",
			zap.String("invitation_id", invitationID),
			zap.String("company_id", companyID.(string)),
		)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Only resend pending invitations
	if invitation.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only resend pending invitations"})
		return
	}

	// Refresh the existing invitation (update token and expiry)
	refreshedInvitation, err := h.invitationSvc.RefreshInvitation(
		c.Request.Context(),
		invitation,
		7*24*time.Hour,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, SendInvitationResponse{
				Success: false,
				Error:   de.Message,
			})
			return
		}
		h.logger.Error("failed to resend invitation",
			zap.String("email", invitation.Email),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resend invitation"})
		return
	}

	h.logger.Info("invitation resent",
		zap.String("email", invitation.Email),
		zap.String("invitation_id", refreshedInvitation.ID),
	)

	c.JSON(http.StatusOK, SendInvitationResponse{
		Success:      true,
		InvitationID: refreshedInvitation.ID,
		ExpiresAt:    refreshedInvitation.ExpiresAt.Unix(),
		Message:      "Invitation resent successfully",
	})
}

// CancelInvitation handles DELETE /api/team/invitations/:id
// Requires member.invite permission
func (h *TeamInvitationHandler) CancelInvitation(c *gin.Context) {
	invitationID := c.Param("id")
	if invitationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation ID is required"})
		return
	}

	companyID, exists := c.Get("companyID")
	if !exists {
		h.logger.Warn("companyID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get existing invitation
	invitation, err := h.invitationSvc.GetByID(c.Request.Context(), invitationID)
	if err != nil || invitation == nil {
		h.logger.Error("invitation not found",
			zap.String("invitation_id", invitationID),
			zap.Error(err),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}

	// Verify invitation belongs to user's company
	if invitation.CompanyID != companyID.(string) {
		h.logger.Warn("unauthorized access to invitation",
			zap.String("invitation_id", invitationID),
			zap.String("company_id", companyID.(string)),
		)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Only cancel pending invitations
	if invitation.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only cancel pending invitations"})
		return
	}

	// Cancel the invitation (update status to cancelled)
	err = h.invitationSvc.CancelInvitation(c.Request.Context(), invitationID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message, "success": false})
			return
		}
		h.logger.Error("failed to cancel invitation",
			zap.String("invitation_id", invitationID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel invitation"})
		return
	}

	h.logger.Info("invitation cancelled",
		zap.String("invitation_id", invitationID),
		zap.String("email", invitation.Email),
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invitation cancelled successfully",
	})
}
