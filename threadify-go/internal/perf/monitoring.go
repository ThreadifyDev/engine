package perf

import (
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Global flag and logger for performance monitoring
var (
	monitoringEnabled atomic.Bool
	perfLogger        *zap.Logger
)

// Initialize sets the performance monitoring flag and logger from config
func Initialize(enabled bool, logger *zap.Logger) {
	monitoringEnabled.Store(enabled)
	perfLogger = logger
}

// IsEnabled returns whether performance monitoring is enabled
func IsEnabled() bool {
	return monitoringEnabled.Load() && perfLogger != nil
}

// Now returns the current time if monitoring is enabled, otherwise returns zero time
func Now() time.Time {
	if monitoringEnabled.Load() {
		return time.Now()
	}
	return time.Time{}
}

// Since returns the duration since start if monitoring is enabled, otherwise returns 0
func Since(start time.Time) time.Duration {
	if monitoringEnabled.Load() {
		return time.Since(start)
	}
	return 0
}

// Log logs a performance message if monitoring is enabled (backward compatible with format strings)
func Log(format string, args ...interface{}) {
	if monitoringEnabled.Load() && perfLogger != nil {
		perfLogger.Info(fmt.Sprintf(format, args...))
	}
}

// LogStructured logs a performance message with fields if monitoring is enabled
func LogStructured(message string, fields ...zap.Field) {
	if monitoringEnabled.Load() && perfLogger != nil {
		perfLogger.Info(message, fields...)
	}
}

// Logf is an alias for Log for consistency with log.Printf naming
func Logf(format string, args ...interface{}) {
	Log(format, args...)
}
