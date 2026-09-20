package graphql

import (
	"github.com/jackc/pgx/v5/pgtype"
	"strconv"
	"testing"
	shareddomain "threadify-go/shared/domain"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToGraphQLProfileType(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := toGraphQLProfileType(nil)
		assert.Nil(t, result)
	})

	t.Run("valid input", func(t *testing.T) {
		now := time.Now()
		input := &shareddomain.EntityProfileType{
			ID:          "ept_1",
			CompanyID:   "comp_1",
			Name:        "Test Type",
			Type:        []string{"a", "b"},
			Description: "Test Description",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		result := toGraphQLProfileType(input)
		assert.NotNil(t, result)
		assert.Equal(t, input.ID, result.ID)
		assert.Equal(t, input.CompanyID, result.CompanyID)
		assert.Equal(t, input.Name, result.Name)
		assert.Equal(t, input.Type, result.Type)
		assert.Equal(t, input.Description, *result.Description)
		assert.Equal(t, now.Format(time.RFC3339), result.CreatedAt)
		assert.Equal(t, now.Format(time.RFC3339), result.UpdatedAt)
	})
}

// Decimal rates from PostgreSQL and their JSON/cached equivalents must score
// identically. The real refund workload has 100% completion and 25% breadth.
func TestHealthScorePostgresNumericRates(t *testing.T) {
	numeric := func(value string) any {
		var n pgtype.Numeric
		assert.NoError(t, n.Scan(value))
		return n
	}
	makeMetrics := func(convert func(string) any) map[string]interface{} {
		return map[string]interface{}{
			"success_rate":         map[string]interface{}{"value": convert("100.00")},
			"overall_failure_rate": map[string]interface{}{"value": convert("0.00")},
			"error_rate":           map[string]interface{}{"value": convert("0.00")},
			"recovery_rate":        map[string]interface{}{"value": nil},
			"step_failure_breadth": map[string]interface{}{"value": convert("25.00")},
		}
	}
	fresh := calculateHealthScore(makeMetrics(numeric))
	cached := calculateHealthScore(makeMetrics(func(value string) any { f, err := strconv.ParseFloat(value, 64); assert.NoError(t, err); return f }))
	assert.Equal(t, cached, fresh)
	assert.Equal(t, 93.75, fresh["value"])
	assert.Equal(t, "Healthy", fresh["status"])
	assert.Equal(t, "Stable", fresh["trend"])
	metrics := makeMetrics(numeric)
	metrics["success_rate"] = map[string]interface{}{"value": numeric("30.00")}
	metrics["overall_failure_rate"] = map[string]interface{}{"value": numeric("70.00")}
	low := calculateHealthScore(metrics)
	assert.Equal(t, 44.75, low["value"])
	assert.Equal(t, "Degraded", low["status"])
}
