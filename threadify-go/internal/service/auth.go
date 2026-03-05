package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	sharedauth "threadify-go/shared/auth"

	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/workerpool"
)

const defaultCacheTTL = 3600 // 1 hour in seconds

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

// cachedRoles stores role names with expiration time.
type cachedRoles struct {
	roles     []string
	expiresAt time.Time
}

type AuthService struct {
	authRepo      *postgres.AuthRepository
	cache         sync.Map // key: apiKeyHash   → *cachedUserInfo
	rolesCache    sync.Map // key: userID:type   → *cachedRoles
	cacheTTL      time.Duration
	writeBackPool *workerpool.Pool
	jwksVerifier  *sharedauth.JWKSVerifier
	stopCleanup   chan struct{}
}

func NewAuthService(authRepo *postgres.AuthRepository, cacheTTLSeconds int) *AuthService {
	ttl := time.Duration(cacheTTLSeconds) * time.Second
	if cacheTTLSeconds <= 0 {
		ttl = time.Duration(defaultCacheTTL) * time.Second
	}
	s := &AuthService{
		authRepo:    authRepo,
		cacheTTL:    ttl,
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
			s.rolesCache.Range(func(key, value interface{}) bool {
				if cached, ok := value.(*cachedRoles); ok && now.After(cached.expiresAt) {
					s.rolesCache.Delete(key)
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
	if s.authRepo == nil {
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

	info, err := s.authRepo.ValidateAPIKey(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, ErrApiKeyInvalidOrInactive
	}

	if info.ExpiresAt != nil && info.ExpiresAt.Before(time.Now()) {
		return nil, ErrApiKeyExpired
	}

	// TODO: Consider moving last_used_at tracking to NATS for async processing
	// Currently commented out as this data is not actively used
	// if s.writeBackPool != nil {
	// 	s.writeBackPool.Submit(func(ctx context.Context) {
	// 		updateCtx, updateCancel := context.WithTimeout(ctx, 3*time.Second)
	// 		defer updateCancel()
	// 		s.authRepo.UpdateLastUsed(updateCtx, keyHash)
	// 	})
	// }

	return &UserInfo{
		OwnerID:   info.OwnerID,
		CompanyID: info.CompanyID,
		Role:      info.Role,
	}, nil
}

func (s *AuthService) VerifyToken(ctx context.Context, tokenString string) (*sharedauth.TokenClaims, error) {
	if s.jwksVerifier == nil {
		return nil, ErrJwtVerificationNotConfigured
	}
	return s.jwksVerifier.Verify(ctx, tokenString)
}

func (s *AuthService) GetUserRoles(ctx context.Context, principalID string, principalType string, jwtExpiry time.Time) ([]string, error) {
	if s.authRepo == nil {
		return nil, ErrDatabaseNotConfigured
	}

	cacheKey := principalID + ":" + principalType

	// Fast path: serve from cache
	if cached, ok := s.rolesCache.Load(cacheKey); ok {
		if cr, ok := cached.(*cachedRoles); ok && time.Now().Before(cr.expiresAt) {
			return cr.roles, nil
		}
		s.rolesCache.Delete(cacheKey)
	}

	// Slow path: query DB via repository
	roles, err := s.authRepo.GetUserRoles(ctx, principalID, principalType)
	if err != nil {
		return nil, err
	}

	// Cap TTL to min(jwtExpiry, defaultCacheTTL) so roles never outlive the
	// token that granted access. If jwtExpiry is zero (not provided), fall
	// back to the default TTL.
	ttl := s.cacheTTL
	if !jwtExpiry.IsZero() {
		if remaining := time.Until(jwtExpiry); remaining > 0 && remaining < ttl {
			ttl = remaining
		}
	}

	s.rolesCache.Store(cacheKey, &cachedRoles{
		roles:     roles,
		expiresAt: time.Now().Add(ttl),
	})

	return roles, nil
}

// hashAPIKey returns the hex-encoded SHA-256 hash of an API key.
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
