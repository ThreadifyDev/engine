package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

const defaultMetricsPort = 8082

func LoadFromViper() (*Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Archiver.Retry.InitialBackoff = time.Duration(cfg.Archiver.Retry.InitialBackoffMs) * time.Millisecond
	cfg.Archiver.Retry.MaxBackoff = time.Duration(cfg.Archiver.Retry.MaxBackoffMs) * time.Millisecond
	cfg.Archiver.Streams.BlockTimeout = time.Duration(cfg.Archiver.Streams.BlockTimeoutMs) * time.Millisecond
	cfg.Archiver.Streams.StepStateFlushInterval = time.Duration(cfg.Archiver.Streams.StepStateFlushIntervalMs) * time.Millisecond
	if cfg.Archiver.MetricsPort == 0 {
		cfg.Archiver.MetricsPort = defaultMetricsPort
	}

	return &cfg, nil
}
