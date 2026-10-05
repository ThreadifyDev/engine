package valkey

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"os"
	"testing"
	"threadify-go/shared/actionmapping"
)

func TestActionMappingRegexPersistence(t *testing.T) {
	addr := os.Getenv("THREADIFY_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("requires disposable Valkey")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	store := NewBrowserActionMappings(&database.ValkeyService{Client: client})
	current, err := store.Load(ctx, "company")
	require.NoError(t, err)
	defer client.Del(ctx, browserActionMappingsKey())
	rules := []actionmapping.Rule{
		{Action: `regex:(?i)^POST /orders/[0-9]{1,3}\?state=paid$`, Contract: "order", Version: 1, Step: "paid"},
		{Action: `regex:(?i)^POST /orders`, Contract: "order", Version: 1, Step: "checkout"},
	}
	saved, err := store.Save(ctx, "company", current.Revision, rules)
	require.NoError(t, err)
	loaded, err := store.Load(ctx, "company")
	require.NoError(t, err)
	require.Equal(t, saved.Revision, loaded.Revision)
	require.Equal(t, rules[0].Action, loaded.Rules[0].Action)
	require.Equal(t, rules[1].Action, loaded.Rules[1].Action)
	match, ok := actionmapping.Match(loaded.Rules, "order", 1, "post /orders/123?state=paid")
	require.True(t, ok)
	require.Equal(t, "paid", match.Step)
	_, err = store.Save(ctx, "company", saved.Revision, []actionmapping.Rule{{Action: "regex:[", Contract: "order", Version: 1, Step: "paid"}})
	require.ErrorContains(t, err, "invalid input mapping regex")
	unchanged, err := store.Load(ctx, "company")
	require.NoError(t, err)
	require.Equal(t, saved.Revision, unchanged.Revision)
	_, err = store.Save(ctx, "company", current.Revision, rules)
	require.ErrorIs(t, err, actionmapping.ErrConflict)
}
