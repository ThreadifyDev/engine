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

	// Compute time.Duration fields from seconds
	if cfg.Archiver.Retry.InitialBackoffSeconds > 0 {
		cfg.Archiver.Retry.InitialBackoff = time.Duration(cfg.Archiver.Retry.InitialBackoffSeconds) * time.Second
	}
	if cfg.Archiver.Retry.MaxBackoffSeconds > 0 {
		cfg.Archiver.Retry.MaxBackoff = time.Duration(cfg.Archiver.Retry.MaxBackoffSeconds) * time.Second
	}
	if cfg.Archiver.Streams.BlockTimeoutSeconds > 0 {
		cfg.Archiver.Streams.BlockTimeout = time.Duration(cfg.Archiver.Streams.BlockTimeoutSeconds) * time.Second
	}

	return cfg, nil
}
