package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const defaultMetricsPort = 8082

func LoadFromViper() (*Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Expand environment variables for JWKS configuration
	cfg.JWKS.URL = expandEnv(cfg.JWKS.URL)
	cfg.JWKS.Audience = expandEnv(cfg.JWKS.Audience)
	cfg.JWKS.Issuer = expandEnv(cfg.JWKS.Issuer)

	cfg.Archiver.Retry.InitialBackoff = time.Duration(cfg.Archiver.Retry.InitialBackoffMs) * time.Millisecond
	cfg.Archiver.Retry.MaxBackoff = time.Duration(cfg.Archiver.Retry.MaxBackoffMs) * time.Millisecond
	cfg.Archiver.Streams.BlockTimeout = time.Duration(cfg.Archiver.Streams.BlockTimeoutMs) * time.Millisecond
	cfg.Archiver.Streams.StepStateFlushInterval = time.Duration(cfg.Archiver.Streams.StepStateFlushIntervalMs) * time.Millisecond
	if cfg.Archiver.MetricsPort == 0 {
		cfg.Archiver.MetricsPort = defaultMetricsPort
	}

	return &cfg, nil
}

// expandEnv expands environment variables in the format "$VAR:default"
// If the environment variable is not set, it returns the default value after the colon
func expandEnv(value string) string {
	if !strings.HasPrefix(value, "$") {
		return value
	}

	parts := strings.SplitN(value[1:], ":", 2)
	envVar := parts[0]
	defaultVal := ""
	if len(parts) == 2 {
		defaultVal = parts[1]
	}

	if val := os.Getenv(envVar); val != "" {
		return val
	}
	return defaultVal
}
