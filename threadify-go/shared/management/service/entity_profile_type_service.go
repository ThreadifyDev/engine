package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	serror "threadify-go/shared/errors"
	"threadify-go/shared/management/domain"
	"threadify-go/shared/metricbuilder"
	"threadify-go/shared/repository"
	"threadify-go/shared/slug"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func metricName(metric domain.EntityTypeMetric) string {
	if name := strings.TrimSpace(metric.Name); name != "" {
		return name
	}
	if metric.CustomDefinition != nil {
		return strings.TrimSpace(metric.CustomDefinition.Name)
	}
	return ""
}

func metricKey(metric domain.EntityTypeMetric) string {
	return strings.ToLower(metricName(metric))
}

func normalizeParameters(parameters map[string]any) map[string]any {
	if parameters == nil {
		return map[string]any{}
	}
	return parameters
}

func metricsEqual(a, b domain.EntityTypeMetric) bool {
	templateMatches := a.TemplateID == b.TemplateID
	// Custom template IDs are an implementation detail and are intentionally not
	// required in declarative config. The definition and parameters are its state.
	if a.CustomDefinition != nil && b.CustomDefinition != nil && b.TemplateID == "" {
		templateMatches = true
	}
	return strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name) && templateMatches &&
		reflect.DeepEqual(normalizeParameters(a.Parameters), normalizeParameters(b.Parameters)) &&
		reflect.DeepEqual(a.CustomDefinition, b.CustomDefinition)
}

func sameStrings(a, b []string) bool {
	left, right := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	return slices.Equal(left, right)
}

func configHash(profile *domain.EntityProfileType) string {
	types := append([]string(nil), profile.Type...)
	sort.Strings(types)
	metrics := append([]domain.EntityTypeMetric(nil), profile.Metrics...)
	sort.Slice(metrics, func(i, j int) bool { return metricKey(metrics[i]) < metricKey(metrics[j]) })
	for i := range metrics {
		metrics[i].ID = ""
		if metrics[i].CustomDefinition != nil {
			metrics[i].TemplateID = ""
		}
		metrics[i].Parameters = normalizeParameters(metrics[i].Parameters)
	}
	payload, _ := json.Marshal(struct {
		Name        string                    `json:"name"`
		Type        []string                  `json:"type"`
		Description string                    `json:"description"`
		Metrics     []domain.EntityTypeMetric `json:"metrics"`
	}{profile.Name, types, profile.Description, metrics})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (s *EntityProfileTypeService) hydrateMetricNames(ctx context.Context, metrics []domain.EntityTypeMetric) ([]domain.EntityTypeMetric, error) {
	templates, err := s.repo.ListMetricsTemplates(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]string, len(templates))
	for _, template := range templates {
		byID[template.ID] = template.MetricsName
	}
	seen := make(map[string]struct{}, len(metrics))
	result := append([]domain.EntityTypeMetric(nil), metrics...)
	for i := range result {
		if result[i].CustomDefinition != nil {
			if _, err := metricbuilder.BuildSQLFromDefinition(result[i].CustomDefinition); err != nil {
				return nil, err
			}
		}
		if result[i].Name == "" && result[i].CustomDefinition != nil {
			result[i].Name = strings.TrimSpace(result[i].CustomDefinition.Name)
		}
		if result[i].Name == "" {
			result[i].Name = strings.TrimSpace(byID[result[i].TemplateID])
		}
		key := metricKey(result[i])
		if key == "" {
			return nil, serror.NewDomainError("each metric must have a name or valid template_id", 400)
		}
		if _, exists := seen[key]; exists {
			return nil, serror.NewDomainError("metric names must be unique", 400)
		}
		seen[key] = struct{}{}
	}
	return result, nil
}

