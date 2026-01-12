package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// UserInfo represents user information derived from an API key
type UserInfo struct {
	OwnerID   string `json:"ownerId"`
	CompanyID string `json:"companyId"`
	Role      string `json:"role"`
}

type AuthService struct {
	secret     []byte
	issuer     string
	audience   string
	expiration time.Duration
}

func NewAuthService(secret, issuer, audience string, expirationHours int) *AuthService {
	return &AuthService{
		secret:     []byte(secret),
		issuer:     issuer,
		audience:   audience,
		expiration: time.Duration(expirationHours) * time.Hour,
	}
}

// ValidateApiKey provides a mock implementation for API key validation
// For demo purposes, it maps specific API keys to fake users
func (s *AuthService) ValidateApiKey(apiKey string) (*UserInfo, error) {
	// Mock API key to user mapping
	mockUsers := map[string]*UserInfo{
		"api-key-123": {
			OwnerID:   "user-123",
			CompanyID: "company-abc",
			Role:      "admin",
		},
		"api-key-456": {
			OwnerID:   "user-456",
			CompanyID: "company-xyz",
			Role:      "user",
		},
		"test-api-key": {
			OwnerID:   "test-user",
			CompanyID: "test-company",
			Role:      "developer",
		},
		"demo-key": {
			OwnerID:   "demo-user",
			CompanyID: "demo-company",
			Role:      "user",
		},
	}

	// Check if API key exists in our mock mapping
	if userInfo, exists := mockUsers[apiKey]; exists {
		return userInfo, nil
	}

	// Return error for unknown API keys
	return nil, fmt.Errorf("invalid API key")
}

func (s *AuthService) CreateToken(userID string, claims map[string]interface{}) (string, error) {
	now := time.Now()
	jwtClaims := jwt.MapClaims{
		"sub": userID,
		"iss": s.issuer,
		"aud": s.audience,
		"iat": now.Unix(),
		"exp": now.Add(s.expiration).Unix(),
	}

	for k, v := range claims {
		jwtClaims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString(s.secret)
}

func (s *AuthService) VerifyToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("invalid token")
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
