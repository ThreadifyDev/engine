package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"

	"go.uber.org/zap"
)

type TeamInvitationService struct {
	invitationRepo *repository.TeamInvitationRepository
	outboxRepo     *repository.OutboxRepository
	outboxWorker   OutboxWorkerTrigger
	encryptionKey  []byte
	frontendURL    string
	logger         *zap.Logger
}

func NewTeamInvitationService(
	invitationRepo *repository.TeamInvitationRepository,
	outboxRepo *repository.OutboxRepository,
	outboxWorker OutboxWorkerTrigger,
	encryptionKey string,
	frontendURL string,
	logger *zap.Logger,
) *TeamInvitationService {
	key := []byte(encryptionKey)
	return &TeamInvitationService{
		invitationRepo: invitationRepo,
		outboxRepo:     outboxRepo,
		outboxWorker:   outboxWorker,
		encryptionKey:  key,
		frontendURL:    frontendURL,
		logger:         logger,
	}
}

// SendInvitation creates an invitation and queues an email via outbox
func (s *TeamInvitationService) SendInvitation(
	ctx context.Context,
	companyID, email, role, invitedBy string,
	expiryDuration time.Duration,
) (*models.TeamInvitation, error) {
	// Generate invitation token
	token := utils.GenerateID()
	invitationID := utils.GenerateID()

	invitation := &models.TeamInvitation{
		ID:        invitationID,
		CompanyID: companyID,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		Status:    "pending",
		Token:     token,
		ExpiresAt: time.Now().Add(expiryDuration),
		CreatedAt: time.Now(),
	}

	// Save invitation to database
	if err := s.invitationRepo.Create(invitation); err != nil {
		return nil, fmt.Errorf("create invitation: %w", err)
	}

	// Queue email via outbox
	if err := s.queueInvitationEmail(ctx, invitation); err != nil {
		s.logger.Error("failed to queue invitation email",
			zap.String("invitation_id", invitationID),
			zap.String("email", email),
			zap.Error(err),
		)
		// Don't fail the invitation creation if email queueing fails
		// The outbox worker will retry
	}

	s.logger.Info("invitation created and email queued",
		zap.String("invitation_id", invitationID),
		zap.String("email", email),
		zap.String("role", role),
	)

	return invitation, nil
}

// queueInvitationEmail creates an outbox event to send the invitation email
func (s *TeamInvitationService) queueInvitationEmail(ctx context.Context, invitation *models.TeamInvitation) error {
	// Build invitation link
	inviteLink := fmt.Sprintf("%s/signup?invitation_token=%s", s.frontendURL, invitation.Token)

	// Create payload
	payload := map[string]string{
		"email":       invitation.Email,
		"role":        invitation.Role,
		"invite_link": inviteLink,
		"expires_at":  invitation.ExpiresAt.Format(time.RFC3339),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Encrypt payload
	encrypted, err := utils.Encrypt(payloadJSON, s.encryptionKey)
	if err != nil {
		for i := range payloadJSON {
			payloadJSON[i] = 0
		}
		return fmt.Errorf("encrypt payload: %w", err)
	}
	for i := range payloadJSON {
		payloadJSON[i] = 0
	}

	// Create outbox event
	event := &models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeSendTeamInvitation,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  models.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: invitation.ID,
	}

	if err := s.outboxRepo.Create(event); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	// Trigger outbox worker to process immediately
	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	return nil
}

// ValidateToken checks if a token is valid and not expired
func (s *TeamInvitationService) ValidateToken(token string) (*models.TeamInvitation, error) {
	invitation, err := s.invitationRepo.GetByToken(token)
	if err != nil {
		return nil, fmt.Errorf("get invitation: %w", err)
	}

	if invitation == nil {
		return nil, fmt.Errorf("invitation not found")
	}

	if invitation.Status != "pending" {
		return nil, fmt.Errorf("invitation already used")
	}

	if time.Now().After(invitation.ExpiresAt) {
		return nil, fmt.Errorf("invitation expired")
	}

	return invitation, nil
}

// MarkAccepted marks an invitation as accepted
func (s *TeamInvitationService) MarkAccepted(invitationID, userID string) error {
	return s.invitationRepo.MarkAccepted(invitationID, userID)
}

// GetByCompanyAndEmail retrieves pending invitation for a company and email
func (s *TeamInvitationService) GetByCompanyAndEmail(companyID, email string) (*models.TeamInvitation, error) {
	return s.invitationRepo.GetPendingByCompanyAndEmail(companyID, email)
}

// ListByCompany retrieves all invitations for a company
func (s *TeamInvitationService) ListByCompany(companyID string) ([]*models.TeamInvitation, error) {
	return s.invitationRepo.ListByCompany(companyID)
}
