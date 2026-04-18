package service

import (
	"context"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"
	"time"
)

var validServiceAccountRoles = map[string]bool{
	"standard_service": true,
	"reader":           true,
}

type ServiceAccountService struct {
	serviceAccountRepo repository.ServiceAccountRepository
	userRoleRepo       repository.UserRoleRepository
}

func NewServiceAccountService(
	serviceAccountRepo repository.ServiceAccountRepository,
	userRoleRepo repository.UserRoleRepository,
) *ServiceAccountService {
	return &ServiceAccountService{
		serviceAccountRepo: serviceAccountRepo,
		userRoleRepo:       userRoleRepo,
	}
}

type CreateServiceAccountRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Role        string  `json:"role" binding:"required"`
}

type UpdateServiceAccountRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

func (s *ServiceAccountService) CreateServiceAccount(ctx context.Context, companyID, createdBy string, req *CreateServiceAccountRequest) (*models.ServiceAccount, error) {
	if req.Name == "" {
		return nil, ErrServiceAccountNameRequired
	}
	if !validServiceAccountRoles[req.Role] {
		return nil, ErrInvalidRoleAPI
	}

	now := time.Now()
	sa := &models.ServiceAccount{
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

func (s *ServiceAccountService) ListServiceAccounts(ctx context.Context, companyID string) ([]*models.ServiceAccount, error) {
	return s.serviceAccountRepo.FindByCompanyID(ctx, companyID)
}

func (s *ServiceAccountService) GetServiceAccount(ctx context.Context, id, companyID string) (*models.ServiceAccount, error) {
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

func (s *ServiceAccountService) UpdateServiceAccount(ctx context.Context, id, companyID string, req *UpdateServiceAccountRequest) (*models.ServiceAccount, error) {
	sa, err := s.GetServiceAccount(ctx, id, companyID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		sa.Name = *req.Name
	}
	if req.Description != nil {
		sa.Description = req.Description
	}
	if req.IsActive != nil {
		sa.IsActive = *req.IsActive
	}
	sa.UpdatedAt = time.Now()

	if err := s.serviceAccountRepo.Update(ctx, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

func (s *ServiceAccountService) DeleteServiceAccount(ctx context.Context, id, companyID string) error {
	sa, err := s.GetServiceAccount(ctx, id, companyID)
	if err != nil {
		return err
	}
	// TODO: check for active API keys before deleting
	return s.serviceAccountRepo.Delete(ctx, sa.ID)
}
