package graphql

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	sharedauth "threadify-go/shared/auth"
	shareddomain "threadify-go/shared/domain"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/graphql/generated"
)

// Context keys for caching
type contextKey string

const (
	accessCacheKey contextKey = "thread_access_cache"
	stepsCacheKey  contextKey = "thread_steps_cache"
)

// getUserInfoFromContext extracts user info from GraphQL context
func getUserInfoFromContext(ctx context.Context) (*domain.AuthContext, error) {
	ownerIDVal := ctx.Value(sharedauth.CtxUserID)
	companyIDVal := ctx.Value(sharedauth.CtxCompanyID)
	rolesVal := ctx.Value(sharedauth.CtxRoles)

	if ownerIDVal == nil || companyIDVal == nil || rolesVal == nil {
		return nil, fmt.Errorf("user authentication context not found")
	}

	ownerID, ownerOK := ownerIDVal.(string)
	companyID, companyOK := companyIDVal.(string)
	roles, rolesOK := rolesVal.([]string)

	if !ownerOK || !companyOK || !rolesOK {
		return nil, fmt.Errorf("invalid user authentication context types")
	}

	extractedRole := ""
	if len(roles) > 0 {
		extractedRole = roles[0]
	}

	return &domain.AuthContext{
		OwnerID:   ownerID,
		CompanyID: companyID,
		Role:      extractedRole,
	}, nil
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

func calculateHealthScore(metrics map[string]interface{}) map[string]interface{} {
	// Extract metric values with safe type assertions
	getFloatValue := func(key string) float64 {
		if m, ok := metrics[key].(map[string]interface{}); ok {
			// PostgreSQL ROUND/AVG/ratios arrive as pgtype.Numeric through
			// RowToMap. JSON renders them as numbers, but a float64 assertion
			// alone silently discards the same values during scoring.
			if val, ok := m["value"].(pgtype.Numeric); ok {
				converted, err := val.Float64Value()
				if err == nil && converted.Valid && !math.IsNaN(converted.Float64) && !math.IsInf(converted.Float64, 0) {
					return converted.Float64
				}
			}
			if val, ok := m["value"].(float64); ok {
				return val
			}
			if val, ok := m["value"].(int64); ok {
				return float64(val)
			}
		}
		return 0
	}

	successRate := getFloatValue("success_rate")
	failureRate := getFloatValue("overall_failure_rate")
	errorRate := getFloatValue("error_rate")
	recoveryRate := getFloatValue("recovery_rate")
	stepFailureBreadth := getFloatValue("step_failure_breadth")

	// Weighted scoring algorithm
	// Success rate: 40% weight (higher is better)
	// Failure rate: 30% weight (lower is better)
	// Error rate: 15% weight (lower is better)
	// Recovery rate: 10% weight (higher is better, null = neutral)
	// Step failure breadth: 5% weight (lower is better)

	score := 0.0

	// Success rate contribution (0-40 points)
	score += (successRate / 100.0) * 40.0

	// Failure rate contribution (0-30 points, inverted)
	score += ((100.0 - failureRate) / 100.0) * 30.0

	// Error rate contribution (0-15 points, inverted)
	score += ((100.0 - errorRate) / 100.0) * 15.0

	// Recovery rate contribution (0-10 points)
	if recoveryRate > 0 {
		score += (recoveryRate / 100.0) * 10.0
	} else {
		// Neutral if no recovery data
		score += 5.0
	}

	// Step failure breadth contribution (0-5 points, inverted)
	score += ((100.0 - stepFailureBreadth) / 100.0) * 5.0

	// Clamp to 0-100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// Determine status band
	var status string
	var trend string

	if score >= 80 {
		status = "Healthy"
	} else if score >= 60 {
		status = "At Risk"
	} else if score >= 40 {
		status = "Degraded"
	} else {
		status = "Critical"
	}

	// Simple trend analysis based on success vs failure
	if successRate >= 80 && failureRate <= 20 {
		trend = "Stable"
	} else if successRate >= 60 && failureRate <= 40 {
		trend = "Improving"
	} else if failureRate > 50 {
		trend = "Declining"
	} else {
		trend = "Fluctuating"
	}

	return map[string]interface{}{
		"value":       score,
		"status":      status,
		"trend":       trend,
		"description": "Overall delivery health score based on success rate, failure patterns, and recovery capability.",
	}
}
