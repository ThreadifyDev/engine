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
	require.Equal(t, "internal.*", result.Spans[2].Pattern)
	require.Equal(t, "", Match([]string{"refund.[0-9]"}, "refund.5")) // Literal, not regex.
	require.Equal(t, "*", Match([]string{"*"}, "anything"))
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
