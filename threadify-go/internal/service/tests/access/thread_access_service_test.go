package service_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

func TestThreadAccessService_GetUserRole_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := enginemocks.NewMockCacheManager(ctrl)
	cache.EXPECT().GetUserRole("t1", "u1").Return("owner", true).Times(1)

	svc := service.NewThreadAccessService(nil, cache, nil, nil, zap.NewNop())
	role, err := svc.GetUserRole(context.Background(), "t1", "u1")
	require.NoError(t, err)
	assert.Equal(t, "owner", role)
}

func TestThreadAccessService_CheckThreadAccess_OwnerShortcut(t *testing.T) {
	svc := service.NewThreadAccessService(nil, nil, nil, nil, zap.NewNop())
	thread := &models.Thread{ID: "t1", OwnerID: "u1"}

	got, err := svc.CheckThreadAccess(context.Background(), "t1", "u1", "thread.write.*", thread)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestThreadAccessService_BatchCheckThreadAccess_OwnedThreadsBypassRepo(t *testing.T) {
	svc := service.NewThreadAccessService(nil, nil, nil, nil, zap.NewNop())
	threads := []*models.Thread{
		{ID: "t1", OwnerID: "u1"},
		{ID: "t2", OwnerID: "u1"},
	}

	result, err := svc.BatchCheckThreadAccess(context.Background(), threads, "u1", "thread.write.*")
	require.NoError(t, err)
	assert.True(t, result["t1"])
	assert.True(t, result["t2"])
}

func TestThreadAccessService_ValidateUserRoleForStep_Table(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := enginemocks.NewMockCacheManager(ctrl)

	tests := []struct {
		name         string
		cachedRole   string
		requiredRole string
		want         bool
	}{
		{"matching role returns true", "merchant", "merchant", true},
		{"mismatched role returns false", "observer", "merchant", false},
		{"empty cached role returns false", "", "merchant", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache.EXPECT().GetUserRole("t1", "u1").Return(tc.cachedRole, true).Times(1)
			svc := service.NewThreadAccessService(nil, cache, nil, nil, zap.NewNop())
			got, err := svc.ValidateUserRoleForStep(context.Background(), "t1", "u1", tc.requiredRole)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestThreadAccessService_ClearThreadCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := enginemocks.NewMockCacheManager(ctrl)
	cache.EXPECT().ClearThreadRoles("t1").Times(1)

	svc := service.NewThreadAccessService(nil, cache, nil, nil, zap.NewNop())
	svc.ClearThreadCache("t1")
}

func TestThreadAccessService_GrantOrUpdateAccess_EmptyRoles(t *testing.T) {
	svc := service.NewThreadAccessService(nil, nil, nil, nil, zap.NewNop())
	err := svc.GrantOrUpdateAccess(context.Background(), "t1", "u1", nil, "owner", "self")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one role is required")
}
