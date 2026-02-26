package config

import "time"

// Config represents the complete application configuration
type Config struct {
	Server             ServerConfig             `yaml:"server" mapstructure:"server"`
	Postgres           PostgresConfig           `yaml:"postgres" mapstructure:"postgres"`
	Redis              RedisConfig              `yaml:"redis" mapstructure:"redis"`
	JWT                JWTConfig                `yaml:"jwt" mapstructure:"jwt"`
	Queue              QueueConfig              `yaml:"queue" mapstructure:"queue"`
	ThreadActivities   ThreadActivitiesConfig   `yaml:"thread_activities" mapstructure:"thread_activities"`
	RateLimit          RateLimitConfig          `yaml:"rate_limit" mapstructure:"rate_limit"`
	Cache              CacheConfig              `yaml:"cache" mapstructure:"cache"`
	Invitations        InvitationsConfig        `yaml:"invitations" mapstructure:"invitations"`
	Logging            LoggingConfig            `yaml:"logging" mapstructure:"logging"`
	Timeouts           TimeoutsConfig           `yaml:"timeouts" mapstructure:"timeouts"`
	NotificationSystem NotificationSystemConfig `yaml:"notification_system" mapstructure:"notification_system"`
	Archiver           ArchiverConfig           `yaml:"archiver" mapstructure:"archiver"`
	NATS               NATSConfig               `yaml:"nats" mapstructure:"nats"`
	Security           SecurityConfig           `yaml:"security" mapstructure:"security"`
	WebSocket          WebSocketConfig          `yaml:"websocket" mapstructure:"websocket"`
	BotScanner         BotScannerConfig         `yaml:"bot_scanner" mapstructure:"bot_scanner"`
	WorkerPools        WorkerPoolsConfig        `yaml:"worker_pools" mapstructure:"worker_pools"`
	Performance        PerformanceConfig        `yaml:"performance" mapstructure:"performance"`
	JWKS               JWKSSettings             `yaml:"jwks" mapstructure:"jwks"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port int    `yaml:"port" mapstructure:"port"`
	Host string `yaml:"host" mapstructure:"host"`
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
	Host              string `yaml:"host" mapstructure:"host"`
	Port              int    `yaml:"port" mapstructure:"port"`
	Password          string `yaml:"password" mapstructure:"password"`
	DB                int    `yaml:"db" mapstructure:"db"`
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

// QueueConfig holds queue configuration
type QueueConfig struct {
	TTLSeconds int `yaml:"ttl_seconds" mapstructure:"ttl_seconds"`
}

// ThreadActivitiesConfig holds thread activities configuration
type ThreadActivitiesConfig struct {
	BatchSize      int `yaml:"batch_size" mapstructure:"batch_size"`
	BatchTimeoutMs int `yaml:"batch_timeout_ms" mapstructure:"batch_timeout_ms"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled         bool            `yaml:"enabled" mapstructure:"enabled"`
	CleanupInterval string          `yaml:"cleanup_interval" mapstructure:"cleanup_interval"`
	PerIP           IPLimitConfig   `yaml:"per_ip" mapstructure:"per_ip"`
	PerUser         UserLimitConfig `yaml:"per_user" mapstructure:"per_user"`
}

// IPLimitConfig holds per-IP rate limiting configuration
type IPLimitConfig struct {
	Enabled           bool `yaml:"enabled" mapstructure:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute" mapstructure:"requests_per_minute"`
	Burst             int  `yaml:"burst" mapstructure:"burst"`
}

// UserLimitConfig holds per-user rate limiting configuration
type UserLimitConfig struct {
	Enabled           bool `yaml:"enabled" mapstructure:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute" mapstructure:"requests_per_minute"`
	Burst             int  `yaml:"burst" mapstructure:"burst"`
	WindowSeconds     int  `yaml:"window_seconds" mapstructure:"window_seconds"`
	CacheSize         int  `yaml:"cache_size" mapstructure:"cache_size"`               // Max cached users per pod
	CacheTTLSeconds   int  `yaml:"cache_ttl_seconds" mapstructure:"cache_ttl_seconds"` // Cache TTL
	RedisTimeoutMs    int  `yaml:"redis_timeout_ms" mapstructure:"redis_timeout_ms"`   // Redis timeout in milliseconds
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	ContractTTLMs  int `yaml:"contract_ttl_ms" mapstructure:"contract_ttl_ms"`     // Default: 18000000ms (5 hours)
	ThreadTTLMs    int `yaml:"thread_ttl_ms" mapstructure:"thread_ttl_ms"`         // Default: 18000000ms (5 hours, hot cache, PostgreSQL fallback)
	StepEventTTLMs int `yaml:"step_event_ttl_ms" mapstructure:"step_event_ttl_ms"` // Default: 18000000ms (5 hours)
	SessionTTLMs   int `yaml:"session_ttl_ms" mapstructure:"session_ttl_ms"`       // Default: 1800000ms (30 minutes, ephemeral, no persistence)
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
}

// BotScannerConfig holds bot detection and prevention configuration
type BotScannerConfig struct {
	Enabled                bool `yaml:"enabled" mapstructure:"enabled"`
	BlockKnownBots         bool `yaml:"block_known_bots" mapstructure:"block_known_bots"`
	LogSuspicious          bool `yaml:"log_suspicious" mapstructure:"log_suspicious"`
	CleanupIntervalMinutes int  `yaml:"cleanup_interval_minutes" mapstructure:"cleanup_interval_minutes"`
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
