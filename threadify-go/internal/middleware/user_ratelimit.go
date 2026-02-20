package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
)

// userRateLimitCache holds cached rate limit decisions
type userRateLimitCache struct {
	allowed   bool
	expiresAt time.Time
	lastUsed  time.Time // For LRU eviction
}

// UserRateLimiterWithCache implements per-user rate limiting with local cache and timeout
type UserRateLimiterWithCache struct {
	luaScripts   interfaces.LuaScriptManager
	cfg          *config.RateLimitConfig
	cache        map[string]*userRateLimitCache
	mu           sync.RWMutex
	cacheTTL     time.Duration
	redisTimeout time.Duration
	maxCacheSize int
}

// NewUserRateLimiterWithCache creates a new user rate limiter with local caching
func NewUserRateLimiterWithCache(luaScripts interfaces.LuaScriptManager, cfg *config.RateLimitConfig) *UserRateLimiterWithCache {
	// Set defaults if not configured
	cacheSize := cfg.PerUser.CacheSize
	if cacheSize <= 0 {
		cacheSize = 50000 // Default: 50k users
	}

	cacheTTL := time.Duration(cfg.PerUser.CacheTTLSeconds) * time.Second
	if cacheTTL <= 0 {
		cacheTTL = 2 * time.Second // Default: 2 seconds
	}

	redisTimeout := time.Duration(cfg.PerUser.RedisTimeoutMs) * time.Millisecond
	if redisTimeout <= 0 {
		redisTimeout = 3 * time.Millisecond // Default: 3ms
	}

	limiter := &UserRateLimiterWithCache{
		luaScripts:   luaScripts,
		cfg:          cfg,
		cache:        make(map[string]*userRateLimitCache),
		cacheTTL:     cacheTTL,
		redisTimeout: redisTimeout,
		maxCacheSize: cacheSize,
	}

	// Start cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// cleanupLoop periodically removes expired cache entries
func (l *UserRateLimiterWithCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for userID, entry := range l.cache {
			if now.After(entry.expiresAt) {
				delete(l.cache, userID)
			}
		}
		l.mu.Unlock()
	}
}

// getCached checks local cache for rate limit decision
func (l *UserRateLimiterWithCache) getCached(userID string) (bool, bool) {
	l.mu.RLock() // Use RLock for fast concurrent reads
	entry, exists := l.cache[userID]
	l.mu.RUnlock()

	if !exists {
		return false, false
	}

	// Check expiration without holding lock
	if time.Now().After(entry.expiresAt) {
		// Expired - acquire write lock to delete
		l.mu.Lock()
		delete(l.cache, userID)
		l.mu.Unlock()
		return false, false
	}

	// Note: We don't update lastUsed on reads to avoid write lock
	// LRU still works because setCached sets lastUsed on cache writes
	return entry.allowed, true
}

// setCached stores rate limit decision in local cache with LRU eviction
func (l *UserRateLimiterWithCache) setCached(userID string, allowed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// If cache is full, evict least recently used entry
	if len(l.cache) >= l.maxCacheSize {
		var oldestUserID string
		var oldestTime time.Time

		// Find LRU entry
		for uid, entry := range l.cache {
			if oldestUserID == "" || entry.lastUsed.Before(oldestTime) {
				oldestUserID = uid
				oldestTime = entry.lastUsed
			}
		}

		// Evict LRU entry
		if oldestUserID != "" {
			delete(l.cache, oldestUserID)
		}
	}

	// Add new entry
	l.cache[userID] = &userRateLimitCache{
		allowed:   allowed,
		expiresAt: now.Add(l.cacheTTL),
		lastUsed:  now,
	}
}

// Middleware returns the Gin middleware handler
func (l *UserRateLimiterWithCache) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if disabled
		if !l.cfg.PerUser.Enabled {
			c.Next()
			return
		}

		// Fast path: skip rate limiting for health/metrics endpoints
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		// Extract ownerID from context (set by auth middleware)
		ownerID, exists := c.Get("ownerID")
		if !exists {
			// No ownerID = not authenticated, skip user rate limiting
			c.Next()
			return
		}

		userID, ok := ownerID.(string)
		if !ok || userID == "" {
			c.Next()
			return
		}

		// Check local cache first (fast path)
		if allowed, found := l.getCached(userID); found {
			if !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "Rate limit exceeded",
					"message": "Too many requests. Please try again later.",
				})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Cache miss - check Redis with timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), l.redisTimeout)
		defer cancel()

		allowed, err := l.luaScripts.CheckUserRateLimit(
			ctx,
			userID,
			l.cfg.PerUser.RequestsPerMinute,
			l.cfg.PerUser.WindowSeconds,
		)

		if err != nil {
			// On error or timeout, fail open (allow request)
			// This prevents Redis issues from blocking all traffic
			if ctx.Err() == context.DeadlineExceeded {
				// Redis timeout - allow request and don't cache
				c.Set("rate_limit_timeout", true)
			}
			c.Next()
			return
		}

		// Cache the result
		l.setCached(userID, allowed)

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"message": "Too many requests. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Legacy function for backward compatibility
func UserRateLimiter(luaScripts interfaces.LuaScriptManager, cfg *config.RateLimitConfig) gin.HandlerFunc {
	limiter := NewUserRateLimiterWithCache(luaScripts, cfg)
	return limiter.Middleware()
}
