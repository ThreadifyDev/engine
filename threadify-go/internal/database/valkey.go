package database

import (
	"context"
	"fmt"
	"time"

	backoffv4 "github.com/cenkalti/backoff/v4"
	"github.com/redis/go-redis/v9"
)

type ValkeyService struct {
	Client *redis.Client
}

func NewValkeyService(host string, port int, password string, db int) (*ValkeyService, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:            fmt.Sprintf("%s:%d", host, port),
		Password:        password,
		DB:              db,
		PoolSize:        50,                     // Increased from 50 for high concurrency
		MinIdleConns:    50,                     // Increased from 5
		MaxRetries:      1,                      // Reduced from 2 - fail fast
		DialTimeout:     1 * time.Second,        // Reduced from 2s
		ReadTimeout:     200 * time.Millisecond, // Reduced from 500ms
		WriteTimeout:    200 * time.Millisecond, // Reduced from 500ms
		PoolTimeout:     200 * time.Millisecond, // Reduced from 1s
		MaxIdleConns:    100,                    // Increased from 15
		ConnMaxIdleTime: 1 * time.Minute,        // Reduced from 2min
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis/valkey: %w", err)
	}

	return &ValkeyService{Client: rdb}, nil
}

func (v *ValkeyService) Close() error {
	if v.Client != nil {
		return v.Client.Close()
	}
	return nil
}

func (v *ValkeyService) Ping(ctx context.Context) error {
	return v.Client.Ping(ctx).Err()
}

func (v *ValkeyService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return v.Client.Set(ctx, key, value, ttl).Err()
}

func (v *ValkeyService) Get(ctx context.Context, key string) (string, error) {
	return v.Client.Get(ctx, key).Result()
}

func (v *ValkeyService) Del(ctx context.Context, keys ...string) error {
	return v.Client.Del(ctx, keys...).Err()
}

func (v *ValkeyService) Exists(ctx context.Context, key string) (bool, error) {
	count, err := v.Client.Exists(ctx, key).Result()
	return count > 0, err
}

func (v *ValkeyService) Keys(ctx context.Context, pattern string) ([]string, error) {
	return v.Client.Keys(ctx, pattern).Result()
}

func (v *ValkeyService) HSet(ctx context.Context, key string, values ...interface{}) error {
	return v.Client.HSet(ctx, key, values...).Err()
}

func (v *ValkeyService) HGet(ctx context.Context, key, field string) (string, error) {
	return v.Client.HGet(ctx, key, field).Result()
}

func (v *ValkeyService) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return v.Client.HGetAll(ctx, key).Result()
}

func (v *ValkeyService) HDel(ctx context.Context, key string, fields ...string) error {
	return v.Client.HDel(ctx, key, fields...).Err()
}

func (v *ValkeyService) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return v.Client.Eval(ctx, script, keys, args...).Result()
}

func (v *ValkeyService) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return v.Client.Expire(ctx, key, expiration).Err()
}

func (v *ValkeyService) TTL(ctx context.Context, key string) (time.Duration, error) {
	return v.Client.TTL(ctx, key).Result()
}

func (v *ValkeyService) Delete(ctx context.Context, key string) error {
	return v.Client.Del(ctx, key).Err()
}

// ExecuteWithBackoff executes a Redis operation with exponential backoff
func (v *ValkeyService) ExecuteWithBackoff(ctx context.Context, operation func() error) error {
	backoffStrategy := backoffv4.NewExponentialBackOff()
	backoffStrategy.InitialInterval = 10 * time.Millisecond
	backoffStrategy.MaxInterval = 100 * time.Millisecond
	backoffStrategy.MaxElapsedTime = 500 * time.Millisecond

	return backoffv4.Retry(operation, backoffStrategy)
}
