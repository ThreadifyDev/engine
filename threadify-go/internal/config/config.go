package config

import (
	"threadify-go/shared/registry"
	"time"
)

// Config represents the complete application configuration
type Config struct {
	// Invocation-only settings are never accepted from YAML.
	ConfigPath          string                   `yaml:"-" mapstructure:"-"`
	WithAgent           bool                     `yaml:"-" mapstructure:"-"`
	AgentVersion        string                   `yaml:"-" mapstructure:"-"`
	AgentCacheDir       string                   `yaml:"-" mapstructure:"-"`
	AgentRuntimeArchive string                   `yaml:"-" mapstructure:"-"`
	AI                  *AIConfig                `yaml:"ai" mapstructure:"ai"`
	Registry            registry.Config          `yaml:"registry" mapstructure:"registry"`
	RuntimeMode         string                   `yaml:"runtime_mode" mapstructure:"runtime_mode"`
	Server              ServerConfig             `yaml:"server" mapstructure:"server"`
	Postgres            PostgresConfig           `yaml:"postgres" mapstructure:"postgres"`
	Redis               RedisConfig              `yaml:"redis" mapstructure:"redis"`
	JWT                 JWTConfig                `yaml:"jwt" mapstructure:"jwt"`
	Auth                AuthConfig               `yaml:"auth" mapstructure:"auth"`
	Queue               QueueConfig              `yaml:"queue" mapstructure:"queue"`
	ThreadActivities    ThreadActivitiesConfig   `yaml:"thread_activities" mapstructure:"thread_activities"`
	Cache               CacheConfig              `yaml:"cache" mapstructure:"cache"`
	Invitations         InvitationsConfig        `yaml:"invitations" mapstructure:"invitations"`
	Logging             LoggingConfig            `yaml:"logging" mapstructure:"logging"`
	Timeouts            TimeoutsConfig           `yaml:"timeouts" mapstructure:"timeouts"`
	NotificationSystem  NotificationSystemConfig `yaml:"notification_system" mapstructure:"notification_system"`
	Archiver            ArchiverConfig           `yaml:"archiver" mapstructure:"archiver"`
	NATS                NATSConfig               `yaml:"nats" mapstructure:"nats"`
	Security            SecurityConfig           `yaml:"security" mapstructure:"security"`
	WebSocket           WebSocketConfig          `yaml:"websocket" mapstructure:"websocket"`
	WorkerPools         WorkerPoolsConfig        `yaml:"worker_pools" mapstructure:"worker_pools"`
	Performance         PerformanceConfig        `yaml:"performance" mapstructure:"performance"`
	JWKS                JWKSSettings             `yaml:"jwks" mapstructure:"jwks"`
	Supabase            SupabaseSettings         `yaml:"supabase" mapstructure:"supabase"`
	Batch               BatchConfig              `yaml:"batch" mapstructure:"batch"`
}

