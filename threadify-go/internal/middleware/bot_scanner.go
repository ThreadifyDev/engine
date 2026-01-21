package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
)

// BotScanner detects and blocks automated/bot traffic
type BotScanner struct {
	enabled          bool
	blockKnownBots   bool
	logSuspicious    bool
	suspiciousIPs    map[string]*IPBehavior
	mu               sync.RWMutex
	cleanupInterval  time.Duration
	knownBotPatterns []string
	suspiciousPaths  []string
}

// IPBehavior tracks suspicious behavior patterns
type IPBehavior struct {
	RequestCount    int
	NotFoundCount   int
	SuspiciousCount int
	FirstSeen       time.Time
	LastSeen        time.Time
	UserAgents      map[string]int
	Blocked         bool
}

// NewBotScanner creates a new bot detection middleware
func NewBotScanner(cfg *config.BotScannerConfig) *BotScanner {
	if cfg == nil || !cfg.Enabled {
		return &BotScanner{enabled: false}
	}

	bs := &BotScanner{
		enabled:         true,
		blockKnownBots:  cfg.BlockKnownBots,
		logSuspicious:   cfg.LogSuspicious,
		suspiciousIPs:   make(map[string]*IPBehavior),
		cleanupInterval: time.Duration(cfg.CleanupIntervalMinutes) * time.Minute,
		knownBotPatterns: []string{
			"scanner", "nikto", "nmap", "masscan", "sqlmap",
			"havij", "acunetix", "nessus", "openvas", "metasploit",
			"burpsuite", "zaproxy", "w3af", "skipfish",
		},
		suspiciousPaths: []string{
			"/admin", "/wp-admin", "/phpmyadmin", "/.env", "/.git",
			"/config", "/backup", "/sql", "/database", "/shell",
			"/.aws", "/.ssh", "/api/v1/admin", "/console",
		},
	}

	// Start cleanup goroutine
	go bs.cleanupLoop()

	return bs
}

// Middleware returns the Gin middleware handler
func (bs *BotScanner) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !bs.enabled {
			c.Next()
			return
		}

		ip := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")
		path := c.Request.URL.Path

		// Check if IP is already blocked
		if bs.isBlocked(ip) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Access denied",
			})
			return
		}

		// Check for known bot patterns
		if bs.blockKnownBots && bs.isKnownBot(userAgent) {
			bs.trackBehavior(ip, userAgent, path, true, false)
			// Don't log common bots to avoid log clutter
			if !bs.isCommonBot(userAgent) && bs.logSuspicious {
				c.Set("bot_detected", true)
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Automated access not allowed",
			})
			return
		}

		// Track behavior for pattern detection
		isSuspiciousPath := bs.isSuspiciousPath(path)
		bs.trackBehavior(ip, userAgent, path, false, isSuspiciousPath)

		// Check if behavior is suspicious
		if bs.isSuspiciousBehavior(ip) {
			if bs.logSuspicious {
				c.Set("suspicious_behavior", true)
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Suspicious activity detected",
			})
			return
		}

		c.Next()

		// Track 404s but don't log common scanner paths
		if c.Writer.Status() == 404 {
			bs.track404(ip, path)
			// Only set flag for logging if it's not a common scanner path
			if !isSuspiciousPath && bs.logSuspicious {
				c.Set("track_404", true)
			}
		}
	}
}

// isKnownBot checks if user agent matches known bot patterns
func (bs *BotScanner) isKnownBot(userAgent string) bool {
	// Allow empty user agents (some legitimate clients don't set it)
	if userAgent == "" {
		return false
	}

	lowerUA := strings.ToLower(userAgent)

	// Whitelist legitimate API clients and SDKs
	legitimate := []string{
		"threadify",                            // Our own SDK
		"axios", "fetch", "okhttp", "retrofit", // Common HTTP clients
		"postman", "insomnia", "paw", // API testing tools
	}
	for _, legit := range legitimate {
		if strings.Contains(lowerUA, legit) {
			return false
		}
	}

	// Only block obvious malicious patterns
	for _, pattern := range bs.knownBotPatterns {
		if strings.Contains(lowerUA, pattern) {
			return true
		}
	}
	return false
}

// isCommonBot checks if it's a legitimate/common bot (don't log these)
func (bs *BotScanner) isCommonBot(userAgent string) bool {
	lowerUA := strings.ToLower(userAgent)
	commonBots := []string{
		"googlebot", "bingbot", "slackbot", "twitterbot",
		"facebookexternalhit", "linkedinbot", "discordbot",
	}
	for _, bot := range commonBots {
		if strings.Contains(lowerUA, bot) {
			return true
		}
	}
	return false
}

