package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// InvitationConfig represents configuration for invitation system
type InvitationConfig struct {
	DefaultPermissions string   `mapstructure:"default_permissions"`
	DefaultExpiry      string   `mapstructure:"default_expiry"`
	MaxExpiry          string   `mapstructure:"max_expiry"`
	AllowedRoles       []string `mapstructure:"allowed_roles"`
}

// GetDefaultExpiryDuration returns default expiry as time.Duration
func (c *InvitationConfig) GetDefaultExpiryDuration() (time.Duration, error) {
	service := &InvitationTokenService{secretKey: "dummy"}
	return service.ParseExpiry(c.DefaultExpiry)
}

// GetMaxExpiryDuration returns max expiry as time.Duration
func (c *InvitationConfig) GetMaxExpiryDuration() (time.Duration, error) {
	service := &InvitationTokenService{secretKey: "dummy"}
	return service.ParseExpiry(c.MaxExpiry)
}

// IsRoleAllowed checks if role is in allowed list
func (c *InvitationConfig) IsRoleAllowed(role string) bool {
	for _, allowed := range c.AllowedRoles {
		if role == allowed {
			return true
		}
	}
	return false
}

// ThreadInvitationClaims represents JWT claims for thread invitations
type ThreadInvitationClaims struct {
	ThreadID    string `json:"threadId"`
	ContractID  string `json:"contractId"`
	Role        string `json:"role"`
	Permissions string `json:"permissions"`
	InvitedBy   string `json:"invitedBy"`
	jwt.RegisteredClaims
}

// InvitationTokenService handles JWT token creation and validation for invitations
type InvitationTokenService struct {
	secretKey string
	issuer    string
}

// NewInvitationTokenService creates a new invitation token service
func NewInvitationTokenService(secretKey string) *InvitationTokenService {
	return &InvitationTokenService{
		secretKey: secretKey,
		issuer:    "threadify-engine",
	}
}

// CreateToken creates a JWT token for thread invitation
func (s *InvitationTokenService) CreateToken(threadID, contractID, userID, role, permissions string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := &ThreadInvitationClaims{
		ThreadID:    threadID,
		ContractID:  contractID,
		Role:        role,
		Permissions: permissions,
		InvitedBy:   userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			Subject:   "thread-invitation",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secretKey))
}

// ValidateToken validates a JWT token and returns claims
func (s *InvitationTokenService) ValidateToken(tokenString string) (*ThreadInvitationClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &ThreadInvitationClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.secretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*ThreadInvitationClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// ParseExpiry parses expiry string in format "24h", "2d", "30m"
func (s *InvitationTokenService) ParseExpiry(expiresIn string) (time.Duration, error) {
	if expiresIn == "" {
		return 24 * time.Hour, nil // Default 24 hours
	}

	if len(expiresIn) < 2 {
		return 0, fmt.Errorf("invalid expiry format: %s", expiresIn)
	}

	unit := expiresIn[len(expiresIn)-1:]
	value, err := strconv.Atoi(expiresIn[:len(expiresIn)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid expiry value: %s", expiresIn)
	}

	switch unit {
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid expiry unit: %s", unit)
	}
}

// ValidateRole checks if the role is allowed using configuration
func (s *InvitationTokenService) ValidateRole(role string, config *InvitationConfig) error {
	if config != nil && !config.IsRoleAllowed(role) {
		return fmt.Errorf("invalid role: %s. Allowed roles: %s", role, strings.Join(config.AllowedRoles, ", "))
	}

	// Static role validation removed - roles should be validated against contract parties in the handler
	return nil
}

// ValidatePermissions checks if permissions are valid
func (s *InvitationTokenService) ValidatePermissions(permissions string) error {
	if permissions == "" {
		return nil // Empty permissions are allowed (will use default)
	}

	allowedPerms := []string{"read", "write", "execute"}
	permList := strings.Split(permissions, ",")

	for _, perm := range permList {
		perm = strings.TrimSpace(perm)
		valid := false
		for _, allowed := range allowedPerms {
			if perm == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid permission: %s. Allowed permissions: %s", perm, strings.Join(allowedPerms, ", "))
		}
	}

	return nil
}
