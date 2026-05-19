package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	sharedauth "threadify-go/shared/auth"
	shareddomain "threadify-go/shared/domain"

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

func toGraphQLProfileType(t *shareddomain.EntityProfileType) *generated.EntityProfileType {
	if t == nil {
		return nil
	}
	desc := t.Description
	var metricsConfig []*generated.EntityTypeMetricConfig
	for _, m := range t.Metrics {
		var nameStr, idStr *string
		if m.Name != "" {
			name := m.Name
			nameStr = &name
		}
		if m.ID != "" {
			id := m.ID
			idStr = &id
		}
		var customDef *generated.CustomMetricDefinition
		if m.CustomDefinition != nil {
			d := m.CustomDefinition
			customDef = &generated.CustomMetricDefinition{
				Name:          d.Name,
				Operation:     strPtr(d.Operation),
				Field:         strPtr(d.Field),
				GroupBy:       strPtr(d.GroupBy),
				Granularity:   strPtr(d.Granularity),
				Visualisation: strPtr(d.Visualisation),
				Target:        strPtr(d.Target),
				StepName:      strPtr(d.StepName),
			}
			if len(d.Filters) > 0 {
				filters := make([]map[string]interface{}, len(d.Filters))
				for i, f := range d.Filters {
					filters[i] = map[string]interface{}{
						"key":   f.Key,
						"value": f.Value,
					}
				}
				customDef.Filters = filters
			}
		}
		metricsConfig = append(metricsConfig, &generated.EntityTypeMetricConfig{
			ID:               idStr,
			TemplateID:       m.TemplateID,
			Name:             nameStr,
			Parameters:       m.Parameters,
			CustomDefinition: customDef,
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

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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

// toSnakeCase converts any display string to snake_case.
// It lowercases, replaces spaces/hyphens with underscores, and collapses multiple underscores.
func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "_")
	// Replace any run of non-alphanumeric chars (except underscore) with underscore
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteRune('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}
