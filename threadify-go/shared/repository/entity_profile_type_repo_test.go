package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractParameters(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "single parameter",
			sql:      "SELECT * FROM table WHERE col = @param1",
			expected: []string{"param1"},
		},
		{
			name:     "multiple parameters",
			sql:      "SELECT * FROM table WHERE col1 = @param1 AND col2 = @param2",
			expected: []string{"param1", "param2"},
		},
		{
			name:     "duplicate parameters",
			sql:      "SELECT * FROM table WHERE col1 = @param1 OR col2 = @param1",
			expected: []string{"param1"},
		},
		{
			name:     "ignores runtime parameters",
			sql:      "SELECT * FROM t WHERE ref_value = @ref_value AND start = @start_time AND x = @custom",
			expected: []string{"custom"},
		},
		{
			name:     "no parameters",
			sql:      "SELECT * FROM table",
			expected: []string{},
		},
		{
			name:     "parameters with underscores and numbers",
			sql:      "SELECT * FROM table WHERE col = @param_1_alt",
			expected: []string{"param_1_alt"},
		},
		{
			name:     "ignores email addresses",
			sql:      "SELECT * FROM users WHERE email = 'user@example.com' AND id = @user_id",
			expected: []string{"user_id"},
		},
		{
			name:     "parameter at end of string",
			sql:      "SELECT * FROM table WHERE col = @param",
			expected: []string{"param"},
		},
		{
			name:     "parameter at start of string",
			sql:      "@param = col",
			expected: []string{"param"},
		},
		{
			name:     "empty string",
			sql:      "",
			expected: []string{},
		},
		{
			name:     "multiline SQL",
			sql:      "SELECT *\nFROM table\nWHERE col = @param1\n  AND col2 = @param2",
			expected: []string{"param1", "param2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := extractParameters(tt.sql)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