// AIConfig is shared with the separately running agent through the same YAML.
// Gateway secrets and TLS files are resolved only by the model-calling process.
type AIConfig struct {
	Enabled *bool `yaml:"enabled" mapstructure:"enabled"`
	Agent   struct {
		URL string `yaml:"url" mapstructure:"url"`
	} `yaml:"agent" mapstructure:"agent"`
	Gateway struct {
		Auth      string `yaml:"auth" mapstructure:"auth"`
		BaseURL   string `yaml:"base_url" mapstructure:"base_url"`
		Model     string `yaml:"model" mapstructure:"model"`
		APIKeyEnv string `yaml:"api_key_env" mapstructure:"api_key_env"`
		TLS       struct {
			CAFile   string `yaml:"ca_file" mapstructure:"ca_file"`
			CertFile string `yaml:"cert_file" mapstructure:"cert_file"`
			KeyFile  string `yaml:"key_file" mapstructure:"key_file"`
		} `yaml:"tls" mapstructure:"tls"`
	} `yaml:"gateway" mapstructure:"gateway"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	PublicURL string `yaml:"public_url" mapstructure:"public_url"`
	Port      int    `yaml:"port" mapstructure:"port"`
	Host      string `yaml:"host" mapstructure:"host"`
}

// PostgresConfig holds PostgreSQL configuration
type PostgresConfig struct {
	URL                string `yaml:"url" mapstructure:"url"`
	Host               string `yaml:"host" mapstructure:"host"`
	Port               int    `yaml:"port" mapstructure:"port"`
	User               string `yaml:"user" mapstructure:"user"`
	Password           string `yaml:"password" mapstructure:"password"`
	Name               string `yaml:"name" mapstructure:"name"`
	MaxConnections     int    `yaml:"max_connections" mapstructure:"max_connections"`
	MaxIdleConnections int    `yaml:"max_idle_connections" mapstructure:"max_idle_connections"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Mode                  string `mapstructure:"mode" yaml:"mode"`
	Bind                  string `mapstructure:"bind" yaml:"bind"`
	StoreDir              string `mapstructure:"store_dir" yaml:"store_dir"`
	BinaryPath            string `mapstructure:"binary_path" yaml:"binary_path"`
	StartupTimeoutSeconds int    `mapstructure:"startup_timeout_seconds" yaml:"startup_timeout_seconds"`

	URL               string `yaml:"url" mapstructure:"url"`
	PoolSize          int    `yaml:"pool_size" mapstructure:"pool_size"`
	MinIdleConns      int    `yaml:"min_idle_conns" mapstructure:"min_idle_conns"`
	MaxIdleConns      int    `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`
	MaxRetries        int    `yaml:"max_retries" mapstructure:"max_retries"`
	DialTimeoutMs     int    `yaml:"dial_timeout_ms" mapstructure:"dial_timeout_ms"`
	ReadTimeoutMs     int    `yaml:"read_timeout_ms" mapstructure:"read_timeout_ms"`
	WriteTimeoutMs    int    `yaml:"write_timeout_ms" mapstructure:"write_timeout_ms"`
	PoolTimeoutMs     int    `yaml:"pool_timeout_ms" mapstructure:"pool_timeout_ms"`
	ConnMaxIdleTimeMs int    `yaml:"conn_max_idle_time_ms" mapstructure:"conn_max_idle_time_ms"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret          string `yaml:"secret" mapstructure:"secret"`
	Issuer          string `yaml:"issuer" mapstructure:"issuer"`
	Audience        string `yaml:"audience" mapstructure:"audience"`
	Realm           string `yaml:"realm" mapstructure:"realm"`
	ExpirationHours int    `yaml:"expiration_hours" mapstructure:"expiration_hours"`
}

// AuthConfig holds authentication service configuration
type AuthConfig struct {
	CacheTTLSeconds int `yaml:"cache_ttl_seconds" mapstructure:"cache_ttl_seconds"` // API key and roles cache TTL (default: 3600)
}

// QueueConfig holds queue configuration
type QueueConfig struct {
	TTLSeconds int `yaml:"ttl_seconds" mapstructure:"ttl_seconds"`
}

// ThreadActivitiesConfig holds thread activities configuration
type ThreadActivitiesConfig struct {
	BatchSize      int `yaml:"batch_size" mapstructure:"batch_size"`
	BatchTimeoutMs int `yaml:"batch_timeout_ms" mapstructure:"batch_timeout_ms"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	ContractTTLMs  int `yaml:"contract_ttl_ms" mapstructure:"contract_ttl_ms"`     // Default: 18000000ms (5 hours)
	ThreadTTLMs    int `yaml:"thread_ttl_ms" mapstructure:"thread_ttl_ms"`         // Default: 18000000ms (5 hours, hot cache, PostgreSQL fallback)
	StepEventTTLMs int `yaml:"step_event_ttl_ms" mapstructure:"step_event_ttl_ms"` // Default: 18000000ms (5 hours)
	SessionTTLMs   int `yaml:"session_ttl_ms" mapstructure:"session_ttl_ms"`       // Default: 1800000ms (30 minutes, ephemeral, no persistence)
	PlanTTLMs      int `yaml:"plan_ttl_ms" mapstructure:"plan_ttl_ms"`             // Default: 10000ms (10 seconds)
}

