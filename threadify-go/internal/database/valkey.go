package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/pkg/lua"
)

type ValkeyService struct {
	client *redis.Client
	ctx    context.Context
}

func NewValkeyService(host string, port int, password string, db int) (*ValkeyService, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &ValkeyService{client: client, ctx: ctx}, nil
}

func (v *ValkeyService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return v.client.Set(ctx, key, value, ttl).Err()
}

func (v *ValkeyService) Get(ctx context.Context, key string) (string, error) {
	return v.client.Get(ctx, key).Result()
}

func (v *ValkeyService) Delete(ctx context.Context, key string) error {
	return v.client.Del(ctx, key).Err()
}

func (v *ValkeyService) Exists(ctx context.Context, key string) (bool, error) {
	result, err := v.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

func (v *ValkeyService) Keys(ctx context.Context, pattern string) ([]string, error) {
	return v.client.Keys(ctx, pattern).Result()
}

func (v *ValkeyService) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return v.client.Expire(ctx, key, ttl).Err()
}

func (v *ValkeyService) SetIfNotExists(key, value string) (bool, error) {
	result, err := v.client.Eval(v.ctx, lua.SetIfNotExists, []string{key}, value).Result()
	if err != nil {
		return false, err
	}
	return result.(int64) == 1, nil
}

func (v *ValkeyService) GetAndDelete(key string) (string, error) {
	result, err := v.client.Eval(v.ctx, lua.GetAndDelete, []string{key}).Result()
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", redis.Nil
	}
	return result.(string), nil
}

func (v *ValkeyService) Upsert(key, value string) (string, error) {
	result, err := v.client.Eval(v.ctx, lua.Upsert, []string{key}, value).Result()
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func (v *ValkeyService) CompareAndSwap(key, expected, newValue string) (bool, error) {
	result, err := v.client.Eval(v.ctx, lua.CompareAndSwap, []string{key}, expected, newValue).Result()
	if err != nil {
		return false, err
	}
	return result.(int64) == 1, nil
}

func (v *ValkeyService) Close() error {
	return v.client.Close()
}
