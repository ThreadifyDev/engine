package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sharedauth "threadify-go/shared/auth"
	sharedmodels "threadify-go/shared/domain"

	"github.com/threadify/engine/internal/graphql/generated"
)

// Context keys for caching
type contextKey string

const (
	accessCacheKey contextKey = "thread_access_cache"
	stepsCacheKey  contextKey = "thread_steps_cache"
)

// getUserInfoFromContext extracts user info from GraphQL context
func getUserInfoFromContext(ctx context.Context) (ownerID, companyID, role string, err error) {
	ownerIDVal := ctx.Value(sharedauth.CtxUserID)
	companyIDVal := ctx.Value(sharedauth.CtxCompanyID)
	rolesVal := ctx.Value(sharedauth.CtxRoles)

	if ownerIDVal == nil || companyIDVal == nil || rolesVal == nil {
		return "", "", "", fmt.Errorf("user authentication context not found")
	}

	ownerID, ownerOK := ownerIDVal.(string)
	companyID, companyOK := companyIDVal.(string)
	roles, rolesOK := rolesVal.([]string)

	if !ownerOK || !companyOK || !rolesOK {
		return "", "", "", fmt.Errorf("invalid user authentication context types")
	}

	extractedRole := ""
	if len(roles) > 0 {
		extractedRole = roles[0]
	}

	return ownerID, companyID, extractedRole, nil
}

// cacheAccessCheck stores an access check result in the context
func cacheAccessCheck(ctx context.Context, threadID string, hasAccess bool) context.Context {
	cache, ok := ctx.Value(accessCacheKey).(map[string]bool)
	if !ok {
		cache = make(map[string]bool)
	}
	cache[threadID] = hasAccess
	return context.WithValue(ctx, accessCacheKey, cache)
}

// getCachedAccessCheck retrieves a cached access check result
func getCachedAccessCheck(ctx context.Context, threadID string) (hasAccess bool, found bool) {
	cache, ok := ctx.Value(accessCacheKey).(map[string]bool)
	if !ok {
		return false, false
	}
	hasAccess, found = cache[threadID]
	return hasAccess, found
}

// cacheSteps stores batch-loaded steps in the context
func cacheSteps(ctx context.Context, stepsMap map[string]interface{}) context.Context {
	return context.WithValue(ctx, stepsCacheKey, stepsMap)
}

// getCachedSteps retrieves cached steps for a thread
func getCachedSteps(ctx context.Context, threadID string) (steps interface{}, found bool) {
	cache, ok := ctx.Value(stepsCacheKey).(map[string]interface{})
	if !ok {
		return nil, false
	}
	steps, found = cache[threadID]
	return steps, found
}

func toGraphQLMetrics(m *sharedmodels.EntityProfileMetrics) *generated.EntityProfileMetrics {
	if m == nil {
		return nil
	}
	out := &generated.EntityProfileMetrics{
		EntityProfileID:         m.EntityProfileID,
		TotalDeliveries:         m.TotalDeliveries,
		CompletedSuccessfully:   m.CompletedSuccessfully,
		ValidationViolations:    m.ValidationViolations,
		DeliveryHealthScore:     m.DeliveryHealthScore,
		PrevDeliveryHealthScore: m.PrevDeliveryHealthScore,
		HealthTrendSlope:        m.HealthTrendSlope,
	}
	if m.AverageDeliveryTimeMs != nil {
		v := int(*m.AverageDeliveryTimeMs)
		out.AverageDeliveryTimeMs = &v
	}
	if m.LastCalculatedAt != nil {
		v := m.LastCalculatedAt.Format(time.RFC3339)
		out.LastCalculatedAt = &v
	}
	return out
}

func toGraphQLProfileType(t *sharedmodels.EntityProfileType) *generated.EntityProfileType {
	if t == nil {
		return nil
	}
	desc := t.Description
	var metricsConfig []*generated.EntityTypeMetricConfig
	for _, m := range t.Metrics {
		// we need to safely pass map[string]any via the string encoding or just as string?
		// Wait, GraphQL JSON is mapped to string currently, but let's check what it expects.
		// If the schema mapped it to string, we need to marshal it. If to any, we can pass map.
		// We'll leave the marshalling logic inside helpers.go
		var paramsStr *string
		if m.Parameters != nil {
			if b, err := json.Marshal(m.Parameters); err == nil {
				s := string(b)
				paramsStr = &s
			}
		}
		var nameStr *string
		if m.Name != "" {
			name := m.Name
			nameStr = &name
		}
		metricsConfig = append(metricsConfig, &generated.EntityTypeMetricConfig{
			TemplateID: m.TemplateID,
			Name:       nameStr,
			Parameters: paramsStr,
		})
	}

	return &generated.EntityProfileType{
		ID:            t.ID,
		CompanyID:     t.CompanyID,
		Name:          t.Name,
		Type:          t.Type,
		Description:   &desc,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     t.UpdatedAt.Format(time.RFC3339),
		MetricsConfig: metricsConfig,
	}
}

// formatColumnName converts snake_case to Title Case
func formatColumnName(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
