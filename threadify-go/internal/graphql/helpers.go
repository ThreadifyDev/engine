package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/graphql/generated"
	sharedauth "threadify-go/shared/auth"
	sharedmodels "threadify-go/shared/models"
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
		EntityProfileID:       m.EntityProfileID,
		TotalDeliveries:       m.TotalDeliveries,
		CompletedSuccessfully: m.CompletedSuccessfully,
		ValidationViolations:  m.ValidationViolations,
		DeliveryHealthScore:   m.DeliveryHealthScore,
		HealthTrendSlope:      m.HealthTrendSlope,
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
	desc := t.Description
	return &generated.EntityProfileType{
		ID:          t.ID,
		CompanyID:   t.CompanyID,
		Name:        t.Name,
		Type:        t.Type,
		Description: &desc,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
	}
}
