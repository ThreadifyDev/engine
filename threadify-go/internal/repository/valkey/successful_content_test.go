package valkey

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type successCache func(context.Context, string, string) (string, error)

func (f successCache) HGet(ctx context.Context, key, field string) (string, error) {
	return f(ctx, key, field)
}

func TestSuccessfulContentCacheScopeAndErrors(t *testing.T) {
	repo := NewSuccessfulContentRepository(successCache(func(_ context.Context, key, field string) (string, error) {
		require.Equal(t, "thread:thread-a:successful_contexts", key)
		require.Equal(t, "shipment", field)
		return `{"stepID":"s1","order":"1789999999999999","context":"{\"tracking\":\"T1\",\"raw\":\"1.00\"}"}`, nil
	}), nil)
	got, err := repo.GetSuccessfulContent(context.Background(), "thread-a", "shipment")
	require.NoError(t, err)
	require.Equal(t, "1.00", got["raw"])
	for _, tc := range []struct {
		raw string
		err error
	}{{"", redis.Nil}, {"", errors.New("down")}, {`{"stepID":"s","order":"0","context":"{}"}`, nil}, {`{"stepID":"s","order":"1","context":"not json"}`, nil}} {
		repo := NewSuccessfulContentRepository(successCache(func(context.Context, string, string) (string, error) { return tc.raw, tc.err }), nil)
		_, err := repo.GetSuccessfulContent(context.Background(), "thread-a", "shipment")
		require.Error(t, err)
	}
}
