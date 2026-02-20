package perf

import (
	"log"
	"sync/atomic"
	"time"
)

// Global flag for performance monitoring (set once at startup)
var monitoringEnabled atomic.Bool

// Initialize sets the performance monitoring flag from config
// This should be called once during application startup
func Initialize(enabled bool) {
	monitoringEnabled.Store(enabled)
}

// IsEnabled returns whether performance monitoring is enabled
func IsEnabled() bool {
	return monitoringEnabled.Load()
}

// Now returns the current time if monitoring is enabled, otherwise returns zero time
// This eliminates time.Now() allocations when monitoring is disabled
func Now() time.Time {
	if monitoringEnabled.Load() {
		return time.Now()
	}
	return time.Time{}
}

// Since returns the duration since start if monitoring is enabled, otherwise returns 0
// This eliminates time.Since() allocations when monitoring is disabled
func Since(start time.Time) time.Duration {
	if monitoringEnabled.Load() {
		return time.Since(start)
	}
	return 0
}

// Log logs a performance message if monitoring is enabled
// This eliminates log.Printf allocations and formatting when monitoring is disabled
func Log(format string, args ...interface{}) {
	if monitoringEnabled.Load() {
		log.Printf(format, args...)
	}
}

// Logf is an alias for Log for consistency with log.Printf naming
func Logf(format string, args ...interface{}) {
	Log(format, args)
}
