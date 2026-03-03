package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	sharedauth "threadify-go/shared/auth"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/workerpool"
)

const defaultCacheTTL = 1 * time.Hour

type UserInfo struct {
	OwnerID   string `json:"ownerId"`
	CompanyID string `json:"companyId"`
	Role      string `json:"role"`
}

// cachedUserInfo stores UserInfo with expiration time.
type cachedUserInfo struct {
	userInfo  *UserInfo
	expiresAt time.Time
}

type AuthService struct {
	db            *pgxpool.Pool
	cache         sync.Map // key: apiKeyHash -> *cachedUserInfo
	cacheTTL      time.Duration
	writeBackPool *workerpool.Pool
	jwksVerifier  *sharedauth.JWKSVerifier
	stopCleanup   chan struct{}
}

func NewAuthService() *AuthService {
	s := &AuthService{
		cacheTTL:    defaultCacheTTL,
		stopCleanup: make(chan struct{}),
	}
	go s.cleanupExpiredCache()
	return s
}

// Stop shuts down the background cache cleanup goroutine.
func (s *AuthService) Stop() {
	close(s.stopCleanup)
}

func (s *AuthService) SetJWKSVerifier(v *sharedauth.JWKSVerifier) {
	s.jwksVerifier = v
}

// SetDB sets the database connection for API key validation.
func (s *AuthService) SetDB(db *pgxpool.Pool) {
	s.db = db
}

// SetWriteBackPool sets the worker pool for async last_used_at updates.
func (s *AuthService) SetWriteBackPool(pool *workerpool.Pool) {
	s.writeBackPool = pool
}

func (s *AuthService) cleanupExpiredCache() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.cache.Range(func(key, value interface{}) bool {
				if cached, ok := value.(*cachedUserInfo); ok && now.After(cached.expiresAt) {
					s.cache.Delete(key)
				}
				return true
			})
		case <-s.stopCleanup:
			return
		}
	}
}

// ValidateApiKey validates an API key token using a cache-aside pattern.
// Cache hit → no DB query; cache miss or expired → query DB, warm cache.
func (s *AuthService) ValidateApiKey(apiKey string) (*UserInfo, error) {
	if s.db == nil {
		return nil, ErrDatabaseNotConfigured
	}

	keyHash := hashAPIKey(apiKey)

	if cached, ok := s.cache.Load(keyHash); ok {
		if cachedInfo, ok := cached.(*cachedUserInfo); ok {
			if time.Now().Before(cachedInfo.expiresAt) {
				metrics.APIKeyCacheHits.Inc()
				return cachedInfo.userInfo, nil
			}
			s.cache.Delete(keyHash)
		}
	}

	metrics.APIKeyCacheMisses.Inc()
	userInfo, err := s.validateApiKeyFromDB(keyHash)
	if err != nil {
		return nil, err
	}

	s.cache.Store(keyHash, &cachedUserInfo{
		userInfo:  userInfo,
		expiresAt: time.Now().Add(s.cacheTTL),
	})
	return userInfo, nil
}

func (s *AuthService) validateApiKeyFromDB(keyHash string) (*UserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		return nil, ErrApiKeyInvalidOrInactive
	}

	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, ErrApiKeyExpired
	}

	if s.writeBackPool != nil {
		s.writeBackPool.Submit(func(ctx context.Context) {
			updateCtx, updateCancel := context.WithTimeout(ctx, 3*time.Second)
			defer updateCancel()
			s.db.Exec(updateCtx, `UPDATE api_keys SET last_used_at = NOW() WHERE key_hash = $1`, keyHash)
		})
	}

	return &userInfo, nil
}

func (s *AuthService) VerifyToken(ctx context.Context, tokenString string) (*sharedauth.TokenClaims, error) {
	if s.jwksVerifier == nil {
		return nil, ErrJwtVerificationNotConfigured
	}
	return s.jwksVerifier.Verify(ctx, tokenString)
}

func (s *AuthService) GetUserRoles(ctx context.Context, principalID string, principalType string) ([]string, error) {
	if s.db == nil {
		return nil, ErrDatabaseNotConfigured
	}

	rows, err := s.db.Query(ctx, `
		SELECT role_name
		FROM user_roles
		WHERE principal_id = $1 AND principal_type = $2
	`, principalID, principalType)
	if err != nil {
		return nil, fmt.Errorf("failed to query user roles: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return nil, fmt.Errorf("failed to scan role name: %w", err)
		}
		roles = append(roles, roleName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row error fetching user roles: %w", err)
	}

	return roles, nil
}

// hashAPIKey returns the hex-encoded SHA-256 hash of an API key.
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
