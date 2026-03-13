package handlers

import (
	"net/http"
	"time"

	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type TeamInvitationHandler struct {
	invitationSvc *service.TeamInvitationService
	companyRepo   *repository.CompanyRepository
	logger        *zap.Logger
}

func NewTeamInvitationHandler(
	invitationSvc *service.TeamInvitationService,
	companyRepo *repository.CompanyRepository,
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

// ValidateInvitation handles POST /api/team/invitation/validate
func (h *TeamInvitationHandler) ValidateInvitation(c *gin.Context) {
	var req ValidateInvitationRequest
	if !bindJSON(c, &req) {
		return
	}

	// Validate token and get invitation
	invitation, err := h.invitationSvc.ValidateToken(req.Token)
	if err != nil {
		h.logger.Warn("invalid invitation token",
			zap.String("token", req.Token),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get company name
	company, err := h.companyRepo.FindByID(invitation.CompanyID)
	if err != nil {
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
