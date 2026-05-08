package repository

import (
	"testing"

	"threadify-go/shared/domain"

	"github.com/stretchr/testify/assert"
)

func TestExtractParameters(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []domain.ParameterDef
	}{
		{
			name: "single parameter fallback to string",
			sql:  "SELECT * FROM table WHERE col = @param1",
			expected: []domain.ParameterDef{
				{Name: "param1", Type: "string"},
			},
		},
		{
			name: "multiple parameters fallback to string",
			sql:  "SELECT * FROM table WHERE col1 = @param1 AND col2 = @param2",
			expected: []domain.ParameterDef{
				{Name: "param1", Type: "string"},
				{Name: "param2", Type: "string"},
			},
		},
		{
			name: "duplicate parameters",
			sql:  "SELECT * FROM table WHERE col1 = @param1 OR col2 = @param1",
			expected: []domain.ParameterDef{
				{Name: "param1", Type: "string"},
			},
		},
		{
			name: "ignores runtime parameters",
			sql:  "SELECT * FROM t WHERE ref_value = @ref_value AND start = @start_time AND x = @custom",
			expected: []domain.ParameterDef{
				{Name: "custom", Type: "string"},
			},
		},
		{
			name:     "no parameters",
			sql:      "SELECT * FROM table",
			expected: []domain.ParameterDef{},
		},
		{
			name: "parameters with underscores and numbers",
			sql:  "SELECT * FROM table WHERE col = @param_1_alt",
			expected: []domain.ParameterDef{
				{Name: "param_1_alt", Type: "string"},
			},
		},
		{
			name: "ignores email addresses",
			sql:  "SELECT * FROM users WHERE email = 'user@example.com' AND id = @user_id",
			expected: []domain.ParameterDef{
				{Name: "user_id", Type: "string"},
			},
		},
		{
			name: "parameter at end of string",
			sql:  "SELECT * FROM table WHERE col = @param",
			expected: []domain.ParameterDef{
				{Name: "param", Type: "string"},
			},
		},
		{
			name: "parameter at start of string",
			sql:  "@param = col",
			expected: []domain.ParameterDef{
				{Name: "param", Type: "string"},
			},
		},
		{
			name:     "empty string",
			sql:      "",
			expected: []domain.ParameterDef{},
		},
		{
			name: "multiline SQL",
			sql:  "SELECT *\nFROM table\nWHERE col = @param1\n  AND col2 = @param2",
			expected: []domain.ParameterDef{
				{Name: "param1", Type: "string"},
				{Name: "param2", Type: "string"},
			},
		},
		{
			name: "enum parameter from comment",
			sql:  "-- @param status enum(active, completed, failed) Thread status\nSELECT * FROM threads WHERE status = @status",
			expected: []domain.ParameterDef{
				{Name: "status", Type: "enum", Values: []string{"active", "completed", "failed"}, Description: "Thread status"},
			},
		},
		{
			name: "number parameter from comment",
			sql:  "-- @param min_amount number Minimum amount\nSELECT * WHERE amount >= @min_amount",
			expected: []domain.ParameterDef{
				{Name: "min_amount", Type: "number", Description: "Minimum amount"},
			},
		},
		{
			name: "boolean parameter from comment",
			sql:  "-- @param include_retries boolean Include retries\nSELECT * WHERE retries = @include_retries",
			expected: []domain.ParameterDef{
				{Name: "include_retries", Type: "boolean", Description: "Include retries"},
			},
		},
		{
			name: "comment order preserved then fallback",
			sql:  "-- @param status enum(active, completed) Status filter\nSELECT * WHERE status = @status AND id = @user_id",
			expected: []domain.ParameterDef{
				{Name: "status", Type: "enum", Values: []string{"active", "completed"}, Description: "Status filter"},
				{Name: "user_id", Type: "string"},
			},
		},
		{
			name: "ignores commented runtime params",
			sql:  "-- @param ref_value enum(a,b) Should be ignored\nSELECT * WHERE x = @custom",
			expected: []domain.ParameterDef{
				{Name: "custom", Type: "string"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := extractParameters(tt.sql)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