// ApplyEntityProfileType reconciles a complete desired declaration against the
// stored profile. The path slug is the identity, so a different declared name is
// rejected by the handler instead of being interpreted as a rename.
func (s *EntityProfileTypeService) ApplyEntityProfileType(
	ctx context.Context,
	companyID, profileSlug string,
	req *domain.ApplyEntityProfileTypeCmd,
	dryRun bool,
) (*domain.ApplyEntityProfileTypeResult, error) {
	desiredMetrics, err := s.hydrateMetricNames(ctx, req.Metrics)
	if err != nil {
		return nil, err
	}
	desired := &domain.EntityProfileType{
		CompanyID: companyID, Name: strings.TrimSpace(req.Name), Slug: profileSlug,
		Type: normalizeTypes(req.Type), Description: req.Description, Metrics: desiredMetrics,
	}

	current, err := s.repo.GetProfileTypeByType(ctx, companyID, profileSlug)
	if err != nil && !errors.Is(err, serror.ErrEntityProfileTypeNotFound) {
		return nil, err
	}
	changes := domain.EntityProfileTypeChanges{
		MetricsAdded: []string{}, MetricsUpdated: []string{}, MetricsRemoved: []string{},
	}
	status := "created"
	if current != nil {
		desired.ID, desired.CreatedAt, desired.UpdatedAt = current.ID, current.CreatedAt, current.UpdatedAt
		changes.NameChanged = current.Name != desired.Name
		changes.DescriptionChanged = current.Description != desired.Description
		changes.TypesChanged = !sameStrings(current.Type, desired.Type)
		currentByName := make(map[string]domain.EntityTypeMetric, len(current.Metrics))
		for _, metric := range current.Metrics {
			currentByName[metricKey(metric)] = metric
		}
		desiredByName := make(map[string]domain.EntityTypeMetric, len(desired.Metrics))
		for _, metric := range desired.Metrics {
			desiredByName[metricKey(metric)] = metric
			if old, ok := currentByName[metricKey(metric)]; !ok {
				changes.MetricsAdded = append(changes.MetricsAdded, metricName(metric))
			} else if !metricsEqual(old, metric) {
				changes.MetricsUpdated = append(changes.MetricsUpdated, metricName(metric))
			}
		}
		for _, metric := range current.Metrics {
			if _, ok := desiredByName[metricKey(metric)]; !ok {
				changes.MetricsRemoved = append(changes.MetricsRemoved, metricName(metric))
			}
		}
		if !changes.NameChanged && !changes.DescriptionChanged && !changes.TypesChanged &&
			len(changes.MetricsAdded) == 0 && len(changes.MetricsUpdated) == 0 && len(changes.MetricsRemoved) == 0 {
			status = "unchanged"
		} else {
			status = "updated"
		}
	}

	result := &domain.ApplyEntityProfileTypeResult{
		Status: status, DryRun: dryRun, ConfigHash: configHash(desired), Profile: desired,
		Changes: changes, Backfill: domain.EntityProfileTypeBackfill{Supported: false, Applied: false},
	}
	if dryRun || status == "unchanged" {
		if current != nil && status == "unchanged" {
			result.Profile = current
		}
		return result, nil
	}

	if current == nil {
		desired.ID = uuid.New().String()
		desired.CreatedAt, desired.UpdatedAt = time.Now(), time.Now()
		desired.Metrics, err = s.resolveCustomMetrics(ctx, companyID, desired.Metrics, nil)
		if err != nil {
			return nil, err
		}
		if err := s.repo.CreateProfileType(ctx, desired); err != nil {
			return nil, err
		}
		return result, nil
	}

	currentByName := make(map[string]domain.EntityTypeMetric, len(current.Metrics))
	for _, metric := range current.Metrics {
		currentByName[metricKey(metric)] = metric
	}
	modifiedIDs := make([]string, 0)
	for i := range desired.Metrics {
		if old, ok := currentByName[metricKey(desired.Metrics[i])]; ok {
			desired.Metrics[i].ID = old.ID
			if desired.Metrics[i].TemplateID == "" {
				desired.Metrics[i].TemplateID = old.TemplateID
			}
			if !metricsEqual(old, desired.Metrics[i]) {
				modifiedIDs = append(modifiedIDs, old.ID)
			}
		}
	}
	deletions := make([]string, 0)
	for _, old := range current.Metrics {
		found := false
		for _, next := range desired.Metrics {
			if metricKey(next) == metricKey(old) {
				found = true
				break
			}
		}
		if !found {
			deletions = append(deletions, old.ID)
		}
	}
	desired.Metrics, err = s.resolveCustomMetrics(ctx, companyID, desired.Metrics, modifiedIDs)
	if err != nil {
		return nil, err
	}
	desired.MarkedForDeletion, desired.ModifiedMetricIDs = deletions, modifiedIDs
	if err := s.repo.UpdateProfileType(ctx, desired); err != nil {
		return nil, err
	}
	return result, nil
}

