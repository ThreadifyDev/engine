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

func NewValkeyService(host string, port int, password string, db int) (*ValkeyService, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:            fmt.Sprintf("%s:%d", host, port),
		Password:        password,
		DB:              db,
		PoolSize:        50,                     // Max concurrent connections
		MinIdleConns:    10,                     // Reduced to allow more active connections
		MaxRetries:      1,                      // Fail fast
		DialTimeout:     2 * time.Second,        // Connection establishment timeout
		ReadTimeout:     500 * time.Millisecond, // Read operation timeout
		WriteTimeout:    500 * time.Millisecond, // Write operation timeout
		PoolTimeout:     5 * time.Second,        // INCREASED: Wait up to 5s for connection from pool
		MaxIdleConns:    100,                    // Max idle connections to keep
		ConnMaxIdleTime: 5 * time.Minute,        // How long idle connections stay alive
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

// XAdd adds an entry to a stream
func (v *ValkeyService) XAdd(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}

	// Check for MAXLEN in values
	if maxlen, ok := values["maxlen"]; ok {
		if limit, ok := values["limit"]; ok {
			args.MaxLen = int64(limit.(int))
			args.Approx = maxlen.(string) == "~"
		}
		delete(values, "maxlen")
		delete(values, "limit")
	}

	return v.Client.XAdd(ctx, args).Result()
}

// XReadGroup reads from a stream using consumer groups
func (v *ValkeyService) XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]map[string]interface{}, error) {
	args := &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    int64(count),
		Block:    block,
	}

	results, err := v.Client.XReadGroup(ctx, args).Result()
	if err != nil {
		if err == redis.Nil {
			return []map[string]interface{}{}, nil // No messages
		}
		return nil, err
	}

	events := make([]map[string]interface{}, 0)
	for _, stream := range results {
		for _, message := range stream.Messages {
			event := make(map[string]interface{})
			event["id"] = message.ID
			for k, v := range message.Values {
				event[k] = v
			}
			events = append(events, event)
		}
	}

	return events, nil
}

// XAck acknowledges stream messages
func (v *ValkeyService) XAck(ctx context.Context, stream, group string, ids []string) error {
	return v.Client.XAck(ctx, stream, group, ids...).Err()
}

// XGroupCreate creates a consumer group for a stream
func (v *ValkeyService) XGroupCreate(ctx context.Context, stream, group, start string) error {
	return v.Client.XGroupCreate(ctx, stream, group, start).Err()
}

// XGroupCreateMkStream creates a consumer group and stream if it doesn't exist
func (v *ValkeyService) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	return v.Client.XGroupCreateMkStream(ctx, stream, group, start).Err()
}

// XTrim trims a stream to a maximum length
func (v *ValkeyService) XTrim(ctx context.Context, stream, strategy string, approx bool, count int64) error {
	if strategy == "MAXLEN" {
		if approx {
			return v.Client.XTrimMaxLenApprox(ctx, stream, count, 0).Err()
		}
		return v.Client.XTrimMaxLen(ctx, stream, count).Err()
	}
	// For MINID strategy, would need different implementation
	return v.Client.XTrimMaxLenApprox(ctx, stream, count, 0).Err()
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

func (p *RedisPipeline) XAdd(ctx context.Context, stream string, values map[string]interface{}) interfaces.ValkeyPipeline {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}

	// Check for MAXLEN in values
	if maxlen, ok := values["maxlen"]; ok {
		if limit, ok := values["limit"]; ok {
			args.MaxLen = int64(limit.(int))
			args.Approx = maxlen.(string) == "~"
		}
		delete(values, "maxlen")
		delete(values, "limit")
	}

	p.pipe.XAdd(ctx, args)
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
