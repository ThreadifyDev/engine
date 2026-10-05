package ingestion

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNameFiltersAndPreview(t *testing.T) {
	rules, err := Normalize([]string{" healthcheck ", "internal.*", "healthcheck", "调用*"})
	require.NoError(t, err)
	require.Equal(t, []string{"healthcheck", "internal.*", "调用*"}, rules)
	result, err := Preview(rules, []string{"healthcheck", "Healthcheck", "internal.cache", "internal", "调用工具", "refund"})
	require.NoError(t, err)
	require.Equal(t, 3, result.Dropped)
	require.Equal(t, 3, result.Kept)
	require.False(t, result.Spans[2].Drop)
	require.True(t, result.Spans[5].Drop)
	require.Equal(t, "internal.*", result.Spans[2].Pattern)
	require.Equal(t, "", Match([]string{"refund.[0-9]"}, "refund.5")) // Literal, not regex.
	require.Equal(t, "*", Match([]string{"*"}, "anything"))
	require.False(t, ShouldDrop(ModeInclude, []string{"*"}, "anything"))
	require.True(t, ShouldDrop(ModeInclude, nil, "anything"))
	require.True(t, ShouldDrop(ModeExcludeLegacy, []string{"healthcheck"}, "healthcheck"))
	require.False(t, ShouldDrop(ModeExcludeLegacy, []string{"healthcheck"}, "refund"))
	empty, err := Normalize(nil)
	require.NoError(t, err)
	require.NotNil(t, empty)
}
func TestInvalidIngestionRules(t *testing.T) {
	for _, p := range []string{"", "  ", "in*ternal", "a**", "bad\x00name", strings.Repeat("a", 257)} {
		_, err := Normalize([]string{p})
		require.Error(t, err, p)
	}
	_, err := Normalize(make([]string, 101))
	require.Error(t, err)
	_, err = Preview(nil, make([]string, 101))
	require.Error(t, err)
	_, err = Preview(nil, []string{strings.Repeat("a", 4097)})
	require.Error(t, err)
}

func TestKeepAndDropSpanPatterns(t *testing.T) {
	settings := Settings{Mode: ModeInclude, Filters: []string{"POST *", "checkout*"}, Exclude: []string{"POST /graphql*", "checkout.health"}}
	for _, tc := range []struct {
		name, dropPattern string
		drop              bool
	}{
		{"POST /orders", "", false},
		{"POST /graphql", "POST /graphql*", true},
		{"POST /graphql/admin", "POST /graphql*", true},
		{"checkout.health", "checkout.health", true},
		{"checkout.healthcheck", "", false},
		{"GET /orders", "", true},
		{"POST /GraphQL", "", false},
	} {
		result := Evaluate(settings, tc.name)
		require.Equal(t, tc.drop, result.Drop, tc.name)
		require.Equal(t, tc.dropPattern, result.DropPattern, tc.name)
	}
	settings.Filters = []string{"*"}
	settings.Exclude = []string{"*"}
	require.True(t, Evaluate(settings, "anything").Drop)
	settings.Mode = ModeExcludeLegacy
	settings.Filters = []string{"health*"}
	settings.Exclude = nil
	require.True(t, Evaluate(settings, "healthcheck").Drop)
	require.False(t, Evaluate(settings, "checkout").Drop)
}

func TestDropPreviewUsesSameRules(t *testing.T) {
	result, err := PreviewRules([]string{"*"}, []string{"POST /graphql*"}, []string{"POST /graphql", "POST /orders"})
	require.NoError(t, err)
	require.Equal(t, 1, result.Dropped)
	require.Equal(t, 1, result.Kept)
	require.Equal(t, "POST /graphql*", result.Spans[0].DropPattern)
	_, err = PreviewRules([]string{"*"}, []string{"bad*pattern"}, nil)
	require.Error(t, err)
}

func TestRegexKeepAndDrop(t *testing.T) {
	settings := Settings{Mode: ModeInclude, Filters: []string{`regex:(?i)^(GET|POST) /`, "checkout*"}, Exclude: []string{`regex:(?i)^POST /graphql`, `regex:(?i)health|heartbeat`}}
	matcher, err := Compile(settings)
	require.NoError(t, err)
	names := []string{"post /GraphQL", "GET /orders", "POST /orders/HEALTH", "checkout.completed", "Checkout.completed", "prefix GET /orders"}
	expected := []bool{true, false, true, false, true, true}
	preview, err := PreviewRules(settings.Filters, settings.Exclude, names)
	require.NoError(t, err)
	for i, name := range names {
		decision := matcher.Evaluate(name)
		require.Equal(t, expected[i], decision.Drop, name)
		require.Equal(t, decision, preview.Spans[i], "preview must match ingestion")
	}
	require.Equal(t, `regex:(?i)^POST /graphql`, preview.Spans[0].DropPattern)
	require.Equal(t, `regex:(?i)^(GET|POST) /`, preview.Spans[1].Pattern)
	require.Equal(t, 4, preview.Dropped)
	require.Equal(t, 2, preview.Kept)
	require.Equal(t, `regex:(?i)health|heartbeat`, Match(settings.Exclude, "background.HEARTBEAT"))
	require.Equal(t, `regex:^调用\d+$`, Match([]string{`regex:^调用\d+$`}, "调用42"))
	require.Empty(t, Match([]string{`regex:^调用\d+$`}, "调用42extra"))
	// Regex metacharacters in plain patterns remain literal.
	require.Equal(t, "health.[0-9]", Match([]string{"health.[0-9]"}, "health.[0-9]"))
	require.Empty(t, Match([]string{"health.[0-9]"}, "health.2"))
	require.Empty(t, Match([]string{"health*"}, "HEALTH"))
	legacy, err := Compile(Settings{Mode: ModeExcludeLegacy, Filters: []string{`regex:(?i)health`}})
	require.NoError(t, err)
	require.True(t, legacy.Evaluate("HEALTH").Drop)
	require.False(t, legacy.Evaluate("checkout").Drop)
}

func TestInvalidRegexRejectedInBothSections(t *testing.T) {
	for _, value := range []string{"regex:", "regex:[", "regex:(?=health)", `regex:(a)\1`, "regex:(?z)health", "regex:" + strings.Repeat("a", 251)} {
		_, err := Normalize([]string{value})
		require.Error(t, err, value)
		for _, settings := range []Settings{{Filters: []string{value}}, {Filters: []string{"*"}, Exclude: []string{value}}} {
			_, err := Compile(settings)
			require.Error(t, err, value)
			_, err = PreviewRules(settings.Filters, settings.Exclude, []string{"health"})
			require.Error(t, err, value)
		}
	}
	normalized, err := Normalize([]string{` regex:(?i)^POST /graphql `, `regex:(?i)^POST /graphql`})
	require.NoError(t, err)
	require.Equal(t, []string{`regex:(?i)^POST /graphql`}, normalized)
}
