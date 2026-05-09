package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/shared/metricbuilder"
	"threadify-go/shared/repository"
	"threadify-go/shared/slug"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EntityProfileTypeService struct {
	repo   repository.EntityProfileTypeRepository
	logger *zap.Logger
}

func NewEntityProfileTypeService(
	repo repository.EntityProfileTypeRepository,
	logger *zap.Logger,
) *EntityProfileTypeService {
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

func (s *EntityProfileTypeService) CreateEntityProfileType(
	ctx context.Context,
	companyID string,
	req *domain.CreateEntityProfileTypeCmd,
) (*domain.EntityProfileType, error) {
	metrics, err := s.resolveCustomMetrics(ctx, companyID, req.Metrics, nil)
	if err != nil {
		return nil, err
	}

	profileType := &domain.EntityProfileType{
		ID:          uuid.New().String(),
		CompanyID:   companyID,
		Name:        req.Name,
		Slug:        slug.ToSlug(req.Name),
		Type:        normalizeTypes(req.Type),
		Description: req.Description,
		Metrics:     metrics,
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

func (s *EntityProfileTypeService) ListEntityProfileTypes(
	ctx context.Context,
	companyID string,
) ([]*domain.EntityProfileType, error) {
	types, err := s.repo.GetProfileTypesByCompanyID(ctx, companyID)
	if err != nil {
		s.logger.Error("failed to list entity profile types", zap.Error(err))
		return nil, err
	}

	s.logger.Info("entity profile types fetched successfully", zap.Int("count", len(types)))
	return types, nil
}

func (s *EntityProfileTypeService) ListMetricsTemplates(
	ctx context.Context,
) ([]*domain.MetricsTemplate, error) {
	templates, err := s.repo.ListMetricsTemplates(ctx)
	if err != nil {
		s.logger.Error("failed to list metrics templates", zap.Error(err))
		return nil, err
	}

	return templates, nil
}

func (s *EntityProfileTypeService) UpdateEntityProfileType(
	ctx context.Context,
	companyID, id string,
	req *domain.UpdateEntityProfileTypeCmd,
) (*domain.EntityProfileType, error) {
	var nextTypes []string
	if req.Type != nil {
		nextTypes = normalizeTypes(req.Type)
	}

	metrics, err := s.resolveCustomMetrics(ctx, companyID, req.Metrics, req.ModifiedMetricIDs)
	if err != nil {
		return nil, err
	}

	profileType := &domain.EntityProfileType{
		ID:                id,
		CompanyID:         companyID,
		Name:              req.Name,
		Slug:              slug.ToSlug(req.Name),
		Description:       req.Description,
		Type:              nextTypes,
		Metrics:           metrics,
		UpdatedAt:         time.Now(),
		MarkedForDeletion: req.MarkedForDeletion,
		ModifiedMetricIDs: req.ModifiedMetricIDs,
	}

	if err := s.repo.UpdateProfileType(ctx, profileType); err != nil {
		s.logger.Error("failed to update entity profile type", zap.Error(err))
		return nil, err
	}

	s.logger.Info("entity profile type updated successfully", zap.String("id", id))
	return profileType, nil
}

func (s *EntityProfileTypeService) resolveCustomMetrics(ctx context.Context, companyID string, metrics []domain.EntityTypeMetric, modifiedIDs []string) ([]domain.EntityTypeMetric, error) {
	resolved := make([]domain.EntityTypeMetric, 0, len(metrics))
	for _, m := range metrics {
		if m.CustomDefinition != nil {
			sql, err := metricbuilder.BuildSQLFromDefinition(m.CustomDefinition)
			if err != nil {
				s.logger.Error("failed to build SQL from custom definition", zap.Error(err))
				return nil, err
			}

			isModified := m.ID != "" && slices.Contains(modifiedIDs, m.ID)

			if isModified {
				// Update existing custom template
				if err := s.repo.UpdateMetricsTemplate(ctx, m.TemplateID, m.CustomDefinition.Name, sql); err != nil {
					s.logger.Error("failed to update metrics template from custom definition", zap.Error(err))
					return nil, err
				}
			} else if m.ID == "" {
				// Create new custom template
				templateID := "custom_" + uuid.New().String()
				if err := s.repo.CreateMetricsTemplate(ctx, companyID, templateID, m.CustomDefinition.Name, sql); err != nil {
					s.logger.Error("failed to create metrics template from custom definition", zap.Error(err))
					return nil, err
				}
				m.TemplateID = templateID
			}

			params := m.Parameters
			if params == nil {
				params = make(map[string]any)
			}
			if m.CustomDefinition.Target == "step" && m.CustomDefinition.StepName != "" {
				params["step_name"] = m.CustomDefinition.StepName
			}

			resolved = append(resolved, domain.EntityTypeMetric{
				ID:               m.ID,
				TemplateID:       m.TemplateID,
				Name:             m.CustomDefinition.Name,
				Parameters:       params,
				CustomDefinition: m.CustomDefinition,
			})
		} else {
			resolved = append(resolved, m)
		}
	}
	return resolved, nil
}

func (s *EntityProfileTypeService) ArchiveEntityProfileType(
	ctx context.Context,
	companyID string,
	id string,
) error {
	if err := s.repo.ArchiveProfileType(ctx, companyID, id); err != nil {
		s.logger.Error("failed to archive entity profile type", zap.Error(err))
		return err
	}

	s.logger.Info("entity profile type archived successfully", zap.String("id", id))
	return nil
}
