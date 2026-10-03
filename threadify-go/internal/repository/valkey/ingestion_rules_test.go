package valkey

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"threadify-go/shared/ingestion"
)

func TestIngestionRulesSharedPersistenceAndCAS(t *testing.T) {
	addr := os.Getenv("THREADIFY_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("requires disposable Valkey")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	company := uuid.NewString()
	defer client.Del(ctx, ingestionRulesKey(company))
	a := NewIngestionRules(&database.ValkeyService{Client: client})
	first, err := a.Load(ctx, company)
	require.NoError(t, err)
	require.Equal(t, []string{"*"}, first.Filters)
	require.Equal(t, ingestion.ModeInclude, first.Mode)
	require.Empty(t, first.Revision)
	saved, err := a.Save(ctx, company, first.Revision, []string{"healthcheck"})
	require.NoError(t, err)
	require.NoError(t, a.Record(ctx, company, 10, 3))
	// A fresh connection represents another replica or a restarted Engine.
	client2 := redis.NewClient(&redis.Options{Addr: addr})
	defer client2.Close()
	b := NewIngestionRules(&database.ValkeyService{Client: client2})
	loaded, err := b.Load(ctx, company)
	require.NoError(t, err)
	require.Equal(t, saved.Filters, loaded.Filters)
	require.Equal(t, ingestion.ModeInclude, loaded.Mode)
	require.Equal(t, saved.Revision, loaded.Revision)
	require.EqualValues(t, 10, loaded.EvaluatedSpans)
	require.EqualValues(t, 3, loaded.DroppedSpans)
	ttl, err := client.TTL(ctx, ingestionRulesKey(company)).Result()
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), ttl)
	other, err := b.Load(ctx, company+"-other")
	require.NoError(t, err)
	require.Equal(t, []string{"*"}, other.Filters)
	// Only one editor can save against a revision, even on separate replicas.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, store := range []*IngestionRules{a, b} {
		wg.Add(1)
		go func(store *IngestionRules) {
			defer wg.Done()
			_, e := store.Save(ctx, company, saved.Revision, []string{"internal.*"})
			results <- e
		}(store)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, ingestion.ErrConflict)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	loaded, err = b.Load(ctx, company)
	require.NoError(t, err)
	cleared, err := b.Save(ctx, company, loaded.Revision, []string{})
	require.NoError(t, err)
	require.Empty(t, cleared.Filters)
	require.Equal(t, ingestion.ModeInclude, cleared.Mode)
	require.EqualValues(t, 3, cleared.DroppedSpans)
	// Pre-migration hashes keep their exclusion semantics until explicitly replaced.
	require.NoError(t, client.HSet(ctx, ingestionRulesKey(company), "filters", `["healthcheck"]`).Err())
	require.NoError(t, client.HDel(ctx, ingestionRulesKey(company), "mode").Err())
	legacy, err := a.Load(ctx, company)
	require.NoError(t, err)
	require.Equal(t, ingestion.ModeExcludeLegacy, legacy.Mode)
	require.Equal(t, []string{"healthcheck"}, legacy.Filters)
	migrated, err := a.Save(ctx, company, legacy.Revision, []string{"*"})
	require.NoError(t, err)
	require.Equal(t, ingestion.ModeInclude, migrated.Mode)
	_, err = a.Save(ctx, company, migrated.Revision, []string{"bad*pattern"})
	require.Error(t, err)
	require.NoError(t, client.HSet(ctx, ingestionRulesKey(company), "filters", "broken-json").Err())
	_, err = a.Load(ctx, company)
	require.Error(t, err)
	require.NoError(t, client2.Close())
	_, err = b.Load(ctx, company)
	require.Error(t, err)
}
