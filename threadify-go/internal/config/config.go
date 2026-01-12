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
	Host     string `yaml:"host" mapstructure:"host"`
	Port     int    `yaml:"port" mapstructure:"port"`
	Password string `yaml:"password" mapstructure:"password"`
	DB       int    `yaml:"db" mapstructure:"db"`
	PoolSize int    `yaml:"pool_size" mapstructure:"pool_size"`
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
	RequestsPerSecond    int `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	BurstSize            int `yaml:"burst_size" mapstructure:"burst_size"`
	CleanupIntervalHours int `yaml:"cleanup_interval_hours" mapstructure:"cleanup_interval_hours"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	ContractTTLHours  int `yaml:"contract_ttl_hours" mapstructure:"contract_ttl_hours"`
	ThreadTTLHours    int `yaml:"thread_ttl_hours" mapstructure:"thread_ttl_hours"`
	StepEventTTLHours int `yaml:"step_event_ttl_hours" mapstructure:"step_event_ttl_hours"`
	SessionTTLMinutes int `yaml:"session_ttl_minutes" mapstructure:"session_ttl_minutes"`
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
	URL            string `yaml:"url" mapstructure:"url"`
	ClusterID      string `yaml:"cluster_id" mapstructure:"cluster_id"`
	ClientID       string `yaml:"client_id" mapstructure:"client_id"`
	StreamName     string `yaml:"stream_name" mapstructure:"stream_name"`
	RetentionHours int    `yaml:"retention_hours" mapstructure:"retention_hours"`
	MaxAgeHours    int    `yaml:"max_age_hours" mapstructure:"max_age_hours"`
	AckWaitSeconds int    `yaml:"ack_wait_seconds" mapstructure:"ack_wait_seconds"`
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
	ConsumerGroup  string        `yaml:"consumer_group" mapstructure:"consumer_group"`
	BlockTimeoutMs int           `yaml:"block_timeout_ms" mapstructure:"block_timeout_ms"`
	BatchSize      int           `yaml:"batch_size" mapstructure:"batch_size"`
	BlockTimeout   time.Duration // Computed from BlockTimeoutMs
}
