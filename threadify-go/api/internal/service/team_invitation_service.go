package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/utils"
	serror "threadify-go/shared/errors"

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

func (s *TeamInvitationService) SendInvitation(
	ctx context.Context,
	companyID, email, role, invitedBy string,
	expiryDuration time.Duration,
) (*domain.TeamInvitation, error) {
	existingUser, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, serror.ErrUserNotFound) {
		s.logger.Error("send invitation: failed to check existing user",
			zap.String("email", email),
			zap.Error(err),
		)
		return nil, fmt.Errorf("check existing user: %w", err)
	}
	if existingUser != nil {
		s.logger.Warn("send invitation: user already has an account",
			zap.String("email", email),
			zap.String("company_id", companyID),
		)
		return nil, ErrUserAlreadyExists
	}

	invitation := &domain.TeamInvitation{
		ID:        utils.GenerateID(),
		CompanyID: companyID,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		Status:    "pending",
		Token:     utils.GenerateID(),
		ExpiresAt: time.Now().Add(expiryDuration),
		CreatedAt: time.Now(),
	}

	if err := s.invitationRepo.Create(ctx, invitation); err != nil {
		s.logger.Error("send invitation: failed to persist invitation",
			zap.String("email", email),
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("create invitation: %w", err)
	}

	if err := s.queueInvitationEmail(ctx, invitation); err != nil {
		s.logger.Error("send invitation: failed to queue email",
			zap.String("invitation_id", invitation.ID),
			zap.String("email", email),
			zap.Error(err),
		)
	}

	s.logger.Info("send invitation: invitation created",
		zap.String("invitation_id", invitation.ID),
		zap.String("email", email),
		zap.String("role", role),
		zap.String("company_id", companyID),
	)

	return invitation, nil
}

func (s *TeamInvitationService) queueInvitationEmail(ctx context.Context, invitation *domain.TeamInvitation) error {
	// Embedded login accepts invitations through the verified mailbox.
	inviteLink := fmt.Sprintf("%s/login", s.frontendURL)

	payloadJSON, err := json.Marshal(map[string]string{
		"email":       invitation.Email,
		"role":        invitation.Role,
		"invite_link": inviteLink,
		"expires_at":  invitation.ExpiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

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

	if err := s.outboxRepo.Create(ctx, &domain.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        domain.EventTypeSendTeamInvitation,
		Payload:     encrypted,
		Status:      domain.OutboxStatusPending,
		MaxRetries:  domain.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: invitation.ID,
	}); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	return nil
}

func (s *TeamInvitationService) ValidateToken(ctx context.Context, token string) (*domain.InvitationTokenInfo, error) {
	invitation, err := s.invitationRepo.GetByToken(ctx, token)
	if err != nil {
		s.logger.Warn("validate token: failed to retrieve invitation", zap.Error(err))
		return nil, fmt.Errorf("get invitation: %w", err)
	}
	if invitation == nil {
		s.logger.Warn("validate token: invitation not found")
		return nil, fmt.Errorf("invitation not found")
	}
	if invitation.Status != "pending" {
		s.logger.Warn("validate token: invitation already used",
			zap.String("invitation_id", invitation.ID),
			zap.String("status", invitation.Status),
		)
		return nil, fmt.Errorf("invitation already used")
	}
	if invitation.IsExpired() {
		s.logger.Warn("validate token: invitation expired",
			zap.String("invitation_id", invitation.ID),
			zap.Time("expired_at", invitation.ExpiresAt),
		)
		return nil, fmt.Errorf("invitation expired")
	}

	company, err := s.companyRepo.FindByID(ctx, invitation.CompanyID)
	if err != nil {
		s.logger.Error("validate token: failed to retrieve company",
			zap.String("company_id", invitation.CompanyID),
			zap.Error(err),
		)
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

func (s *TeamInvitationService) MarkAccepted(ctx context.Context, invitationID, userID string) error {
	err := s.invitationRepo.MarkAccepted(ctx, invitationID, userID)
	if err != nil {
		s.logger.Error("mark accepted: failed to update invitation",
			zap.String("invitation_id", invitationID),
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return fmt.Errorf("mark accepted: %w", err)
	}

	return nil
}

func (s *TeamInvitationService) GetByCompanyAndEmail(ctx context.Context, companyID, email string) (*domain.TeamInvitation, error) {
	invitation, err := s.invitationRepo.GetPendingByCompanyAndEmail(ctx, companyID, email)
	if err != nil {
		s.logger.Error("get by company and email: failed to retrieve invitation",
			zap.String("company_id", companyID),
			zap.String("email", email),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get by company and email: %w", err)
	}
	s.logger.Info("get by company and email: invitation found",
		zap.String("invitation_id", invitation.ID),
		zap.String("email", email),
		zap.String("company_id", companyID),
	)

	return invitation, nil
}

func (s *TeamInvitationService) GetByID(ctx context.Context, invitationID string) (*domain.TeamInvitation, error) {
	invitation, err := s.invitationRepo.GetByID(ctx, invitationID)
	if err != nil {
		s.logger.Error("get by id: failed to retrieve invitation",
			zap.String("invitation_id", invitationID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get by id: %w", err)
	}

	s.logger.Info("get by id: invitation found",
		zap.String("invitation_id", invitation.ID),
		zap.String("email", invitation.Email),
		zap.String("company_id", invitation.CompanyID),
	)

	return invitation, nil
}

func (s *TeamInvitationService) ListByCompany(ctx context.Context, companyID string) ([]*domain.TeamInvitation, error) {
	invitations, err := s.invitationRepo.ListByCompany(ctx, companyID)
	if err != nil {
		s.logger.Error("list by company: failed to retrieve invitations",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("list by company: %w", err)
	}

	s.logger.Info("list by company: invitations found",
		zap.String("company_id", companyID),
		zap.Int("count", len(invitations)),
	)

	return invitations, nil
}

func (s *TeamInvitationService) CancelInvitation(ctx context.Context, invitationID string) error {
	err := s.invitationRepo.Delete(ctx, invitationID)
	if err != nil {
		s.logger.Error("cancel invitation: failed to delete invitation",
			zap.String("invitation_id", invitationID),
			zap.Error(err),
		)
		return fmt.Errorf("cancel invitation: %w", err)
	}

	s.logger.Info("cancel invitation: invitation deleted",
		zap.String("invitation_id", invitationID),
	)

	return nil
}

func (s *TeamInvitationService) RefreshInvitation(ctx context.Context, invitation *domain.TeamInvitation, duration time.Duration) (*domain.TeamInvitation, error) {
	newToken := utils.GenerateID()
	expiresAt := time.Now().Add(duration)

	if err := s.invitationRepo.RefreshInvitation(ctx, invitation.ID, newToken, expiresAt); err != nil {
		s.logger.Error("refresh invitation: failed to update invitation",
			zap.String("invitation_id", invitation.ID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("refresh invitation: %w", err)
	}

	invitation.Token = newToken
	invitation.ExpiresAt = expiresAt

	if err := s.queueInvitationEmail(ctx, invitation); err != nil {
		s.logger.Error("refresh invitation: failed to queue email",
			zap.String("invitation_id", invitation.ID),
			zap.String("email", invitation.Email),
			zap.Error(err),
		)
	}

	s.logger.Info("refresh invitation: invitation refreshed",
		zap.String("invitation_id", invitation.ID),
		zap.String("email", invitation.Email),
		zap.Time("expires_at", expiresAt),
	)

	return invitation, nil
}
