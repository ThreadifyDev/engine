package service

import (
	"testing"
	"time"

	"bytes"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvitationConfigurationLoading(t *testing.T) {
	// Set up test configuration
	viper.Reset()
	viper.SetConfigType("yaml")

	testConfig := `
invitations:
  default_permissions: "read,write"
  default_expiry: "24h"
  max_expiry: "7d"
  allowed_roles:
    - "external_partner"
    - "contractor"
    - "auditor"
    - "support"
`

	err := viper.ReadConfig(bytes.NewBufferString(testConfig))
	require.NoError(t, err)

	// Test that invitation config loads correctly
	var invitationConfig InvitationConfig
	err = viper.UnmarshalKey("invitations", &invitationConfig)
	require.NoError(t, err)

	// Verify loaded values
	assert.Equal(t, "read,write", invitationConfig.DefaultPermissions)
	assert.Equal(t, "24h", invitationConfig.DefaultExpiry)
	assert.Equal(t, "7d", invitationConfig.MaxExpiry)
	assert.Len(t, invitationConfig.AllowedRoles, 4)
	assert.Contains(t, invitationConfig.AllowedRoles, "external_partner")
}

func TestInvitationConfig_HelperMethods(t *testing.T) {
	config := &InvitationConfig{
		DefaultPermissions: "read,write",
		DefaultExpiry:      "24h",
		MaxExpiry:          "7d",
		AllowedRoles: []string{
			"external_partner",
			"contractor",
		},
	}

	// Test GetDefaultExpiryDuration
	defaultExpiry, err := config.GetDefaultExpiryDuration()
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, defaultExpiry)

	// Test GetMaxExpiryDuration
	maxExpiry, err := config.GetMaxExpiryDuration()
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, maxExpiry)

	// Test IsRoleAllowed
	assert.True(t, config.IsRoleAllowed("external_partner"))
	assert.True(t, config.IsRoleAllowed("contractor"))
	assert.False(t, config.IsRoleAllowed("invalid_role"))
}

func TestInvitationTokenService_ConfigurationValidation(t *testing.T) {
	service := NewInvitationTokenService("test-secret")

	config := &InvitationConfig{
		AllowedRoles: []string{"external_partner", "contractor"},
	}

	// Test valid role
	err := service.ValidateRole("external_partner", config)
	assert.NoError(t, err)

	// Test invalid role
	err = service.ValidateRole("invalid_role", config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid role")

	// Test without config (fallback)
	err = service.ValidateRole("external_partner", nil)
	assert.NoError(t, err)
}
