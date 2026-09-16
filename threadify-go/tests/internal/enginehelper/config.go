package enginehelper

import (
	"github.com/threadify/engine/internal/config"
)

func GenerateTestConfig(pgConn, valkeyURI, natsURL string, jwksURL string) (*config.Config, error) {
	// Create a minimal config for testing
	cfg := &config.Config{}

	// Basic server config
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0 // Let httptest.NewServer decide

	// Postgres
	cfg.Postgres.URL = pgConn
	cfg.Postgres.MaxConnections = 10

	cfg.Redis.URL = valkeyURI
	cfg.Redis.Mode = "external"

	// These tests explicitly use their shared external broker and seed persistence themselves.
	cfg.RuntimeMode = "engine"
	cfg.NATS.Mode = "external"
	cfg.NATS.URL = natsURL
	cfg.NATS.StreamName = "NOTIFICATIONS"
	cfg.NATS.ConsumerAckWaitSeconds = 1
	cfg.NATS.ConsumerMaxDeliver = 3
	cfg.NATS.PoolSize = 1
	cfg.NATS.RetentionHours = 24
	cfg.NATS.MaxAgeHours = 24
	cfg.NATS.AckWaitSeconds = 30

	// Security
	cfg.Security.HashChainSecrets = map[string]string{"1.0": "test-secret"}
	cfg.Security.HashChainCurrentVersion = "1.0"

	// Invitations
	cfg.Invitations.DefaultPermissions = "read,write"
	cfg.Invitations.DefaultExpiry = "24h"
	cfg.Invitations.MaxExpiry = "168h"

	// JWT
	cfg.JWT.Secret = "test-jwt-secret"
	cfg.JWT.Issuer = "threadify-test"
	cfg.JWT.Audience = "threadify-engine-test"
	cfg.JWT.ExpirationHours = 24

	// JWKS
	cfg.JWKS.URL = jwksURL + "/auth/v1/.well-known/jwks.json"
	cfg.JWKS.Audience = "authenticated"
	cfg.JWKS.Issuer = jwksURL

	// Notification System
	cfg.NotificationSystem.DefaultScope = "participant"
	cfg.NotificationSystem.Scopes = map[string]config.ScopeConfig{
		"owner": {
			Permissions: []string{"read", "write", "admin"},
		},
		"participant": {
			Permissions: []string{"read", "write"},
		},
		"external": {
			Permissions: []string{"read"},
		},
	}

	// Worker Pools
	cfg.WorkerPools.Validation = config.PoolConfig{MinWorkers: 1, MaxWorkers: 2, QueueSize: 10}
	cfg.WorkerPools.Notification = config.PoolConfig{MinWorkers: 1, MaxWorkers: 2, QueueSize: 10}
	cfg.WorkerPools.WriteBack = config.PoolConfig{MinWorkers: 1, MaxWorkers: 2, QueueSize: 10}
	cfg.WorkerPools.Activity = config.PoolConfig{MinWorkers: 1, MaxWorkers: 2, QueueSize: 10}

	// Cache
	cfg.Cache.PlanTTLMs = 100
	cfg.Cache.ThreadTTLMs = 1000
	cfg.Cache.ContractTTLMs = 1000

	// Billing/Subscription
	cfg.Billing.Provider = "noop"

	// Rates

	return cfg, nil
}