// RenameEntityProfileType is deliberately separate from ApplyEntityProfileType
// because a name change also changes the profile's external identity.
func (s *EntityProfileTypeService) RenameEntityProfileType(
	ctx context.Context,
	companyID, profileSlug, name string,
) (*domain.EntityProfileType, error) {
	current, err := s.repo.GetProfileTypeByType(ctx, companyID, profileSlug)
	if err != nil {
		return nil, err
	}
	newName := strings.TrimSpace(name)
	newSlug := slug.ToSlug(newName)
	if newSlug == "" {
		return nil, serror.NewDomainError("name is required", 400)
	}
	if newSlug != profileSlug {
		if _, err := s.repo.GetProfileTypeByType(ctx, companyID, newSlug); err == nil {
			return nil, serror.ErrEntityProfileTypeAlreadyExists
		} else if !errors.Is(err, serror.ErrEntityProfileTypeNotFound) {
			return nil, err
		}
	}
	current.Name, current.Slug, current.UpdatedAt = newName, newSlug, time.Now()
	if err := s.repo.UpdateProfileType(ctx, current); err != nil {
		return nil, err
	}
	return current, nil
}

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

	if len(metrics) > 0 {
		for _, m := range metrics {
			tmpl, err := s.repo.GetMetricsTemplate(ctx, m.TemplateID)
			if err != nil {
				return nil, err
			}
			if err := s.repo.ValidateMetricsSQL(ctx, tmpl.SQLContent, m.Parameters); err != nil {
				s.logger.Warn("invalid metric template bind attempt", zap.String("templateID", m.TemplateID), zap.Error(err))
				return nil, err
			}
		}
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

	if len(metrics) > 0 {
		for _, m := range metrics {
			tmpl, err := s.repo.GetMetricsTemplate(ctx, m.TemplateID)
			if err != nil {
				return nil, err
			}
			if err := s.repo.ValidateMetricsSQL(ctx, tmpl.SQLContent, m.Parameters); err != nil {
				s.logger.Warn("invalid metric template bind attempt during update", zap.String("templateID", m.TemplateID), zap.Error(err))
				return nil, err
			}
		}
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

			if isModified && strings.HasPrefix(m.TemplateID, "custom_") {
				// Update existing custom template
				if err := s.repo.UpdateMetricsTemplate(ctx, m.TemplateID, m.CustomDefinition.Name, sql); err != nil {
					s.logger.Error("failed to update metrics template from custom definition", zap.Error(err))
					return nil, err
				}
			} else if m.TemplateID == "" || !strings.HasPrefix(m.TemplateID, "custom_") {
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
	profileSlug string,
) error {
	profileType, err := s.repo.GetProfileTypeByType(ctx, companyID, profileSlug)
	if err != nil {
		return err
	}
	if err := s.repo.ArchiveProfileType(ctx, companyID, profileType.ID); err != nil {
		s.logger.Error("failed to archive entity profile type", zap.Error(err))
		return err
	}

	s.logger.Info("entity profile type archived successfully", zap.String("slug", profileSlug))
	return nil
}
