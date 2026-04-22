package service

import (
	"context"
	"strings"
	"time"

	"threadify-go/api/internal/models"
	sharedmodels "threadify-go/shared/models"
	"threadify-go/shared/repository"
	"threadify-go/shared/slug"

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

func normalizeTypes(types []string) []string {
	seen := make(map[string]struct{}, len(types))
	out := make([]string, 0, len(types))

	for _, t := range types {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}

	return out
}

func (s *EntityProfileTypeService) CreateEntityProfileType(ctx context.Context, companyID string, req *models.CreateEntityProfileTypeRequest) (*sharedmodels.EntityProfileType, error) {
	profileType := &sharedmodels.EntityProfileType{
		ID:          uuid.New().String(),
		CompanyID:   companyID,
		Name:        req.Name,
		Slug:        slug.ToSlug(req.Name),
		Type:        normalizeTypes(req.Type),
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.repo.CreateProfileType(ctx, profileType); err != nil {
		s.logger.Error("failed to create entity profile type", zap.Error(err))
		return nil, err
	}

	s.logger.Info("entity profile type created successfully", zap.String("id", profileType.ID))
	return profileType, nil
}

func (s *EntityProfileTypeService) ListEntityProfileTypes(ctx context.Context, companyID string) ([]*sharedmodels.EntityProfileType, error) {
	types, err := s.repo.GetProfileTypesByCompanyID(ctx, companyID)
	if err != nil {
		s.logger.Error("failed to list entity profile types", zap.Error(err))
		return nil, err
	}

	s.logger.Info("entity profile types fetched successfully", zap.Int("count", len(types)))
	return types, nil
}

func (s *EntityProfileTypeService) UpdateEntityProfileType(ctx context.Context, companyID, id string, req *models.UpdateEntityProfileTypeRequest) (*sharedmodels.EntityProfileType, error) {
	var nextTypes []string
	if req.Type != nil {
		nextTypes = normalizeTypes(req.Type)
	}

	profileType := &sharedmodels.EntityProfileType{
		ID:          id,
		CompanyID:   companyID,
		Name:        req.Name,
		Slug:        slug.ToSlug(req.Name),
		Description: req.Description,
		Type:        nextTypes,
	}

	if err := s.repo.UpdateProfileType(ctx, profileType); err != nil {
		s.logger.Error("failed to update entity profile type", zap.Error(err))
		return nil, err
	}

	s.logger.Info("entity profile type updated successfully", zap.String("id", id))
	return profileType, nil
}

func (s *EntityProfileTypeService) ArchiveEntityProfileType(ctx context.Context, companyID, id string) error {
	if err := s.repo.ArchiveProfileType(ctx, companyID, id); err != nil {
		s.logger.Error("failed to archive entity profile type", zap.Error(err))
		return err
	}

	s.logger.Info("entity profile type archived successfully", zap.String("id", id))
	return nil
}
