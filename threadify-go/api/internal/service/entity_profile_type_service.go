package service

import (
	"context"
	"time"

	"threadify-go/api/internal/models"
	sharedmodels "threadify-go/shared/models"
	"threadify-go/shared/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EntityProfileTypeService struct {
	repo   repository.EntityProfileTypeRepository
	logger *zap.Logger
}

func NewEntityProfileTypeService(repo repository.EntityProfileTypeRepository, logger *zap.Logger) *EntityProfileTypeService {
	return &EntityProfileTypeService{
		repo:   repo,
		logger: logger,
	}
}

func (s *EntityProfileTypeService) CreateEntityProfileType(ctx context.Context, companyID string, req *models.CreateEntityProfileTypeRequest) (*sharedmodels.EntityProfileType, error) {
	profileType := &sharedmodels.EntityProfileType{
		ID:          uuid.New().String(),
		CompanyID:   companyID,
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.repo.CreateProfileType(ctx, profileType); err != nil {
		s.logger.Error("Failed to create entity profile type", zap.Error(err))
		return nil, err
	}

	s.logger.Info("Entity profile type created successfully", zap.String("id", profileType.ID))
	return profileType, nil
}

func (s *EntityProfileTypeService) ListEntityProfileTypes(ctx context.Context, companyID string) ([]*sharedmodels.EntityProfileType, error) {
	types, err := s.repo.GetProfileTypesByCompanyID(ctx, companyID)
	if err != nil {
		s.logger.Error("Failed to list entity profile types", zap.Error(err))
		return nil, err
	}
	s.logger.Info("Entity profile types fetched successfully", zap.Int("count", len(types)))
	return types, nil
}

func (s *EntityProfileTypeService) UpdateEntityProfileType(ctx context.Context, companyID string, id string, req *models.UpdateEntityProfileTypeRequest) (*sharedmodels.EntityProfileType, error) {
	profileType := &sharedmodels.EntityProfileType{
		ID:          id,
		CompanyID:   companyID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := s.repo.UpdateProfileType(ctx, profileType); err != nil {
		s.logger.Error("Failed to update entity profile type", zap.Error(err))
		return nil, err
	}

	s.logger.Info("Entity profile type updated successfully", zap.String("id", id))
	return profileType, nil
}

func (s *EntityProfileTypeService) ArchiveEntityProfileType(ctx context.Context, companyID string, id string) error {
	if err := s.repo.ArchiveProfileType(ctx, companyID, id); err != nil {
		s.logger.Error("Failed to archive entity profile type", zap.Error(err))
		return err
	}

	s.logger.Info("Entity profile type archived safely", zap.String("id", id))
	return nil
}
