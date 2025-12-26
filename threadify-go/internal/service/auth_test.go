package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuthService(t *testing.T) {
	secret := "test-secret"
	issuer := "test-issuer"
	audience := "test-audience"
	expirationHours := 24

	service := NewAuthService(secret, issuer, audience, expirationHours)

	assert.NotNil(t, service)
	assert.Equal(t, []byte(secret), service.secret)
	assert.Equal(t, issuer, service.issuer)
	assert.Equal(t, audience, service.audience)
	assert.Equal(t, 24*time.Hour, service.expiration)
}

func TestAuthService_CreateToken(t *testing.T) {
	service := NewAuthService("test-secret", "test-issuer", "test-audience", 24)

	tests := []struct {
		name   string
		userID string
		claims map[string]interface{}
	}{
		{
			name:   "basic token",
			userID: "user123",
			claims: map[string]interface{}{
				"role": "user",
			},
		},
		{
			name:   "token with multiple claims",
			userID: "user456",
			claims: map[string]interface{}{
				"role":    "admin",
				"ownerId": "owner123",
			},
		},
		{
			name:   "token with no extra claims",
			userID: "user789",
			claims: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := service.CreateToken(tt.userID, tt.claims)
			require.NoError(t, err)
			assert.NotEmpty(t, token)

			// Verify the token can be parsed
			parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
				return service.secret, nil
			})
			require.NoError(t, err)
			assert.True(t, parsedToken.Valid)

			// Verify claims
			claims, ok := parsedToken.Claims.(jwt.MapClaims)
			require.True(t, ok)
			assert.Equal(t, tt.userID, claims["sub"])
			assert.Equal(t, service.issuer, claims["iss"])
			assert.Equal(t, service.audience, claims["aud"])

			// Verify custom claims
			for k, v := range tt.claims {
				assert.Equal(t, v, claims[k])
			}
		})
	}
}

func TestAuthService_VerifyToken(t *testing.T) {
	service := NewAuthService("test-secret", "test-issuer", "test-audience", 24)

	t.Run("valid token", func(t *testing.T) {
		userID := "user123"
		customClaims := map[string]interface{}{
			"role":    "admin",
			"ownerId": "owner123",
		}

		token, err := service.CreateToken(userID, customClaims)
		require.NoError(t, err)

		claims, err := service.VerifyToken(token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims["sub"])
		assert.Equal(t, "admin", claims["role"])
		assert.Equal(t, "owner123", claims["ownerId"])
	})

	t.Run("invalid token - wrong secret", func(t *testing.T) {
		wrongService := NewAuthService("wrong-secret", "test-issuer", "test-audience", 24)
		token, err := wrongService.CreateToken("user123", map[string]interface{}{})
		require.NoError(t, err)

		_, err = service.VerifyToken(token)
		assert.Error(t, err)
	})

	t.Run("invalid token - malformed", func(t *testing.T) {
		_, err := service.VerifyToken("not.a.valid.token")
		assert.Error(t, err)
	})

	t.Run("invalid token - expired", func(t *testing.T) {
		// Create service with 0 hour expiration
		expiredService := NewAuthService("test-secret", "test-issuer", "test-audience", 0)

		// Create token that expires immediately
		now := time.Now()
		jwtClaims := jwt.MapClaims{
			"sub": "user123",
			"iss": expiredService.issuer,
			"aud": expiredService.audience,
			"iat": now.Unix(),
			"exp": now.Add(-1 * time.Hour).Unix(), // Already expired
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
		tokenString, err := token.SignedString(expiredService.secret)
		require.NoError(t, err)

		_, err = expiredService.VerifyToken(tokenString)
		assert.Error(t, err)
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := service.VerifyToken("")
		assert.Error(t, err)
	})
}

func TestAuthService_VerifyToken_SigningMethod(t *testing.T) {
	service := NewAuthService("test-secret", "test-issuer", "test-audience", 24)

	// Create a token with wrong signing method (RS256 instead of HS256)
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": service.issuer,
		"aud": service.audience,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	}

	// This would normally use RSA keys, but we're just testing the method check
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = service.VerifyToken(tokenString)
	assert.Error(t, err)
}

func TestAuthService_ValidateApiKey(t *testing.T) {
	auth := NewAuthService("test-secret", "test-issuer", "test-audience", 24)

	tests := []struct {
		name      string
		apiKey    string
		expectErr bool
		expected  *UserInfo
	}{
		{
			name:   "Valid API key 123",
			apiKey: "api-key-123",
			expected: &UserInfo{
				OwnerID:   "user-123",
				CompanyID: "company-abc",
				Role:      "admin",
			},
		},
		{
			name:   "Valid API key 456",
			apiKey: "api-key-456",
			expected: &UserInfo{
				OwnerID:   "user-456",
				CompanyID: "company-xyz",
				Role:      "user",
			},
		},
		{
			name:   "Valid test API key",
			apiKey: "test-api-key",
			expected: &UserInfo{
				OwnerID:   "test-user",
				CompanyID: "test-company",
				Role:      "developer",
			},
		},
		{
			name:   "Valid demo API key",
			apiKey: "demo-key",
			expected: &UserInfo{
				OwnerID:   "demo-user",
				CompanyID: "demo-company",
				Role:      "user",
			},
		},
		{
			name:      "Invalid API key",
			apiKey:    "invalid-key",
			expectErr: true,
		},
		{
			name:      "Empty API key",
			apiKey:    "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := auth.ValidateApiKey(tt.apiKey)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.OwnerID, result.OwnerID)
				assert.Equal(t, tt.expected.CompanyID, result.CompanyID)
				assert.Equal(t, tt.expected.Role, result.Role)
			}
		})
	}
}