// isSuspiciousPath checks if path is commonly targeted by scanners
func (bs *BotScanner) isSuspiciousPath(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, suspicious := range bs.suspiciousPaths {
		if strings.Contains(lowerPath, suspicious) {
			return true
		}
	}
	return false
}

// trackBehavior records IP behavior patterns
func (bs *BotScanner) trackBehavior(ip, userAgent, path string, isBot, isSuspicious bool) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	behavior, exists := bs.suspiciousIPs[ip]
	if !exists {
		behavior = &IPBehavior{
			FirstSeen:  time.Now(),
			UserAgents: make(map[string]int),
		}
		bs.suspiciousIPs[ip] = behavior
	}

	behavior.RequestCount++
	behavior.LastSeen = time.Now()
	behavior.UserAgents[userAgent]++

	if isBot || isSuspicious {
		behavior.SuspiciousCount++
	}
}

// track404 records 404 responses
func (bs *BotScanner) track404(ip, path string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if behavior, exists := bs.suspiciousIPs[ip]; exists {
		behavior.NotFoundCount++
	}
}

// isBlocked checks if IP is blocked
func (bs *BotScanner) isBlocked(ip string) bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	if behavior, exists := bs.suspiciousIPs[ip]; exists {
		return behavior.Blocked
	}
	return false
}

// isSuspiciousBehavior detects suspicious patterns
func (bs *BotScanner) isSuspiciousBehavior(ip string) bool {
	bs.mu.RLock()
	behavior, exists := bs.suspiciousIPs[ip]
	if !exists {
		bs.mu.RUnlock()
		return false
	}

	// Check for suspicious patterns (read-only, no blocking yet)
	timeSinceFirst := time.Since(behavior.FirstSeen)
	shouldBlock := false

	// Too many requests in short time (>100 req/min)
	if timeSinceFirst < time.Minute && behavior.RequestCount > 100 {
		shouldBlock = true
	}

	// High 404 rate (>50% of requests)
	if behavior.RequestCount > 10 && float64(behavior.NotFoundCount)/float64(behavior.RequestCount) > 0.5 {
		shouldBlock = true
	}

	// Multiple user agents from same IP (>5 different UAs)
	if len(behavior.UserAgents) > 5 {
		shouldBlock = true
	}

	// High suspicious request rate (>30% suspicious)
	if behavior.RequestCount > 20 && float64(behavior.SuspiciousCount)/float64(behavior.RequestCount) > 0.3 {
		shouldBlock = true
	}

	bs.mu.RUnlock()

	// Block IP if suspicious (acquire write lock separately)
	if shouldBlock {
		bs.blockIP(ip)
		return true
	}

	return false
}

// blockIP blocks an IP address
func (bs *BotScanner) blockIP(ip string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if behavior, exists := bs.suspiciousIPs[ip]; exists {
		behavior.Blocked = true
	}
}

// cleanupLoop periodically cleans up old entries
func (bs *BotScanner) cleanupLoop() {
	ticker := time.NewTicker(bs.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		bs.cleanup()
	}
}

// cleanup removes old entries
func (bs *BotScanner) cleanup() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	now := time.Now()
	for ip, behavior := range bs.suspiciousIPs {
		// Remove entries older than 1 hour (unless blocked)
		if !behavior.Blocked && now.Sub(behavior.LastSeen) > time.Hour {
			delete(bs.suspiciousIPs, ip)
		}
		// Remove blocked IPs after 24 hours
		if behavior.Blocked && now.Sub(behavior.LastSeen) > 24*time.Hour {
			delete(bs.suspiciousIPs, ip)
		}
	}
}

// GetStats returns current bot scanner statistics
func (bs *BotScanner) GetStats() map[string]interface{} {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	blockedCount := 0
	suspiciousCount := 0
	for _, behavior := range bs.suspiciousIPs {
		if behavior.Blocked {
			blockedCount++
		} else if behavior.SuspiciousCount > 0 {
			suspiciousCount++
		}
	}

	return map[string]interface{}{
		"enabled":          bs.enabled,
		"tracked_ips":      len(bs.suspiciousIPs),
		"blocked_ips":      blockedCount,
		"suspicious_ips":   suspiciousCount,
		"block_known_bots": bs.blockKnownBots,
	}
}
