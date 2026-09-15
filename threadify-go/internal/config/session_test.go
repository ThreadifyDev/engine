package config

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestLoadSessionProviderEnvironment(t *testing.T) {
	t.Setenv("ENGINE_TEST_SUPABASE_URL", "https://auth.example.invalid")
	t.Setenv("ENGINE_TEST_SUPABASE_KEY", "public-test-key")
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader("supabase:\n  url: '$ENGINE_TEST_SUPABASE_URL:'\n  publishable_key: '$ENGINE_TEST_SUPABASE_KEY:'\n  request_timeout_seconds: 3\n")))
	c, err := LoadFromViper(v)
	require.NoError(t, err)
	require.Equal(t, "https://auth.example.invalid", c.Supabase.URL)
	require.Equal(t, "public-test-key", c.Supabase.PublishableKey)
	require.Equal(t, 3, c.Supabase.RequestTimeoutSeconds)
}
