package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThreadInvitationClaims_Creation(t *testing.T) {
	// Test that invitation claims can be created with all required fields
	claims := &ThreadInvitationClaims{
		ThreadID:    "thread-123",
		ContractID:  "contract-456",
		Role:        "external_partner",
		Permissions: "read,write",
		InvitedBy:   "user-789",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "threadify-engine",
			Subject:   "invitation-token",
		},
	}

	// Verify all fields are set correctly
	assert.Equal(t, "thread-123", claims.ThreadID)
	assert.Equal(t, "contract-456", claims.ContractID)
	assert.Equal(t, "external_partner", claims.Role)
	assert.Equal(t, "read,write", claims.Permissions)
	assert.Equal(t, "user-789", claims.InvitedBy)
	assert.True(t, claims.ExpiresAt.After(time.Now()))
}

func TestInvitationTokenService_CreateToken(t *testing.T) {
	// Test token creation with valid parameters
	service := NewInvitationTokenService("test-secret")

	token, err := service.CreateToken("thread-123", "contract-456", "user-789", "external_partner", "read,write", 24*time.Hour)

	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Verify token can be parsed
	parsedClaims, err := service.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "thread-123", parsedClaims.ThreadID)
	assert.Equal(t, "contract-456", parsedClaims.ContractID)
	assert.Equal(t, "external_partner", parsedClaims.Role)
	assert.Equal(t, "read,write", parsedClaims.Permissions)
}

func TestInvitationTokenService_ValidateToken(t *testing.T) {
	service := NewInvitationTokenService("test-secret")

	// Test valid token
	token, _ := service.CreateToken("thread-123", "contract-456", "user-789", "external_partner", "read,write", 24*time.Hour)

	claims, err := service.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "thread-123", claims.ThreadID)

	// Test invalid token
	_, err = service.ValidateToken("invalid-token")
	assert.Error(t, err)

	// Test expired token
	expiredToken, _ := service.CreateToken("thread-123", "contract-456", "user-789", "external_partner", "read,write", -1*time.Hour)
	_, err = service.ValidateToken(expiredToken)
	assert.Error(t, err)
}

func TestInvitationTokenService_ExpiryParsing(t *testing.T) {
	service := NewInvitationTokenService("test-secret")

	testCases := []struct {
		input    string
		expected time.Duration
	}{
		{"1h", 1 * time.Hour},
		{"24h", 24 * time.Hour},
		{"2d", 48 * time.Hour},
		{"30m", 30 * time.Minute},
		{"", 24 * time.Hour}, // default
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			duration, err := service.ParseExpiry(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, duration)
		})
	}

	// Test invalid format
	_, err := service.ParseExpiry("invalid")
	assert.Error(t, err)
}
