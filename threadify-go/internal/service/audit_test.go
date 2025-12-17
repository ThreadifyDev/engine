package service

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditQueueConfigurationLoading(t *testing.T) {
	// Set up test configuration
	viper.Reset()
	viper.SetConfigType("yaml")

	testConfig := `
audit_queue:
  name: "test_audit_events"
  retention_hours: 12
  batch_size: 50
  batch_timeout_ms: 5000
  max_retries: 3
`

	err := viper.ReadConfig(bytes.NewBufferString(testConfig))
	require.NoError(t, err)

	// Test that audit queue config loads correctly
	var auditConfig AuditQueueConfig
	err = viper.UnmarshalKey("audit_queue", &auditConfig)
	require.NoError(t, err)

	// Verify loaded values
	assert.Equal(t, "test_audit_events", auditConfig.Name)
	assert.Equal(t, 12, auditConfig.RetentionHours)
	assert.Equal(t, 50, auditConfig.BatchSize)
	assert.Equal(t, 5000, auditConfig.BatchTimeoutMs)
	assert.Equal(t, 3, auditConfig.MaxRetries)
}

func TestAuditQueueConfigurationDefaults(t *testing.T) {
	// Test default values when config is missing
	viper.Reset()
	viper.SetConfigType("yaml")

	testConfig := `{}`
	err := viper.ReadConfig(bytes.NewBufferString(testConfig))
	require.NoError(t, err)

	// Load with defaults
	var auditConfig AuditQueueConfig
	err = viper.UnmarshalKey("audit_queue", &auditConfig)
	require.NoError(t, err)

	// Should have empty/default values
	assert.Equal(t, "", auditConfig.Name)
	assert.Equal(t, 0, auditConfig.RetentionHours)
	assert.Equal(t, 0, auditConfig.BatchSize)
}

func TestAuditQueueConfigurationValidation(t *testing.T) {
	// Test validation of required fields
	viper.Reset()
	viper.SetConfigType("yaml")

	testConfig := `
audit_queue:
  name: ""
  retention_hours: -1
  batch_size: 0
`

	err := viper.ReadConfig(bytes.NewBufferString(testConfig))
	require.NoError(t, err)

	var auditConfig AuditQueueConfig
	err = viper.UnmarshalKey("audit_queue", &auditConfig)
	require.NoError(t, err)

	// Validation should fail for invalid values
	assert.False(t, auditConfig.IsValid())
}
