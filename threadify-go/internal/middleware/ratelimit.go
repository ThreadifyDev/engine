package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
	"golang.org/x/time/rate"
)

// IPRateLimiter implements per-IP rate limiting using token bucket algorithm
type IPRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	enabled  bool
}

// NewIPRateLimiter creates a new IP-based rate limiter from config
func NewIPRateLimiter(cfg *config.RateLimitConfig) *IPRateLimiter {
	if cfg == nil || !cfg.PerIP.Enabled {
		return &IPRateLimiter{enabled: false}
	}

	// Convert requests per minute to requests per second
	rps := float64(cfg.PerIP.RequestsPerMinute) / 60.0

	return &IPRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    cfg.PerIP.Burst,
		enabled:  true,
	}
}

func (rl *IPRateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}

	return limiter
}

func (rl *IPRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.enabled {
			c.Next()
			return
		}

		// Use IP address as the rate limit key
		key := c.ClientIP()

		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"message": "Too many requests from your IP. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Cleanup removes old limiters periodically
func (rl *IPRateLimiter) Cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			rl.mu.Lock()
			// Clear all limiters (simple approach)
			// In production, you'd track last access time and only remove stale ones
			rl.limiters = make(map[string]*rate.Limiter)
			rl.mu.Unlock()
		}
	}()
}