// InvitationsConfig holds invitation configuration
type InvitationsConfig struct {
	DefaultPermissions string `yaml:"default_permissions" mapstructure:"default_permissions"`
	DefaultExpiry      string `yaml:"default_expiry" mapstructure:"default_expiry"`
	MaxExpiry          string `yaml:"max_expiry" mapstructure:"max_expiry"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level string `yaml:"level" mapstructure:"level"`
}

// TimeoutsConfig holds timeout configuration for context operations
type TimeoutsConfig struct {
	DefaultOperationSeconds  int `yaml:"default_operation_seconds" mapstructure:"default_operation_seconds"`
	ValidationSeconds        int `yaml:"validation_seconds" mapstructure:"validation_seconds"`
	DatabaseQuerySeconds     int `yaml:"database_query_seconds" mapstructure:"database_query_seconds"`
	RedisOperationSeconds    int `yaml:"redis_operation_seconds" mapstructure:"redis_operation_seconds"`
	NatsPublishSeconds       int `yaml:"nats_publish_seconds" mapstructure:"nats_publish_seconds"`
	ArchivalOperationSeconds int `yaml:"archival_operation_seconds" mapstructure:"archival_operation_seconds"`
}

// NATSConfig holds NATS configuration
type NATSConfig struct {
	Mode                       string `yaml:"mode" mapstructure:"mode"`
	StoreDir                   string `yaml:"store_dir" mapstructure:"store_dir"`
	MaxMemoryBytes             int64  `yaml:"max_memory_bytes" mapstructure:"max_memory_bytes"`
	MaxStoreBytes              int64  `yaml:"max_store_bytes" mapstructure:"max_store_bytes"`
	ArchivalMaxBytes           int64  `yaml:"archival_max_bytes" mapstructure:"archival_max_bytes"`
	ArchivalMaxAgeHours        int    `yaml:"archival_max_age_hours" mapstructure:"archival_max_age_hours"`
	URL                        string `yaml:"url" mapstructure:"url"`
	ClusterID                  string `yaml:"cluster_id" mapstructure:"cluster_id"`
	ClientID                   string `yaml:"client_id" mapstructure:"client_id"`
	StreamName                 string `yaml:"stream_name" mapstructure:"stream_name"`
	RetentionHours             int    `yaml:"retention_hours" mapstructure:"retention_hours"`
	MaxAgeHours                int    `yaml:"max_age_hours" mapstructure:"max_age_hours"`
	AckWaitSeconds             int    `yaml:"ack_wait_seconds" mapstructure:"ack_wait_seconds"`
	ConsumerAckWaitSeconds     int    `yaml:"consumer_ack_wait_seconds" mapstructure:"consumer_ack_wait_seconds"`
	ConsumerMaxDeliver         int    `yaml:"consumer_max_deliver" mapstructure:"consumer_max_deliver"`
	ConsumerMaxAckPending      int    `yaml:"consumer_max_ack_pending" mapstructure:"consumer_max_ack_pending"`
	ArchiverMaxDeliver         int    `yaml:"archiver_max_deliver" mapstructure:"archiver_max_deliver"`
	ArchiverAckWaitSeconds     int    `yaml:"archiver_ack_wait_seconds" mapstructure:"archiver_ack_wait_seconds"`
	NotificationsRetentionDays int    `yaml:"notifications_retention_days" mapstructure:"notifications_retention_days"`
	DLQRetentionDays           int    `yaml:"dlq_retention_days" mapstructure:"dlq_retention_days"`
	PoolSize                   int    `yaml:"pool_size" mapstructure:"pool_size"`
}

// NotificationSystemConfig holds notification system configuration
type NotificationSystemConfig struct {
	Scopes       map[string]ScopeConfig `yaml:"scopes" mapstructure:"scopes"`
	DefaultScope string                 `yaml:"default_scope" mapstructure:"default_scope"`
}

// ScopeConfig holds configuration for a notification scope
type ScopeConfig struct {
	Description string   `yaml:"description" mapstructure:"description"`
	Permissions []string `yaml:"permissions" mapstructure:"permissions"`
}

// ArchiverConfig holds archiver configuration
type ArchiverConfig struct {
	Enabled         bool                    `yaml:"enabled" mapstructure:"enabled"`
	MetricsPort     int                     `yaml:"metrics_port" mapstructure:"metrics_port"`
	Buffers         map[string]BufferConfig `yaml:"buffers" mapstructure:"buffers"`
	ActivityStreams ActivityStreamsConfig   `yaml:"activity_streams" mapstructure:"activity_streams"`
	Retry           RetryConfig             `yaml:"retry" mapstructure:"retry"`
	Streams         StreamsConfig           `yaml:"streams" mapstructure:"streams"`
}

// BufferConfig holds buffer configuration
type BufferConfig struct {
	Size            int `yaml:"size" mapstructure:"size"`
	FlushIntervalMs int `yaml:"flush_interval_ms" mapstructure:"flush_interval_ms"`
}

// ActivityStreamsConfig holds activity streams configuration
type ActivityStreamsConfig struct {
	Enabled            bool `yaml:"enabled" mapstructure:"enabled"`
	NumPartitions      int  `yaml:"num_partitions" mapstructure:"num_partitions"`
	WorkersPerInstance int  `yaml:"workers_per_instance" mapstructure:"workers_per_instance"`
	BufferSize         int  `yaml:"buffer_size" mapstructure:"buffer_size"`
	FlushIntervalMs    int  `yaml:"flush_interval_ms" mapstructure:"flush_interval_ms"`
	MaxBufferSize      int  `yaml:"max_buffer_size" mapstructure:"max_buffer_size"`
	TrimEnabled        bool `yaml:"trim_enabled" mapstructure:"trim_enabled"`
	TrimMaxLen         int  `yaml:"trim_maxlen" mapstructure:"trim_maxlen"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts      int           `yaml:"max_attempts" mapstructure:"max_attempts"`
	InitialBackoffMs int           `yaml:"initial_backoff_ms" mapstructure:"initial_backoff_ms"`
	MaxBackoffMs     int           `yaml:"max_backoff_ms" mapstructure:"max_backoff_ms"`
	InitialBackoff   time.Duration // Computed from InitialBackoffMs
	MaxBackoff       time.Duration // Computed from MaxBackoffMs
}

