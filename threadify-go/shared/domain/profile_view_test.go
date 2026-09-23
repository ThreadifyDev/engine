package domain

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func testView() ProfileViewDefinition {
	return ProfileViewDefinition{SchemaVersion: 1, Title: "Overview", Range: "30d", Columns: 2, Blocks: []ProfileViewBlock{{ID: "health", Source: "deliveryHealth", Title: "Health"}}}
}
func TestProfileViewValidation(t *testing.T) {
	tests := map[string]func(*ProfileViewDefinition){
		"version":                             func(v *ProfileViewDefinition) { v.SchemaVersion = 2 },
		"blank title":                         func(v *ProfileViewDefinition) { v.Title = " " },
		"long title":                          func(v *ProfileViewDefinition) { v.Title = strings.Repeat("a", 101) },
		"unicode title matches browser limit": func(v *ProfileViewDefinition) { v.Title = strings.Repeat("😀", 51) },
		"description":                         func(v *ProfileViewDefinition) { v.Description = strings.Repeat("a", 501) },
		"range":                               func(v *ProfileViewDefinition) { v.Range = "all" },
		"columns":                             func(v *ProfileViewDefinition) { v.Columns = 3 },
		"empty":                               func(v *ProfileViewDefinition) { v.Blocks = nil },
		"history":                             func(v *ProfileViewDefinition) { v.Blocks[0].Source = "history" },
		"code":                                func(v *ProfileViewDefinition) { v.Blocks[0].Source = "javascript" },
		"id":                                  func(v *ProfileViewDefinition) { v.Blocks[0].ID = "../outside" },
		"duplicate":                           func(v *ProfileViewDefinition) { v.Blocks = append(v.Blocks, v.Blocks[0]) },
		"duplicate source":                    func(v *ProfileViewDefinition) { b := v.Blocks[0]; b.ID = "another"; v.Blocks = append(v.Blocks, b) },
		"blank block":                         func(v *ProfileViewDefinition) { v.Blocks[0].Title = " " },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) { v := testView(); change(&v); require.Error(t, v.Validate()) })
	}
	var empty *ProfileViewDefinition
	require.Error(t, empty.Validate())
}
func TestCompileProfileViewBindings(t *testing.T) {
	v := testView()
	v.Title = " Overview "
	v.Blocks = append(v.Blocks, ProfileViewBlock{ID: "summary", Source: "totalDeliveries", Title: " Count "})
	normalized, requests, err := CompileProfileView(v)
	require.NoError(t, err)
	require.Equal(t, "Overview", normalized.Title)
	require.Equal(t, "Count", normalized.Blocks[1].Title)
	require.Equal(t, " Count ", v.Blocks[1].Title)
	require.Equal(t, "getDeliveryHealthMetrics", requests[0].Operation)
	require.Equal(t, map[string]string{"refKey": "$entity.refKey", "type": "$profileType.name", "range": "30d"}, requests[0].Parameters)
	require.Equal(t, "metrics.totalDeliveries", requests[1].Field)
	require.NotContains(t, requests[1].Parameters, "range")
}

func TestCompileProfileViewChart(t *testing.T) {
	v := testView()
	v.Range = "90d"
	v.Blocks[0].Source = "deliveryHealthChart"
	_, requests, err := CompileProfileView(v)
	require.NoError(t, err)
	require.Equal(t, "getDeliveryHealthMetrics", requests[0].Operation)
	require.Equal(t, "deliveryHealth", requests[0].Field)
	require.Equal(t, map[string]string{"refKey": "$entity.refKey", "type": "$profileType.name", "range": "90d"}, requests[0].Parameters)
}

func TestProfileViewSupportsMetricDefaultsBeyondTwelveBlocks(t *testing.T) {
	v := testView()
	for i := 0; i < 50; i++ {
		v.Blocks = append(v.Blocks, ProfileViewBlock{ID: fmt.Sprintf("metric_%d", i), Source: "configuredMetric", Title: "Metric", MetricID: fmt.Sprintf("11111111-1111-4111-8111-%012d", i), Display: "card"})
	}
	_, requests, err := CompileProfileView(v)
	require.NoError(t, err)
	require.Len(t, requests, 51)
	require.Equal(t, "$entity.refKey", requests[50].Parameters["refKey"])
}
