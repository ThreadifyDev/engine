package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"slices"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/threadify/engine/internal/models"
)

// allowedAccessLevels is the fixed set of valid access levels.
var allowedAccessLevels = []string{"owner", "participant", "observer", "external"}

// InvitationConfig represents configuration for the invitation system.
type InvitationConfig struct {
	DefaultPermissions string   `mapstructure:"default_permissions"`
	DefaultExpiry      string   `mapstructure:"default_expiry"`
	MaxExpiry          string   `mapstructure:"max_expiry"`
	AllowedRoles       []string `mapstructure:"allowed_roles"`
}

// GetDefaultExpiryDuration returns the default expiry as a time.Duration.
func (c *InvitationConfig) GetDefaultExpiryDuration() (time.Duration, error) {
	return parseExpiry(c.DefaultExpiry)
}

// GetMaxExpiryDuration returns the max expiry as a time.Duration.
func (c *InvitationConfig) GetMaxExpiryDuration() (time.Duration, error) {
	return parseExpiry(c.MaxExpiry)
}

// IsRoleAllowed reports whether role is in the allowed list.
func (c *InvitationConfig) IsRoleAllowed(role string) bool {
	return slices.Contains(c.AllowedRoles, role)
}

// InvitationTokenService handles JWT token creation and validation for invitations.
type InvitationTokenService struct {
	secretKey string
	issuer    string
}

// NewInvitationTokenService creates a new invitation token service.
func NewInvitationTokenService(secretKey, issuer string) *InvitationTokenService {
	return &InvitationTokenService{
		secretKey: secretKey,
		issuer:    issuer,
	}
}

// CreateToken creates a signed JWT for a thread invitation.
func (s *InvitationTokenService) CreateToken(threadID, userID, role, accessLevel string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := &models.ThreadInvitationClaims{
		ThreadID:    threadID,
		Role:        role,
		AccessLevel: accessLevel,
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

// ValidateToken validates a JWT and returns its claims.
func (s *InvitationTokenService) ValidateToken(tokenString string) (*models.ThreadInvitationClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &models.ThreadInvitationClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(s.secretKey), nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*models.ThreadInvitationClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ValidateRole checks if role is permitted by the given config.
func (s *InvitationTokenService) ValidateRole(role string, config *InvitationConfig) error {
	if config != nil && !config.IsRoleAllowed(role) {
		return fmt.Errorf("invalid role %q, allowed: %s", role, strings.Join(config.AllowedRoles, ", "))
	}
	return nil
}

// ValidateAccessLevel checks if accessLevel is one of the known values.
// An empty string is allowed and will default to "external" at the call site.
func (s *InvitationTokenService) ValidateAccessLevel(accessLevel string) error {
	if accessLevel == "" {
		return nil
	}
	if !slices.Contains(allowedAccessLevels, accessLevel) {
		return fmt.Errorf("invalid access level %q, allowed: %s", accessLevel, strings.Join(allowedAccessLevels, ", "))
	}
	return nil
}

// ParseExpiry parses an expiry string in the format "30m", "24h", or "7d".
// Defaults to 24h if the string is empty.
func (s *InvitationTokenService) ParseExpiry(expiresIn string) (time.Duration, error) {
	return parseExpiry(expiresIn)
}

func parseExpiry(expiresIn string) (time.Duration, error) {
	if expiresIn == "" {
		return 24 * time.Hour, nil
	}
	if len(expiresIn) < 2 {
		return 0, fmt.Errorf("invalid expiry format: %q", expiresIn)
	}

	unit := expiresIn[len(expiresIn)-1:]
	value, err := strconv.Atoi(expiresIn[:len(expiresIn)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid expiry value: %q", expiresIn)
	}

	switch unit {
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid expiry unit %q, use m, h, or d", unit)
	}
}
