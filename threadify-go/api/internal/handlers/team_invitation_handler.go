package handlers

import (
	"net/http"
	"time"

	"threadify-go/api/internal/dto"
	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type TeamInvitationHandler struct {
	invitationSvc ports.TeamInvitationService
}

func NewTeamInvitationHandler(invitationSvc ports.TeamInvitationService) *TeamInvitationHandler {
	return &TeamInvitationHandler{
		invitationSvc: invitationSvc,
	}
}

func (h *TeamInvitationHandler) SendInvitation(c *gin.Context) {
	var req dto.SendInvitationRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateSendInvitationRequest(req.Email, req.Role); err != nil {
		respondValidationError(c, err)
		return
	}

	userID, exists := ctxString(c, sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, dto.SendInvitationResponse{
			Success: false,
			Error:   "Unauthorized",
		})
		return
	}

	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, dto.SendInvitationResponse{
			Success: false,
			Error:   "Unauthorized",
		})
		return
	}

	expiry := 7 * 24 * time.Hour

	invitation, err := h.invitationSvc.SendInvitation(
		c.Request.Context(),
		companyID,
		req.Email,
		req.Role,
		userID,
		expiry,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, dto.SendInvitationResponse{
				Success: false,
				Error:   de.Message,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, dto.SendInvitationResponse{
			Success: false,
			Error:   "Failed to send invitation",
		})
		return
	}

	c.JSON(http.StatusCreated, dto.SendInvitationResponse{
		Success:      true,
		InvitationID: invitation.ID,
		ExpiresAt:    invitation.ExpiresAt.Unix(),
		Message:      "Invitation sent successfully",
	})
}

// ValidateInvitation handles POST /api/team/invitation/validate
func (h *TeamInvitationHandler) ValidateInvitation(c *gin.Context) {
	var req dto.ValidateInvitationRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateValidateInvitationRequest(req.Token); err != nil {
		respondValidationError(c, err)
		return
	}

	result, err := h.invitationSvc.ValidateToken(c.Request.Context(), req.Token)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid token"})
		return
	}

	c.JSON(http.StatusOK, dto.ValidateInvitationResponse{
		CompanyName: result.CompanyName,
		Email:       result.Email,
	})
}

// ListInvitations handles GET /api/team/invitations
// Requires member.view permission
func (h *TeamInvitationHandler) ListInvitations(c *gin.Context) {
	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get invitations for company
	invitations, err := h.invitationSvc.ListByCompany(c.Request.Context(), companyID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invitations"})
		return
	}

	// Convert to response format
	response := dto.ListInvitationsResponse{
		Invitations: make([]*dto.InvitationListItem, 0, len(invitations)),
	}

	for _, inv := range invitations {
		response.Invitations = append(response.Invitations, &dto.InvitationListItem{
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

	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get existing invitation
	invitation, err := h.invitationSvc.GetByID(c.Request.Context(), invitationID)
	if err != nil || invitation == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}

	// Verify invitation belongs to user's company
	if invitation.CompanyID != companyID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Only resend pending invitations
	if invitation.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only resend pending invitations"})
		return
	}

	refreshExpiry := 7 * 24 * time.Hour

	refreshedInvitation, err := h.invitationSvc.RefreshInvitation(
		c.Request.Context(),
		invitation,
		refreshExpiry,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, dto.SendInvitationResponse{
				Success: false,
				Error:   de.Message,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resend invitation"})
		return
	}

	c.JSON(http.StatusOK, dto.SendInvitationResponse{
		Success:      true,
		InvitationID: refreshedInvitation.ID,
		ExpiresAt:    refreshedInvitation.ExpiresAt.Unix(),
		Message:      "Invitation resent successfully",
	})
}

func (h *TeamInvitationHandler) CancelInvitation(c *gin.Context) {
	invitationID := c.Param("id")
	if invitationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation ID is required"})
		return
	}

	companyID, exists := ctxString(c, sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	invitation, err := h.invitationSvc.GetByID(c.Request.Context(), invitationID)
	if err != nil || invitation == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}

	if invitation.CompanyID != companyID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	if invitation.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only cancel pending invitations"})
		return
	}

	err = h.invitationSvc.CancelInvitation(c.Request.Context(), invitationID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message, "success": false})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invitation cancelled successfully",
	})
}
