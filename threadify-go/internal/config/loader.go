package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	sharedconfig "threadify-go/shared/config"
	"time"

	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/redisurl"
)

const defaultMetricsPort = 8082

func LoadFromViper(v *viper.Viper) (*Config, error) {
	if v == nil {
		v = viper.GetViper()
	}
	for _, field := range []string{"host", "port", "password", "db", "username"} {
		if v.IsSet("redis." + field) {
			return nil, fmt.Errorf("redis.%s is no longer supported; use redis.url", field)
		}
	}
	setRuntimeDefaults(v)
	var cfg Config

	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.AI != nil {
		cfg.AI.Agent.URL = expandEnv(cfg.AI.Agent.URL)
	}
	cfg.Classifier.BaseURL = expandEnv(cfg.Classifier.BaseURL)
	cfg.Classifier.Model = expandEnv(cfg.Classifier.Model)

	cfg.Registry.URL = expandEnv(cfg.Registry.URL)
	publicURL, err := sharedconfig.NormalizePublicURL(expandEnv(cfg.Server.PublicURL))
	if err != nil {
		return nil, fmt.Errorf("server.public_url: %w", err)
	}
	cfg.Server.PublicURL = publicURL
	cfg.Registry.BrowserOrigin = expandEnv(cfg.Registry.BrowserOrigin)
	cfg.Registry.LicenseKey = expandEnv(cfg.Registry.LicenseKey)
	cfg.Registry.InstallationID = expandEnv(cfg.Registry.InstallationID)
	cfg.Registry.CompanyID = expandEnv(cfg.Registry.CompanyID)
	cfg.NATS.URL = expandEnv(cfg.NATS.URL)
	cfg.NATS.StoreDir = expandEnv(cfg.NATS.StoreDir)
	// Persistent embedded storage follows the installed binary, never the shell's
	// working directory. Absolute paths still support mounted data volumes.
	if cfg.NATS.Mode == "embedded" && cfg.NATS.StoreDir != "" && !filepath.IsAbs(cfg.NATS.StoreDir) {
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate binary for embedded storage: %w", err)
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return nil, fmt.Errorf("resolve installed binary: %w", err)
		}
		cfg.NATS.StoreDir = filepath.Join(filepath.Dir(executable), cfg.NATS.StoreDir)
	}
	cfg.Redis.URL = expandEnv(cfg.Redis.URL)
	cfg.Redis.Bind = expandEnv(cfg.Redis.Bind)
	cfg.Redis.StoreDir = expandEnv(cfg.Redis.StoreDir)
	cfg.Redis.BinaryPath = expandEnv(cfg.Redis.BinaryPath)
	if cfg.Redis.Mode == "managed" {
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate binary for Valkey: %w", err)
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return nil, fmt.Errorf("resolve installed binary: %w", err)
		}
		for _, path := range []*string{&cfg.Redis.StoreDir, &cfg.Redis.BinaryPath} {
			if *path != "" && !filepath.IsAbs(*path) {
				*path = filepath.Join(filepath.Dir(executable), *path)
			}
		}
	}
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
	// An explicit URL selects an external server unless managed mode is requested.
	// An omitted redis section uses the bundled server on supported platforms.
	mode := "managed"
	if runtime.GOOS == "windows" || v.IsSet("redis.url") || os.Getenv("REDIS_URL") != "" {
		mode = "external"
	}
	if !v.IsSet("redis.mode") {
		v.SetDefault("redis.mode", mode)
	}
	v.SetDefault("redis.url", "$REDIS_URL:redis://127.0.0.1:6379/0")
	v.SetDefault("redis.bind", "127.0.0.1")
	v.SetDefault("redis.store_dir", "data/valkey")
	v.SetDefault("redis.binary_path", "libexec/valkey-server")
	v.SetDefault("redis.startup_timeout_seconds", 120)
	// Preserve the former self-host template's connection capacity when the
	// complete redis section is omitted.
	v.SetDefault("redis.pool_size", 200)
	v.SetDefault("server.public_url", "$THREADIFY_PUBLIC_URL:")
	v.SetDefault("runtime_mode", "combined")
	v.SetDefault("nats.mode", "embedded")
	v.SetDefault("nats.store_dir", "data/jetstream")
	v.SetDefault("nats.max_memory_bytes", int64(64<<20))
	v.SetDefault("nats.max_store_bytes", int64(8<<30))
	v.SetDefault("nats.archival_max_bytes", int64(1<<30))
	v.SetDefault("nats.archival_max_age_hours", 0)
	v.SetDefault("nats.pool_size", 2)
	v.SetDefault("nats.archiver_max_deliver", -1)
	v.SetDefault("nats.archiver_ack_wait_seconds", 30)
	// These defaults make the complete NATS section optional, including the
	// notification stream required by all normal Engine startup paths.
	v.SetDefault("nats.cluster_id", "threadify-cluster")
	v.SetDefault("nats.client_id", "threadify-server")
	v.SetDefault("nats.stream_name", "NOTIFICATIONS")
	v.SetDefault("nats.retention_hours", 48)
	v.SetDefault("nats.max_age_hours", 168)
	v.SetDefault("nats.ack_wait_seconds", 30)
	v.SetDefault("nats.consumer_ack_wait_seconds", 30)
	v.SetDefault("nats.consumer_max_deliver", 3)
	v.SetDefault("nats.consumer_max_ack_pending", 100)
	v.SetDefault("nats.notifications_retention_days", 3)
	v.SetDefault("nats.dlq_retention_days", 7)
	v.SetDefault("archiver.enabled", true)
	v.SetDefault("archiver.streams.batch_size", 100)
	v.SetDefault("archiver.streams.block_timeout_ms", 1000)
	v.SetDefault("archiver.streams.step_state_flush_interval_ms", 1000)
}

func ValidateRuntime(cfg *Config) error {
	if cfg.Classifier.BaseURL != "" {
		if cfg.Classifier.Model == "" {
			return fmt.Errorf("classifier.model is required when classifier.base_url is set")
		}
		if cfg.Classifier.Auth != "" && cfg.Classifier.Auth != "custom" && cfg.Classifier.Auth != "threadify_license" {
			return fmt.Errorf("classifier.auth must be custom or threadify_license")
		}
		if cfg.Classifier.Auth == "threadify_license" && cfg.Registry.LicenseKey == "" {
			return fmt.Errorf("classifier.auth=threadify_license requires registry.license_key")
		}
		if cfg.Classifier.TimeoutMs < 0 || cfg.Classifier.TimeoutMs > 30000 {
			return fmt.Errorf("classifier.timeout_ms must be between 0 and 30000")
		}
	}
	switch cfg.Redis.Mode {
	case "managed", "external":
	default:
		return fmt.Errorf("redis.mode must be managed or external")
	}
	parse := redisurl.Parse
	if cfg.Redis.Mode == "managed" {
		parse = redisurl.Managed
	}
	if _, err := parse(cfg.Redis.URL); err != nil {
		return err
	}
	if cfg.Redis.Mode == "managed" && (cfg.Redis.StoreDir == "" || cfg.Redis.BinaryPath == "" || cfg.Redis.StartupTimeoutSeconds < 1) {
		return fmt.Errorf("managed Valkey requires store_dir, binary_path and a positive startup_timeout_seconds")
	}
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
