package contractcontent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContradictions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules []Rule
		bad   bool
	}{
		{"different literals", []Rule{{Field: "status", Operator: "equals", Value: "approved"}, {Field: "status", Operator: "equals", Value: "rejected"}}, true},
		{"disjoint enums", []Rule{{Field: "status", Operator: "one_of", Values: []string{"approved"}}, {Field: "status", Operator: "one_of", Values: []string{"rejected"}}}, true},
		{"empty enum intersection", []Rule{{Field: "status", Operator: "equals", Value: "approved"}, {Field: "status", Operator: "one_of", Values: []string{"rejected"}}}, true},
		{"exclusive equal bounds", []Rule{{Field: "amount", Operator: "gt", Value: "1"}, {Field: "amount", Operator: "lte", Value: "1"}}, true},
		{"number and boolean", []Rule{{Field: "value", Operator: "number"}, {Field: "value", Operator: "boolean"}}, true},
		{"literal fails regex", []Rule{{Field: "code", Operator: "equals", Value: "ABC"}, {Field: "code", Operator: "matches", Value: "^[0-9]+$"}}, true},
		{"nonempty whitespace", []Rule{{Field: "code", Operator: "equals", Value: "  "}, {Field: "code", Operator: "nonempty"}}, true},
		{"compatible range", []Rule{{Field: "amount", Operator: "gt", Value: "1"}, {Field: "amount", Operator: "lt", Value: "2"}}, false},
		{"compatible enum", []Rule{{Field: "status", Operator: "one_of", Values: []string{"approved", "pending"}}, {Field: "status", Operator: "matches", Value: "^approved$"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.bad, len(Contradictions(tc.rules)) > 0)
		})
	}
}
