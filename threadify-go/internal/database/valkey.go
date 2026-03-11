package database

import (
	"context"
	"fmt"
	"time"

	backoffv4 "github.com/cenkalti/backoff/v4"
	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/interfaces"
)

type ValkeyService struct {
	Client *redis.Client
}

func NewValkeyService(host string, port int, password string, db int, poolSize int, minIdleConns int, maxIdleConns int, maxRetries int, dialTimeoutMs int, readTimeoutMs int, writeTimeoutMs int, poolTimeoutMs int, connMaxIdleTimeMs int) (*ValkeyService, error) {
	// Apply defaults if values are 0
	if poolSize == 0 {
		poolSize = 50
	}
	if minIdleConns == 0 {
		minIdleConns = 10
	}
	if maxIdleConns == 0 {
		maxIdleConns = 100
	}
	if maxRetries == 0 {
		maxRetries = 1
	}
	if dialTimeoutMs == 0 {
		dialTimeoutMs = 2000
	}
	if readTimeoutMs == 0 {
		readTimeoutMs = 500
	}
	if writeTimeoutMs == 0 {
		writeTimeoutMs = 500
	}
	if poolTimeoutMs == 0 {
		poolTimeoutMs = 5000
	}
	if connMaxIdleTimeMs == 0 {
		connMaxIdleTimeMs = 300000
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:            fmt.Sprintf("%s:%d", host, port),
		Password:        password,
		DB:              db,
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		MaxRetries:      maxRetries,
		DialTimeout:     time.Duration(dialTimeoutMs) * time.Millisecond,
		ReadTimeout:     time.Duration(readTimeoutMs) * time.Millisecond,
		WriteTimeout:    time.Duration(writeTimeoutMs) * time.Millisecond,
		PoolTimeout:     time.Duration(poolTimeoutMs) * time.Millisecond,
		MaxIdleConns:    maxIdleConns,
		ConnMaxIdleTime: time.Duration(connMaxIdleTimeMs) * time.Millisecond,
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

// Enqueue adds an item to a Redis queue with TTL
func (v *ValkeyService) Enqueue(ctx context.Context, queueKey, item string, ttl time.Duration) error {
	pipe := v.Client.Pipeline()
	pipe.LPush(ctx, queueKey, item)
	if ttl > 0 {
		pipe.Expire(ctx, queueKey, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (v *ValkeyService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return v.Client.Set(ctx, key, value, ttl).Err()
}

func (v *ValkeyService) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return v.Client.SetNX(ctx, key, value, ttl).Result()
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

func (v *ValkeyService) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	keys, nextCursor, err := v.Client.Scan(ctx, cursor, match, count).Result()
	return keys, nextCursor, err
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

// LPush adds items to the left of a Redis list
func (v *ValkeyService) LPush(ctx context.Context, key string, values ...interface{}) error {
	return v.Client.LPush(ctx, key, values...).Err()
}

// LRange gets a range of items from a Redis list
func (v *ValkeyService) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return v.Client.LRange(ctx, key, start, stop).Result()
}

// ZAdd adds a member with score to a sorted set
func (v *ValkeyService) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return v.Client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

// ZRem removes members from a sorted set
func (v *ValkeyService) ZRem(ctx context.Context, key string, members ...string) error {
	// Convert []string to []interface{} for variadic parameter
	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}
	return v.Client.ZRem(ctx, key, args...).Err()
}

// ZCard returns the number of members in a sorted set
func (v *ValkeyService) ZCard(ctx context.Context, key string) (int64, error) {
	return v.Client.ZCard(ctx, key).Result()
}

// ZRange returns members in a sorted set by index range
func (v *ValkeyService) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return v.Client.ZRange(ctx, key, start, stop).Result()
}

// Pipeline creates a new Redis pipeline
func (v *ValkeyService) Pipeline() interfaces.ValkeyPipeline {
	return &RedisPipeline{pipe: v.Client.Pipeline()}
}

// ScriptLoad loads a Lua script into Redis and returns its SHA1 hash
func (v *ValkeyService) ScriptLoad(ctx context.Context, script string) (string, error) {
	cmd := v.Client.ScriptLoad(ctx, script)
	return cmd.Result()
}

// EvalSHA executes a Lua script by its SHA1 hash
func (v *ValkeyService) EvalSHA(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error) {
	cmd := v.Client.EvalSha(ctx, sha, keys, args...)
	return cmd.Result()
}

// ExecuteWithBackoff executes a Redis operation with exponential backoff
func (v *ValkeyService) ExecuteWithBackoff(ctx context.Context, operation func() error) error {
	backoffStrategy := backoffv4.NewExponentialBackOff()
	backoffStrategy.InitialInterval = 10 * time.Millisecond
	backoffStrategy.MaxInterval = 100 * time.Millisecond
	backoffStrategy.MaxElapsedTime = 500 * time.Millisecond

	return backoffv4.Retry(operation, backoffStrategy)
}

// DecrBy atomically decrements a key by the given value, returns new value
func (v *ValkeyService) DecrBy(ctx context.Context, key string, value int64) (int64, error) {
	return v.Client.DecrBy(ctx, key, value).Result()
}

// IncrBy atomically increments a key by the given value, returns new value
func (v *ValkeyService) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return v.Client.IncrBy(ctx, key, value).Result()
}

// SAdd adds members to a SET
func (v *ValkeyService) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return v.Client.SAdd(ctx, key, members...).Err()
}

// SMembers returns all members of a SET
func (v *ValkeyService) SMembers(ctx context.Context, key string) ([]string, error) {
	return v.Client.SMembers(ctx, key).Result()
}

// SRem removes members from a SET
func (v *ValkeyService) SRem(ctx context.Context, key string, members ...interface{}) error {
	return v.Client.SRem(ctx, key, members...).Err()
}

// XAdd adds an entry to a stream
func (v *ValkeyService) XAdd(ctx context.Context, stream string, id string, values interface{}) (string, error) {
	return v.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     id,
		Values: values,
	}).Result()
}

// RedisPipeline implements the ValkeyPipeline interface
type RedisPipeline struct {
	pipe redis.Pipeliner
}

func (p *RedisPipeline) HSet(ctx context.Context, key string, values ...interface{}) interfaces.ValkeyPipeline {
	p.pipe.HSet(ctx, key, values...)
	return p
}

func (p *RedisPipeline) HDel(ctx context.Context, key string, fields ...string) interfaces.ValkeyPipeline {
	p.pipe.HDel(ctx, key, fields...)
	return p
}

func (p *RedisPipeline) LPush(ctx context.Context, key string, values ...interface{}) interfaces.ValkeyPipeline {
	p.pipe.LPush(ctx, key, values...)
	return p
}

func (p *RedisPipeline) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) interfaces.ValkeyPipeline {
	p.pipe.Set(ctx, key, value, expiration)
	return p
}

func (p *RedisPipeline) Del(ctx context.Context, keys ...string) interfaces.ValkeyPipeline {
	p.pipe.Del(ctx, keys...)
	return p
}

func (p *RedisPipeline) Expire(ctx context.Context, key string, expiration time.Duration) interfaces.ValkeyPipeline {
	p.pipe.Expire(ctx, key, expiration)
	return p
}

func (p *RedisPipeline) Exec(ctx context.Context) ([]interface{}, error) {
	// Convert []redis.Cmder to []interface{}
	cmders, err := p.pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(cmders))
	for i, cmd := range cmders {
		result[i] = cmd
	}
	return result, nil
}
