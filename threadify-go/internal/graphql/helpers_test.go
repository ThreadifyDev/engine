package graphql

import (
	"testing"
	sharedmodels "threadify-go/shared/domain"
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
		input := &sharedmodels.EntityProfileType{
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
