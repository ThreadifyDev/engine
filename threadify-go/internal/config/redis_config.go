package config

import "time"

// RedisTTLConfig contains TTL configurations for different Redis keys
type RedisTTLConfig struct {
	Thread      time.Duration `yaml:"thread"`      // Thread metadata TTL
	Roles       time.Duration `yaml:"roles"`       // Role hash TTL
	Permissions time.Duration `yaml:"permissions"` // Permission hash TTL
	Events      time.Duration `yaml:"events"`      // Event queue TTL
	Session     time.Duration `yaml:"session"`     // Session TTL
}

// DefaultRedisTTLConfig returns default TTL configurations
func DefaultRedisTTLConfig() RedisTTLConfig {
	return RedisTTLConfig{
		Thread:      24 * time.Hour,
		Roles:       24 * time.Hour,
		Permissions: 24 * time.Hour,
		Events:      72 * time.Hour,
		Session:     1 * time.Hour,
	}
}
