package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const defaultMetricsPort = 8082

func LoadFromViper(v *viper.Viper) (*Config, error) {
	if v == nil {
		v = viper.GetViper()
	}
	setRuntimeDefaults(v)
	var cfg Config

	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Registry.URL = expandEnv(cfg.Registry.URL)
	cfg.Registry.LicenseKey = expandEnv(cfg.Registry.LicenseKey)
	cfg.Registry.InstallationID = expandEnv(cfg.Registry.InstallationID)
	cfg.Registry.CompanyID = expandEnv(cfg.Registry.CompanyID)
	cfg.NATS.URL = expandEnv(cfg.NATS.URL)
	cfg.NATS.StoreDir = expandEnv(cfg.NATS.StoreDir)
	cfg.Redis.Host = expandEnv(cfg.Redis.Host)
	cfg.Redis.Password = expandEnv(cfg.Redis.Password)
	cfg.JWT.Secret = expandEnv(cfg.JWT.Secret)

	if err := ValidateRuntime(&cfg); err != nil {
		return nil, err
	}

	// Expand environment variables for PostgreSQL URL
	cfg.Postgres.URL = expandEnv(cfg.Postgres.URL)

	// Expand environment variables for JWKS configuration
	cfg.JWKS.URL = expandEnv(cfg.JWKS.URL)
	cfg.JWKS.Audience = expandEnv(cfg.JWKS.Audience)
	cfg.JWKS.Issuer = expandEnv(cfg.JWKS.Issuer)
	cfg.Supabase.URL = expandEnv(cfg.Supabase.URL)
	cfg.Supabase.PublishableKey = expandEnv(cfg.Supabase.PublishableKey)

	// Expand environment variables for hash chain secrets
	for key, secret := range cfg.Security.HashChainSecrets {
		cfg.Security.HashChainSecrets[key] = expandEnv(secret)
	}

	// Expand environment variables for billing configuration

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

// setRuntimeDefaults supplies safe defaults without overriding explicitly configured values.
func setRuntimeDefaults(v *viper.Viper) {
	v.SetDefault("runtime_mode", "combined")
	v.SetDefault("nats.mode", "embedded")
	v.SetDefault("nats.store_dir", "./data/jetstream")
	v.SetDefault("nats.max_memory_bytes", int64(64<<20))
	v.SetDefault("nats.max_store_bytes", int64(8<<30))
	v.SetDefault("nats.archival_max_bytes", int64(1<<30))
	v.SetDefault("nats.archival_max_age_hours", 0)
	v.SetDefault("nats.pool_size", 2)
	v.SetDefault("nats.archiver_max_deliver", -1)
	v.SetDefault("nats.archiver_ack_wait_seconds", 30)
	v.SetDefault("archiver.enabled", true)
	v.SetDefault("archiver.streams.batch_size", 100)
	v.SetDefault("archiver.streams.block_timeout_ms", 1000)
	v.SetDefault("archiver.streams.step_state_flush_interval_ms", 1000)
}

func ValidateRuntime(cfg *Config) error {
	switch cfg.RuntimeMode {
	case "", "combined", "engine", "writer":
	default:
		return fmt.Errorf("runtime_mode must be combined, engine, or writer")
	}
	switch cfg.NATS.Mode {
	case "embedded":
		if strings.TrimSpace(cfg.NATS.StoreDir) == "" {
			return fmt.Errorf("nats.store_dir is required in embedded mode")
		}
		if cfg.NATS.MaxMemoryBytes <= 0 || cfg.NATS.MaxStoreBytes <= 0 {
			return fmt.Errorf("embedded NATS storage and memory limits must be positive")
		}
		if cfg.RuntimeMode == "engine" || cfg.RuntimeMode == "writer" {
			return fmt.Errorf("split engine/writer modes require nats.mode=external; independent embedded brokers do not share messages")
		}
	case "external":
		if cfg.NATS.URL == "" {
			return fmt.Errorf("nats.url is required in external mode")
		}
	default:
		return fmt.Errorf("nats.mode must be embedded or external")
	}
	if cfg.NATS.ArchivalMaxBytes <= 0 || cfg.NATS.ArchivalMaxAgeHours < 0 {
		return fmt.Errorf("archival_max_bytes must be positive and archival_max_age_hours cannot be negative")
	}
	if cfg.NATS.PoolSize <= 0 {
		return fmt.Errorf("nats.pool_size must be positive")
	}
	if cfg.Archiver.Enabled && (cfg.Archiver.Streams.BatchSize <= 0 || cfg.Archiver.Streams.BlockTimeoutMs <= 0 || cfg.Archiver.Streams.StepStateFlushIntervalMs <= 0) {
		return fmt.Errorf("archiver batch size and flush intervals must be positive")
	}
	if cfg.RuntimeMode == "writer" && !cfg.Archiver.Enabled {
		return fmt.Errorf("writer mode requires archiver.enabled=true")
	}
	return nil
}
