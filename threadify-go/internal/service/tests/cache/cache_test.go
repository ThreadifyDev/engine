package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
)

func TestCacheService_ClearThreadRoles_OnlyClearsPrefix(t *testing.T) {
	cache := service.NewCacheService(zap.NewNop())

	cache.SetUserRole("t1", "u1", "owner")
	cache.SetUserRole("t1", "u2", "participant")
	cache.SetUserRole("t2", "u1", "observer")

	cache.ClearThreadRoles("t1")

	_, ok := cache.GetUserRole("t1", "u1")
	require.False(t, ok)
	_, ok = cache.GetUserRole("t1", "u2")
	require.False(t, ok)

	role, ok := cache.GetUserRole("t2", "u1")
	require.True(t, ok)
	require.Equal(t, "observer", role)
}

func TestCacheService_ContractGraph_RoundTripAndClear(t *testing.T) {
	cache := service.NewCacheService(zap.NewNop())

	graph := &domain.ContractGraph{}
	cache.SetContractGraph("contract-1", 1, "owner-1", graph)

	got, ok := cache.GetContractGraph("contract-1", 1, "owner-1")
	require.True(t, ok)
	require.Equal(t, graph, got)

	cache.ClearContractCache("contract-1", 1, "owner-1")
	_, ok = cache.GetContractGraph("contract-1", 1, "owner-1")
	require.False(t, ok)
}

func TestCacheService_StepStatus_RoundTripAndClear(t *testing.T) {
	cache := service.NewCacheService(zap.NewNop())

	cache.SetStepStatus("k1", "started")
	v, ok := cache.GetStepStatus("k1")
	require.True(t, ok)
	require.Equal(t, "started", v)

	cache.ClearStepStatus("k1")
	_, ok = cache.GetStepStatus("k1")
	require.False(t, ok)
}