// StreamsConfig holds streams configuration
type StreamsConfig struct {
	ConsumerGroup            string        `yaml:"consumer_group" mapstructure:"consumer_group"`
	BlockTimeoutMs           int           `yaml:"block_timeout_ms" mapstructure:"block_timeout_ms"`
	BatchSize                int           `yaml:"batch_size" mapstructure:"batch_size"`
	StepStateFlushIntervalMs int           `yaml:"step_state_flush_interval_ms" mapstructure:"step_state_flush_interval_ms"`
	BlockTimeout             time.Duration // Computed
	StepStateFlushInterval   time.Duration // Computed
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	HashChainSecrets        map[string]string `yaml:"hash_chain_secrets" mapstructure:"hash_chain_secrets"`
	HashChainCurrentVersion string            `yaml:"hash_chain_current_version" mapstructure:"hash_chain_current_version"`
}

// WebSocketConfig holds WebSocket configuration
type WebSocketConfig struct {
	HandshakeTimeoutSeconds int `yaml:"handshake_timeout_seconds" mapstructure:"handshake_timeout_seconds"`
	ReadBufferSize          int `yaml:"read_buffer_size" mapstructure:"read_buffer_size"`
	WriteBufferSize         int `yaml:"write_buffer_size" mapstructure:"write_buffer_size"`
	MaxInFlightMax          int `yaml:"max_in_flight_max" mapstructure:"max_in_flight_max"`
	MaxInFlightDefault      int `yaml:"max_in_flight_default" mapstructure:"max_in_flight_default"`
	ReadDeadlineSeconds     int `yaml:"read_deadline_seconds" mapstructure:"read_deadline_seconds"`
}

// WorkerPoolsConfig holds configuration for all worker pools
type WorkerPoolsConfig struct {
	Validation   PoolConfig `yaml:"validation" mapstructure:"validation"`
	Notification PoolConfig `yaml:"notification" mapstructure:"notification"`
	WriteBack    PoolConfig `yaml:"writeback" mapstructure:"writeback"`
	Archival     PoolConfig `yaml:"archival" mapstructure:"archival"`
	Activity     PoolConfig `yaml:"activity" mapstructure:"activity"`
}

// PoolConfig holds configuration for a single worker pool
type PoolConfig struct {
	MinWorkers        int `yaml:"min_workers" mapstructure:"min_workers"`
	MaxWorkers        int `yaml:"max_workers" mapstructure:"max_workers"`
	QueueSize         int `yaml:"queue_size" mapstructure:"queue_size"`
	ScaleUpThreshold  int `yaml:"scale_up_threshold" mapstructure:"scale_up_threshold"`
	ScaleDownAfterMs  int `yaml:"scale_down_after_ms" mapstructure:"scale_down_after_ms"`
	JobTimeoutMs      int `yaml:"job_timeout_ms" mapstructure:"job_timeout_ms"`
	SubmitRetryWaitMs int `yaml:"submit_retry_wait_ms" mapstructure:"submit_retry_wait_ms"`
}

// PerformanceConfig holds performance monitoring configuration
type PerformanceConfig struct {
	MonitoringEnabled bool `yaml:"monitoring_enabled" mapstructure:"monitoring_enabled"`
}

// JWKSSettings holds the JWKS endpoint configuration
type JWKSSettings struct {
	URL      string `yaml:"url" mapstructure:"url"`
	Audience string `yaml:"audience" mapstructure:"audience"`
	Issuer   string `yaml:"issuer" mapstructure:"issuer"`
}

type SupabaseSettings struct {
	URL                   string `yaml:"url" mapstructure:"url"`
	PublishableKey        string `yaml:"publishable_key" mapstructure:"publishable_key"`
	RequestTimeoutSeconds int    `yaml:"request_timeout_seconds" mapstructure:"request_timeout_seconds"`
}

type BatchConfig struct {
	IntervalMs  int `yaml:"interval_ms" mapstructure:"interval_ms"`
	ChannelSize int `yaml:"channel_size" mapstructure:"channel_size"`
}
