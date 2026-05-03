package service

import (
	"context"
	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/utils"
	"time"

	"go.uber.org/zap"
)

var validServiceAccountRoles = map[string]bool{
	"standard_service": true,
	"reader":           true,
}

type ServiceAccountService struct {
	logger             *zap.Logger
	serviceAccountRepo domain.ServiceAccountRepository
	userRoleRepo       domain.UserRoleRepository
}

func NewServiceAccountService(
	logger *zap.Logger,
	serviceAccountRepo domain.ServiceAccountRepository,
	userRoleRepo domain.UserRoleRepository,
) *ServiceAccountService {
	return &ServiceAccountService{
		logger:             logger,
		serviceAccountRepo: serviceAccountRepo,
		userRoleRepo:       userRoleRepo,
	}
}

func (s *ServiceAccountService) CreateServiceAccount(
	ctx context.Context,
	companyID string,
	createdBy string,
	req *domain.CreateServiceAccountCmd,
) (*domain.ServiceAccount, error) {
	if req.Name == "" {
		return nil, ErrServiceAccountNameRequired
	}
	if !validServiceAccountRoles[req.Role] {
		return nil, ErrInvalidRoleAPI
	}

	now := time.Now()
	sa := &domain.ServiceAccount{
		ID:          utils.GenerateID(),
		CompanyID:   companyID,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    true,
		CreatedBy:   &createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.serviceAccountRepo.Create(ctx, sa); err != nil {
		s.logger.Error("failed to create service account",
			zap.Error(err),
			zap.String("company_id", companyID),
			zap.String("created_by", createdBy),
			zap.String("name", req.Name),
		)
		return nil, err
	}
	if err := s.userRoleRepo.AssignRoleToServiceAccount(ctx, sa.ID, req.Role, createdBy); err != nil {
		s.logger.Error("failed to assign role to service account",
			zap.Error(err),
			zap.String("service_account_id", sa.ID),
			zap.String("role", req.Role),
			zap.String("created_by", createdBy),
		)
		return nil, ErrFailedToAssignRole
	}

	s.logger.Info("service account created successfully",
		zap.String("service_account_id", sa.ID),
		zap.String("company_id", companyID),
		zap.String("created_by", createdBy),
		zap.String("name", req.Name),
	)

	return sa, nil
}

func (s *ServiceAccountService) ListServiceAccounts(
	ctx context.Context,
	companyID string,
) ([]*domain.ServiceAccount, error) {
	serviceAccounts, err := s.serviceAccountRepo.FindByCompanyID(ctx, companyID)
	if err != nil {
		s.logger.Error("failed to list service accounts",
			zap.Error(err),
			zap.String("company_id", companyID),
		)
		return nil, err
	}

	s.logger.Info("service accounts listed successfully",
		zap.Int("count", len(serviceAccounts)),
		zap.String("company_id", companyID),
	)

	return serviceAccounts, nil
}

func (s *ServiceAccountService) GetServiceAccount(
	ctx context.Context,
	id string,
	companyID string,
) (*domain.ServiceAccount, error) {
	sa, err := s.serviceAccountRepo.FindByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get service account",
			zap.Error(err),
			zap.String("id", id),
			zap.String("company_id", companyID),
		)
		return nil, err
	}
	if sa == nil {
		s.logger.Error("service account not found",
			zap.String("id", id),
			zap.String("company_id", companyID),
		)
		return nil, ErrServiceAccountNotFound
	}
	if sa.CompanyID != companyID {
		s.logger.Warn("service account not authorized",
			zap.String("id", id),
			zap.String("company_id", companyID),
			zap.String("sa_company_id", sa.CompanyID),
		)
		return nil, ErrUnauthorized
	}

	s.logger.Info("service account retrieved successfully",
		zap.String("id", id),
		zap.String("company_id", companyID),
	)

	return sa, nil
}

func (s *ServiceAccountService) UpdateServiceAccount(
	ctx context.Context,
	id string,
	companyID string,
	req *domain.UpdateServiceAccountCmd,
) (*domain.ServiceAccount, error) {
	sa, err := s.GetServiceAccount(ctx, id, companyID)
	if err != nil {
		s.logger.Error("failed to get service account",
			zap.Error(err),
			zap.String("id", id),
			zap.String("company_id", companyID),
		)
		return nil, err
	}

	if req.Name == "" {
		s.logger.Warn("service account name is required",
			zap.String("id", id),
			zap.String("company_id", companyID),
			zap.String("name", req.Name),
		)
		return nil, ErrServiceAccountNameRequired
	}

	sa.Name = req.Name
	sa.Description = &req.Description
	sa.IsActive = req.IsActive
	sa.UpdatedAt = time.Now()

	if err := s.serviceAccountRepo.Update(ctx, sa); err != nil {
		s.logger.Error("failed to update service account",
			zap.Error(err),
			zap.String("id", id),
			zap.String("company_id", companyID),
		)
		return nil, err
	}

	s.logger.Info("service account updated successfully",
		zap.String("id", id),
		zap.String("company_id", companyID),
	)

	return sa, nil
}

func (s *ServiceAccountService) DeleteServiceAccount(
	ctx context.Context,
	id string,
	companyID string,
) error {
	sa, err := s.GetServiceAccount(ctx, id, companyID)
	if err != nil {
		s.logger.Error("failed to get service account",
			zap.Error(err),
			zap.String("id", id),
			zap.String("company_id", companyID),
		)
		return err
	}
	// TODO: check for active API keys before deleting
	if err := s.serviceAccountRepo.Delete(ctx, sa.ID); err != nil {
		s.logger.Error("failed to delete service account",
			zap.Error(err),
			zap.String("id", id),
			zap.String("company_id", companyID),
		)
		return err
	}

	s.logger.Info("service account deleted successfully",
		zap.String("id", id),
		zap.String("company_id", companyID),
	)

	return nil
}
