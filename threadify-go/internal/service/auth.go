package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	sharedauth "threadify-go/shared/auth"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/workerpool"
	"golang.org/x/sync/singleflight"
)

const (
	defaultCacheTTL = 3600   // 1 hour in seconds
	maxCacheEntries = 100000 // Cap to prevent unbounded memory growth
)

// cachedUserInfo stores UserInfo with expiration time.
type cachedUserInfo struct {
	userInfo  *interfaces.UserInfo
	expiresAt time.Time
}

// cachedRoles stores role names with expiration time.
type cachedRoles struct {
	roles     []string
	expiresAt time.Time
}

type AuthService struct {
	db            *pgxpool.Pool
	authRepo      interfaces.AuthRepository
	cache         sync.Map // key: apiKeyHash   → *cachedUserInfo
	rolesCache    sync.Map // key: userID:type   → *cachedRoles
	cacheTTL      time.Duration
	writeBackPool *workerpool.Pool
	jwksVerifier  *sharedauth.JWKSVerifier
	stopCleanup   chan struct{}
	sfApiKey      singleflight.Group // Prevents cache stampedes on API key validation
	sfRoles       singleflight.Group // Prevents cache stampedes on role lookup
}

func NewAuthService(authRepo interfaces.AuthRepository, cacheTTLSeconds int) *AuthService {
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
			s.performCleanup()
		case <-s.stopCleanup:
			return
		}
	}
}

func (s *AuthService) performCleanup() {
	now := time.Now()
	var cacheCount, rolesCount int
	s.cache.Range(func(key, value interface{}) bool {
		cacheCount++
		if cached, ok := value.(*cachedUserInfo); ok && now.After(cached.expiresAt) {
			s.cache.Delete(key)
			cacheCount--
		}
		return true
	})
	s.rolesCache.Range(func(key, value interface{}) bool {
		rolesCount++
		if cached, ok := value.(*cachedRoles); ok && now.After(cached.expiresAt) {
			s.rolesCache.Delete(key)
			rolesCount--
		}
		return true
	})
	// Evict excess entries beyond max cap to prevent unbounded memory growth
	if cacheCount > maxCacheEntries {
		evicted := 0
		s.cache.Range(func(key, value interface{}) bool {
			s.cache.Delete(key)
			evicted++
			return evicted < cacheCount-maxCacheEntries
		})
	}
	if rolesCount > maxCacheEntries {
		evicted := 0
		s.rolesCache.Range(func(key, value interface{}) bool {
			s.rolesCache.Delete(key)
			evicted++
			return evicted < rolesCount-maxCacheEntries
		})
	}
}

func (s *AuthService) ValidateApiKey(apiKey string) (*interfaces.UserInfo, error) {
	if s.authRepo == nil {
		return nil, ErrDatabaseNotConfigured
	}

	keyHash := hashAPIKey(apiKey)

	// Fast path: serve from cache
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

	// Use singleflight to prevent cache stampede: when many concurrent
	// requests share the same API key and the cache entry expires, only
	// one DB lookup runs; the rest wait and share the result.
	v, err, _ := s.sfApiKey.Do(keyHash, func() (interface{}, error) {
		// Double-check: another goroutine in this singleflight group may have populated the cache
		if cached, ok := s.cache.Load(keyHash); ok {
			if cachedInfo, ok := cached.(*cachedUserInfo); ok && time.Now().Before(cachedInfo.expiresAt) {
				return cachedInfo.userInfo, nil
			}
		}

		userInfo, err := s.validateApiKeyFromDB(keyHash)
		if err != nil {
			return nil, err
		}

		s.cache.Store(keyHash, &cachedUserInfo{
			userInfo:  userInfo,
			expiresAt: time.Now().Add(s.cacheTTL),
		})
		return userInfo, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*interfaces.UserInfo), nil
}

func (s *AuthService) validateApiKeyFromDB(keyHash string) (*interfaces.UserInfo, error) {
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

	return &interfaces.UserInfo{
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

	// Use singleflight to prevent cache stampede: when a popular user's
	// cached roles expire, only one DB lookup runs for all concurrent requests.
	v, err, _ := s.sfRoles.Do(cacheKey, func() (interface{}, error) {
		// Double-check cache after acquiring the singleflight slot
		if cached, ok := s.rolesCache.Load(cacheKey); ok {
			if cr, ok := cached.(*cachedRoles); ok && time.Now().Before(cr.expiresAt) {
				return cr.roles, nil
			}
		}

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
	})
	if err != nil {
		return nil, err
	}
	return v.([]string), nil
}

// hashAPIKey returns the hex-encoded SHA-256 hash of an API key.
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
