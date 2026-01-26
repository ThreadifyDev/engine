package service

import (
	"errors"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/utils"
	"time"
)

type ServiceAccountService struct {
	serviceAccountRepo *repository.ServiceAccountRepository
	userRoleRepo       *repository.UserRoleRepository
}

func NewServiceAccountService(
	serviceAccountRepo *repository.ServiceAccountRepository,
	userRoleRepo *repository.UserRoleRepository,
) *ServiceAccountService {
	return &ServiceAccountService{
		serviceAccountRepo: serviceAccountRepo,
		userRoleRepo:       userRoleRepo,
	}
}

type CreateServiceAccountRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Role        string  `json:"role" binding:"required"` // e.g., "owner", "participant", "observer"
}

func (s *ServiceAccountService) CreateServiceAccount(companyID, createdBy string, req *CreateServiceAccountRequest) (*models.ServiceAccount, error) {
	// Validate name
	if req.Name == "" {
		return nil, errors.New("service account name is required")
	}

	// Validate role - service accounts get app_level roles
	validRoles := map[string]bool{
		"standard_service": true,
		"reader":           true,
	}
	if !validRoles[req.Role] {
		return nil, errors.New("invalid role: must be 'standard_service' or 'reader'")
	}

	// Create service account
	sa := &models.ServiceAccount{
		ID:          utils.GenerateID(),
		CompanyID:   companyID,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    true,
		CreatedBy:   &createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.serviceAccountRepo.Create(sa); err != nil {
		return nil, err
	}

	// Assign role to service account
	if err := s.userRoleRepo.AssignRoleToServiceAccount(sa.ID, req.Role, createdBy); err != nil {
		// Log error but don't fail creation - role can be assigned later
		// TODO: Consider rolling back service account creation on role assignment failure
		return nil, errors.New("failed to assign role to service account")
	}

	return sa, nil
}

func (s *ServiceAccountService) ListServiceAccounts(companyID string) ([]*models.ServiceAccount, error) {
	return s.serviceAccountRepo.FindByCompanyID(companyID)
}

func (s *ServiceAccountService) GetServiceAccount(id, companyID string) (*models.ServiceAccount, error) {
	sa, err := s.serviceAccountRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if sa == nil {
		return nil, errors.New("service account not found")
	}
	if sa.CompanyID != companyID {
		return nil, errors.New("unauthorized")
	}
	return sa, nil
}

type UpdateServiceAccountRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

func (s *ServiceAccountService) UpdateServiceAccount(id, companyID string, req *UpdateServiceAccountRequest) (*models.ServiceAccount, error) {
	sa, err := s.GetServiceAccount(id, companyID)
	if err != nil {
		return nil, err
	}

	// Update fields
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

	if err := s.serviceAccountRepo.Update(sa); err != nil {
		return nil, err
	}

	return sa, nil
}

func (s *ServiceAccountService) DeleteServiceAccount(id, companyID string) error {
	sa, err := s.GetServiceAccount(id, companyID)
	if err != nil {
		return err
	}

	// Check if service account has any active API keys
	// TODO: Add check for active API keys

	return s.serviceAccountRepo.Delete(sa.ID)
}

// GetPermissions is deprecated - use role-based permissions instead
// This method is kept for backward compatibility but should not be used
func (s *ServiceAccountService) GetPermissions(roleStr string) ([]models.Permission, error) {
	return nil, errors.New("deprecated: use role-based permissions instead")
}
