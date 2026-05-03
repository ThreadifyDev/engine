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

type UserService struct {
	userRepo      domain.UserRepository
	companyRepo   domain.CompanyRepository
	userRoleRepo  domain.UserRoleRepository
	outboxRepo    domain.OutboxRepository
	outboxTrigger OutboxWorkerTrigger
	encryptionKey []byte
	logger        *zap.Logger
}

func NewUserService(
	userRepo domain.UserRepository,
	companyRepo domain.CompanyRepository,
	userRoleRepo domain.UserRoleRepository,
	outboxRepo domain.OutboxRepository,
	outboxTrigger OutboxWorkerTrigger,
	encryptionKey []byte,
	logger *zap.Logger,
) *UserService {
	return &UserService{
		userRepo:      userRepo,
		companyRepo:   companyRepo,
		userRoleRepo:  userRoleRepo,
		outboxRepo:    outboxRepo,
		outboxTrigger: outboxTrigger,
		encryptionKey: encryptionKey,
		logger:        logger,
	}
}

func (s *UserService) GetProfile(ctx context.Context, userID, companyID string) (*domain.UserProfile, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.logger.Error("get profile: failed to find user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("find user: %w", err)
	}

	company, err := s.companyRepo.FindByID(ctx, companyID)
	if err != nil {
		s.logger.Error("get profile: failed to find company",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("find company: %w", err)
	}

	return &domain.UserProfile{User: user, Company: company}, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID, companyID string, req *domain.UpdateProfileCmd) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.logger.Error("update profile: failed to find user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("find user: %w", err)
	}

	company, err := s.companyRepo.FindByID(ctx, companyID)
	if err != nil {
		s.logger.Error("update profile: failed to find company",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("find company: %w", err)
	}

	if err := company.CanUpdateDetails(req.Industry, req.CompanySize, req.UseCase); err != nil {
		s.logger.Warn("update profile: company details update not allowed",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, serror.NewDomainError(err.Error(), 400)
	}

	if err := s.userRepo.UpdateProfile(ctx, user.ID, req.FullName, req.JobRole, true); err != nil {
		s.logger.Error("update profile: failed to update user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("update user profile: %w", err)
	}

	companyProvided := (req.Industry != nil && *req.Industry != "") ||
		(req.CompanySize != nil && *req.CompanySize != "") ||
		(req.UseCase != nil && *req.UseCase != "")

	if companyProvided && !company.HasDetails() {
		if err := s.companyRepo.UpdateDetails(ctx, companyID, req.Industry, req.CompanySize, req.UseCase); err != nil {
			s.logger.Error("update profile: failed to update company details",
				zap.String("company_id", companyID),
				zap.Error(err),
			)
			return nil, fmt.Errorf("update company details: %w", err)
		}
	}

	updatedUser, err := s.userRepo.FindByID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("fetch updated user profile: %w", err)
	}

	s.logger.Info("update profile: profile updated",
		zap.String("user_id", userID),
		zap.String("company_id", companyID),
	)

	return updatedUser, nil
}

func (s *UserService) MarkInstrumentationDone(ctx context.Context, userID string) (*domain.User, error) {
	if err := s.userRepo.MarkFirstInstrumentationDone(ctx, userID); err != nil {
		s.logger.Error("mark instrumentation done: failed",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("mark instrumentation done: %w", err)
	}

	updatedUser, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.logger.Error("mark instrumentation done: failed to fetch updated user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("fetch updated user: %w", err)
	}

	s.logger.Info("mark instrumentation done: user updated",
		zap.String("user_id", userID),
	)

	return updatedUser, nil
}

func (s *UserService) ListTeamMembers(ctx context.Context, companyID string) ([]*domain.TeamMember, error) {
	users, err := s.userRepo.ListByCompanyID(ctx, companyID)
	if err != nil {
		s.logger.Error("list team members: failed to fetch users",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("list company users: %w", err)
	}

	members := make([]*domain.TeamMember, len(users))
	for i, u := range users {
		role := "member"
		if roles, err := s.userRoleRepo.GetUserRoles(ctx, u.ID); err == nil && len(roles) > 0 {
			role = roles[0]
		}
		members[i] = &domain.TeamMember{
			ID:        u.ID,
			Email:     u.Email,
			FullName:  u.FullName,
			JobRole:   u.JobRole,
			Role:      role,
			CreatedAt: u.CreatedAt,
		}
	}

	s.logger.Info("list team members: users fetched successfully",
		zap.String("company_id", companyID),
		zap.Int("count", len(members)),
	)

	return members, nil
}

func (s *UserService) RemoveTeamMember(ctx context.Context, requesterID, companyID, targetUserID string) error {
	if targetUserID == requesterID {
		return serror.NewDomainError("You cannot remove yourself from the team", 400)
	}

	target, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, serror.ErrUserNotFound) {
			return serror.NewDomainError("User not found", 404)
		}
		return fmt.Errorf("look up target user: %w", err)
	}

	if target.CompanyID != companyID {
		s.logger.Warn("remove team member: company mismatch",
			zap.String("requester_id", requesterID),
			zap.String("target_user_id", targetUserID),
			zap.String("expected_company", companyID),
			zap.String("actual_company", target.CompanyID),
		)
		return serror.NewDomainError("User does not belong to your company", 403)
	}

	if roles, err := s.userRoleRepo.GetUserRoles(ctx, targetUserID); err == nil {
		for _, r := range roles {
			if r == "admin" {
				s.logger.Warn("remove team member: attempt to remove admin",
					zap.String("requester_id", requesterID),
					zap.String("target_user_id", targetUserID),
				)
				return serror.NewDomainError("Cannot remove an administrator", 403)
			}
		}
	}

	archivedEmail := target.GenerateArchivedEmail()

	if err := s.userRepo.ArchiveUser(ctx, targetUserID, archivedEmail); err != nil {
		s.logger.Error("remove team member: failed to archive user",
			zap.String("target_user_id", targetUserID),
			zap.Error(err),
		)
		return fmt.Errorf("archive user: %w", err)
	}

	s.logger.Info("remove team member: user archived",
		zap.String("requester_id", requesterID),
		zap.String("target_user_id", targetUserID),
		zap.String("company_id", companyID),
	)

	if target.AuthUserID != nil && *target.AuthUserID != "" {
		if err := s.queueAuthEmailUpdate(ctx, *target.AuthUserID, archivedEmail, targetUserID); err != nil {
			s.logger.Warn("remove team member: failed to queue auth email update",
				zap.String("target_user_id", targetUserID),
				zap.Error(err),
			)
		}
	}

	return nil
}

func (s *UserService) queueAuthEmailUpdate(ctx context.Context, authUserID, newEmail, referenceID string) error {
	payload, err := json.Marshal(map[string]string{
		"auth_user_id": authUserID,
		"new_email":    newEmail,
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	encrypted, err := utils.Encrypt(payload, s.encryptionKey)
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return fmt.Errorf("encrypt payload: %w", err)
	}
	for i := range payload {
		payload[i] = 0
	}

	if err := s.outboxRepo.Create(ctx, &domain.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        domain.EventTypeUpdateAuthUserEmail,
		Payload:     encrypted,
		Status:      domain.OutboxStatusPending,
		MaxRetries:  domain.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: referenceID,
	}); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	if s.outboxTrigger != nil {
		s.outboxTrigger.Trigger()
	}

	return nil
}
