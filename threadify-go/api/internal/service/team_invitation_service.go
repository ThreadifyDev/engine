package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/utils"

	"go.uber.org/zap"
)

type TeamInvitationService struct {
	invitationRepo domain.TeamInvitationRepository
	outboxRepo     domain.OutboxRepository
	outboxWorker   OutboxWorkerTrigger
	userRepo       domain.UserRepository
	companyRepo    domain.CompanyRepository
	encryptionKey  []byte
	frontendURL    string
	logger         *zap.Logger
}

func NewTeamInvitationService(
	invitationRepo domain.TeamInvitationRepository,
	outboxRepo domain.OutboxRepository,
	outboxWorker OutboxWorkerTrigger,
	userRepo domain.UserRepository,
	companyRepo domain.CompanyRepository,
	encryptionKey []byte,
	frontendURL string,
	logger *zap.Logger,
) *TeamInvitationService {
	return &TeamInvitationService{
		invitationRepo: invitationRepo,
		outboxRepo:     outboxRepo,
		outboxWorker:   outboxWorker,
		userRepo:       userRepo,
		companyRepo:    companyRepo,
		encryptionKey:  encryptionKey,
		frontendURL:    frontendURL,
		logger:         logger,
	}
}

// SendInvitation creates an invitation and queues an email via outbox
func (s *TeamInvitationService) SendInvitation(
	ctx context.Context,
	companyID, email, role, invitedBy string,
	expiryDuration time.Duration,
) (*domain.TeamInvitation, error) {
	// Check if user already exists
	existingUser, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil && existingUser != nil {
		return nil, fmt.Errorf("user with email %s already has an account", email)
	}

	// Generate invitation token
	token := utils.GenerateID()
	invitationID := utils.GenerateID()

	invitation := &domain.TeamInvitation{
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
	if err := s.invitationRepo.Create(ctx, invitation); err != nil {
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
func (s *TeamInvitationService) queueInvitationEmail(ctx context.Context, invitation *domain.TeamInvitation) error {
	// Build invitation link
	inviteLink := fmt.Sprintf("%s/login", s.frontendURL)

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
	event := &domain.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        domain.EventTypeSendTeamInvitation,
		Payload:     encrypted,
		Status:      domain.OutboxStatusPending,
		MaxRetries:  domain.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: invitation.ID,
	}

	if err := s.outboxRepo.Create(ctx, event); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	// Trigger outbox worker to process immediately
	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	return nil
}

func (s *TeamInvitationService) ValidateToken(ctx context.Context, token string) (*domain.InvitationTokenInfo, error) {
	invitation, err := s.invitationRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("get invitation: %w", err)
	}

	if invitation == nil {
		return nil, fmt.Errorf("invitation not found")
	}

	if invitation.Status != "pending" {
		return nil, fmt.Errorf("invitation already used")
	}

	if invitation.IsExpired() {
		return nil, fmt.Errorf("invitation expired")
	}

	company, err := s.companyRepo.FindByID(ctx, invitation.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("get company: %w", err)
	}

	var companyName string
	if company != nil {
		companyName = company.Name
	}

	return &domain.InvitationTokenInfo{
		CompanyName: companyName,
		Email:       invitation.Email,
	}, nil
}

// MarkAccepted marks an invitation as accepted
func (s *TeamInvitationService) MarkAccepted(ctx context.Context, invitationID, userID string) error {
	return s.invitationRepo.MarkAccepted(ctx, invitationID, userID)
}

// GetByCompanyAndEmail retrieves pending invitation for a company and email
func (s *TeamInvitationService) GetByCompanyAndEmail(ctx context.Context, companyID, email string) (*domain.TeamInvitation, error) {
	return s.invitationRepo.GetPendingByCompanyAndEmail(ctx, companyID, email)
}

// GetByID retrieves an invitation by ID
func (s *TeamInvitationService) GetByID(ctx context.Context, invitationID string) (*domain.TeamInvitation, error) {
	return s.invitationRepo.GetByID(ctx, invitationID)
}

// ListByCompany retrieves all invitations for a company
func (s *TeamInvitationService) ListByCompany(ctx context.Context, companyID string) ([]*domain.TeamInvitation, error) {
	return s.invitationRepo.ListByCompany(ctx, companyID)
}

// CancelInvitation permanently deletes a pending invitation
func (s *TeamInvitationService) CancelInvitation(ctx context.Context, invitationID string) error {
	return s.invitationRepo.Delete(ctx, invitationID)
}

// RefreshInvitation updates an existing invitation with a new token and expiry, and resends the email
func (s *TeamInvitationService) RefreshInvitation(ctx context.Context, invitation *domain.TeamInvitation, duration time.Duration) (*domain.TeamInvitation, error) {
	// Generate new token
	newToken := utils.GenerateID()
	expiresAt := time.Now().Add(duration)

	// Update invitation in database
	err := s.invitationRepo.RefreshInvitation(ctx, invitation.ID, newToken, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh invitation: %w", err)
	}

	// Update invitation object
	invitation.Token = newToken
	invitation.ExpiresAt = expiresAt

	// Queue email via outbox (reuse existing method)
	if err := s.queueInvitationEmail(ctx, invitation); err != nil {
		s.logger.Error("failed to queue invitation email",
			zap.String("invitation_id", invitation.ID),
			zap.String("email", invitation.Email),
			zap.Error(err),
		)
		// Don't fail the refresh if email queueing fails
	}

	return invitation, nil
}
