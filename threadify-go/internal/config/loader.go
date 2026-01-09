package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// LoadFromViper loads configuration from viper into a Config struct
func LoadFromViper() (*Config, error) {
	cfg := &Config{}

	// Load all config sections
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Compute time.Duration fields from milliseconds
	if cfg.Archiver.Retry.InitialBackoffMs > 0 {
		cfg.Archiver.Retry.InitialBackoff = time.Duration(cfg.Archiver.Retry.InitialBackoffMs) * time.Millisecond
	}
	if cfg.Archiver.Retry.MaxBackoffMs > 0 {
		cfg.Archiver.Retry.MaxBackoff = time.Duration(cfg.Archiver.Retry.MaxBackoffMs) * time.Millisecond
	}
	if cfg.Archiver.Streams.BlockTimeoutMs > 0 {
		cfg.Archiver.Streams.BlockTimeout = time.Duration(cfg.Archiver.Streams.BlockTimeoutMs) * time.Millisecond
	}

	return cfg, nil
}
