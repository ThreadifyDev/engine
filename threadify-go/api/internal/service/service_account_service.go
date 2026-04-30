package service

import (
	"context"
	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/utils"
	"time"
)

var validServiceAccountRoles = map[string]bool{
	"standard_service": true,
	"reader":           true,
}

type ServiceAccountService struct {
	serviceAccountRepo domain.ServiceAccountRepository
	userRoleRepo       domain.UserRoleRepository
}

func NewServiceAccountService(
	serviceAccountRepo domain.ServiceAccountRepository,
	userRoleRepo domain.UserRoleRepository,
) *ServiceAccountService {
	return &ServiceAccountService{
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
		return nil, err
	}
	if err := s.userRoleRepo.AssignRoleToServiceAccount(ctx, sa.ID, req.Role, createdBy); err != nil {
		return nil, ErrFailedToAssignRole
	}

	return sa, nil
}

func (s *ServiceAccountService) ListServiceAccounts(
	ctx context.Context,
	companyID string,
) ([]*domain.ServiceAccount, error) {
	return s.serviceAccountRepo.FindByCompanyID(ctx, companyID)
}

func (s *ServiceAccountService) GetServiceAccount(
	ctx context.Context,
	id string,
	companyID string,
) (*domain.ServiceAccount, error) {
	sa, err := s.serviceAccountRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sa == nil {
		return nil, ErrServiceAccountNotFound
	}
	if sa.CompanyID != companyID {
		return nil, ErrUnauthorized
	}
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
		return nil, err
	}

	if req.Name == "" {
		return nil, ErrServiceAccountNameRequired
	}

	sa.Name = req.Name
	sa.Description = &req.Description
	sa.IsActive = req.IsActive
	sa.UpdatedAt = time.Now()

	if err := s.serviceAccountRepo.Update(ctx, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

func (s *ServiceAccountService) DeleteServiceAccount(
	ctx context.Context,
	id string,
	companyID string,
) error {
	sa, err := s.GetServiceAccount(ctx, id, companyID)
	if err != nil {
		return err
	}
	// TODO: check for active API keys before deleting
	return s.serviceAccountRepo.Delete(ctx, sa.ID)
}
