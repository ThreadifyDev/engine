package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/metrics"
)

// UserInfo represents user information derived from an API key
type UserInfo struct {
	OwnerID   string `json:"ownerId"`
	CompanyID string `json:"companyId"`
	Role      string `json:"role"`
}

// cachedUserInfo stores UserInfo with expiration time
type cachedUserInfo struct {
	userInfo  *UserInfo
	expiresAt time.Time
}

type AuthService struct {
	secret     []byte
	issuer     string
	audience   string
	expiration time.Duration
	db         *pgxpool.Pool
	cache      sync.Map // key: apiKeyHash -> cachedUserInfo
	cacheTTL   time.Duration
}

func NewAuthService(secret, issuer, audience string, expirationHours int) *AuthService {
	return &AuthService{
		secret:     []byte(secret),
		issuer:     issuer,
		audience:   audience,
		expiration: time.Duration(expirationHours) * time.Hour,
	}
}

// SetDB sets the database connection for API key validation
func (s *AuthService) SetDB(db *pgxpool.Pool) {
	s.db = db
	s.cacheTTL = 1 * time.Hour // Cache API key lookups for 1 hour

	// Start cache cleanup goroutine
	go s.cleanupExpiredCache()
}

// cleanupExpiredCache periodically removes expired entries from cache
func (s *AuthService) cleanupExpiredCache() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.cache.Range(func(key, value interface{}) bool {
			if cached, ok := value.(*cachedUserInfo); ok {
				if now.After(cached.expiresAt) {
					s.cache.Delete(key)
				}
			}
			return true
		})
	}
}

// ValidateApiKey validates an API key using cache-aside pattern
// 1. Check cache first
// 2. If miss, query database
// 3. Store result in cache
func (s *AuthService) ValidateApiKey(apiKey string) (*UserInfo, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Hash the API key for cache lookup
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	// Check cache first
	if cached, ok := s.cache.Load(keyHash); ok {
		if cachedInfo, ok := cached.(*cachedUserInfo); ok {
			// Check if cache entry is still valid
			if time.Now().Before(cachedInfo.expiresAt) {
				metrics.APIKeyCacheHits.Inc()
				return cachedInfo.userInfo, nil
			}
			// Cache expired, remove it
			s.cache.Delete(keyHash)
		}
	}

	// Cache miss or expired - query database
	metrics.APIKeyCacheMisses.Inc()
	userInfo, err := s.validateApiKeyFromDB(apiKey)
	if err != nil {
		return nil, err
	}

	// Store in cache
	s.cache.Store(keyHash, &cachedUserInfo{
		userInfo:  userInfo,
		expiresAt: time.Now().Add(s.cacheTTL),
	})

	return userInfo, nil
}

// validateApiKeyFromDB validates API key against the database
// This is called only on cache miss - results are cached by ValidateApiKey
func (s *AuthService) validateApiKeyFromDB(apiKey string) (*UserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Hash the API key (assuming keys are stored hashed)
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	// Query to get service account info from API key
	// Role is fetched from user_roles table (service accounts use principal_id)
	query := `
		SELECT 
			sa.id as owner_id,
			sa.company_id,
			COALESCE(ur.role_name, 'standard_service') as role,
			ak.is_active,
			ak.expires_at
		FROM api_keys ak
		JOIN service_accounts sa ON ak.service_account_id = sa.id
		LEFT JOIN user_roles ur ON ur.principal_id = sa.id AND ur.principal_type = 'service_account'
		WHERE ak.key_hash = $1
		AND ak.is_active = true
		AND sa.is_active = true
		LIMIT 1
	`

	var userInfo UserInfo
	var expiresAt *time.Time
	var isActive bool

	err := s.db.QueryRow(ctx, query, keyHash).Scan(
		&userInfo.OwnerID,
		&userInfo.CompanyID,
		&userInfo.Role,
		&isActive,
		&expiresAt,
	)

	if err != nil {
		return nil, fmt.Errorf("API key not found or inactive: %w", err)
	}

	// Check if key is expired
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Update last_used_at timestamp (async, don't wait)
	go func() {
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer updateCancel()

		updateQuery := `UPDATE api_keys SET last_used_at = NOW() WHERE key_hash = $1`
		s.db.Exec(updateCtx, updateQuery, keyHash)
	}()

	return &userInfo, nil
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
