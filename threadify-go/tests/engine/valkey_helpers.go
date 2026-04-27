package engine

import (
	"context"
	"strconv"
	"testing"
	"threadify-go/shared/billing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
)

type valkeyHelpers struct {
	svc *database.ValkeyService
}

func newValkeyHelpers(svc *database.ValkeyService) *valkeyHelpers {
	return &valkeyHelpers{svc: svc}
}

func (v *valkeyHelpers) GetCreditBalance(t *testing.T, companyID string) int64 {
	t.Helper()
	keys := billing.KeysFor(companyID)
	val, err := v.svc.Get(context.Background(), keys.Balance)
	if err == redis.Nil {
		return 0
	}
	require.NoError(t, err, "valkey balance key must exist for company %s", companyID)
	balance, err := strconv.ParseInt(val, 10, 64)
	require.NoError(t, err, "valkey balance key must be an integer for company %s", companyID)
	return balance
}
