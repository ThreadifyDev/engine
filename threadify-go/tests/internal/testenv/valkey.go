package testenv

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

type ValkeyContainer struct {
	*redis.RedisContainer
	URI string
}

func StartValkeyContainer(ctx context.Context) (*ValkeyContainer, error) {
	// Valkey is a fork of Redis, so we can use the Redis container module
	container, err := redis.Run(ctx,
		"valkey/valkey:9.0.0",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start valkey container: %w", err)
	}

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	return &ValkeyContainer{
		RedisContainer: container,
		URI:            uri,
	}, nil
}

func (v *ValkeyContainer) Terminate(ctx context.Context) error {
	if v == nil || v.RedisContainer == nil {
		return nil
	}
	return v.RedisContainer.Terminate(ctx)
}
