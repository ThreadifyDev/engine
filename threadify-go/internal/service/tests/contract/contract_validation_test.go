package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

type fakeContractRepo struct {
	contract *models.Contract
	version  *models.ContractVersion
	getErr   error
	verErr   error
}

func (f fakeContractRepo) GetByNameAndCompany(_ context.Context, _, _ string) (*models.Contract, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.contract, nil
}

func (f fakeContractRepo) GetVersion(_ context.Context, _ string, _ int) (*models.ContractVersion, error) {
	if f.verErr != nil {
		return nil, f.verErr
	}
	return f.version, nil
}

func TestContractValidationService_ValidateStepContext_Table(t *testing.T) {
	svc := service.NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop())

	tests := []struct {
		name      string
		node      models.GraphNode
		ctx       map[string]string
		wantError bool
	}{
		{
			name: "no business context passes",
			node: models.GraphNode{ID: "s1"},
			ctx:  map[string]string{},
		},
		{
			name: "struct required missing errors",
			node: models.GraphNode{ID: "s1", BusinessContext: &models.BusinessContext{Required: []string{"a"}}},
			ctx:  map[string]string{},
			// wantError
			wantError: true,
		},
		{
			name: "struct required satisfied passes",
			node: models.GraphNode{ID: "s1", BusinessContext: &models.BusinessContext{Required: []string{"a"}}},
			ctx:  map[string]string{"a": "1"},
		},
		{
			name: "map required missing errors",
			node: models.GraphNode{ID: "s1", BusinessContext: map[string]interface{}{"required": []interface{}{"a"}}},
			ctx:  map[string]string{},
			// wantError
			wantError: true,
		},
		{
			name: "map required wrong type is ignored",
			node: models.GraphNode{ID: "s1", BusinessContext: map[string]interface{}{"required": "a"}},
			ctx:  map[string]string{},
		},
		{
			name: "unsupported business context type is ignored",
			node: models.GraphNode{ID: "s1", BusinessContext: 123},
			ctx:  map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.ValidateStepContext(tc.node, tc.ctx)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestContractValidationService_ValidateStepInContract_Table(t *testing.T) {
	tests := []struct {
		name      string
		graph     *models.ContractGraph
		step      string
		ctx       map[string]string
		wantError bool
	}{
		{
			name: "missing step errors",
			graph: &models.ContractGraph{Graph: models.Graph{
				Nodes: map[string]models.GraphNode{"s1": {ID: "s1"}},
			}},
			step:      "missing",
			wantError: true,
		},
		{
			name: "existing step validates context",
			graph: &models.ContractGraph{Graph: models.Graph{
				Nodes: map[string]models.GraphNode{"s1": {ID: "s1", BusinessContext: &models.BusinessContext{Required: []string{"a"}}}},
			}},
			step:      "s1",
			ctx:       map[string]string{"a": "1"},
			wantError: false,
		},
		{
			name: "existing step missing context errors",
			graph: &models.ContractGraph{Graph: models.Graph{
				Nodes: map[string]models.GraphNode{"s1": {ID: "s1", BusinessContext: &models.BusinessContext{Required: []string{"a"}}}},
			}},
			step:      "s1",
			ctx:       map[string]string{},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			graphRepo := enginemocks.NewMockContractGraphRepository(ctrl)
			cache := enginemocks.NewMockCacheManager(ctrl)

			cache.EXPECT().GetContractGraph("c1", 1, "o1").Return(tc.graph, true).Times(1)

			svc := service.NewContractValidationServiceFromParts(graphRepo, nil, cache, zap.NewNop())

			err := svc.ValidateStepInContract("c1", 1, tc.step, tc.ctx, "o1")
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestContractValidationService_GetContractGraph_Table(t *testing.T) {
	tests := []struct {
		name      string
		version   int
		setup     func(t *testing.T, graphRepo *enginemocks.MockContractGraphRepository, cache *enginemocks.MockCacheManager) *service.ContractValidationService
		wantError bool
	}{
		{
			name:    "memory cache hit returns immediately",
			version: 1,
			setup: func(t *testing.T, graphRepo *enginemocks.MockContractGraphRepository, cache *enginemocks.MockCacheManager) *service.ContractValidationService {
				t.Helper()
				cached := &models.ContractGraph{Graph: models.Graph{Nodes: map[string]models.GraphNode{}}}
				cache.EXPECT().GetContractGraph("c1", 1, "o1").Return(cached, true).Times(1)
				return service.NewContractValidationServiceFromParts(graphRepo, nil, cache, zap.NewNop())
			},
		},
		{
			name:    "valkey hit populates memory cache",
			version: 1,
			setup: func(t *testing.T, graphRepo *enginemocks.MockContractGraphRepository, cache *enginemocks.MockCacheManager) *service.ContractValidationService {
				t.Helper()
				graph := &models.ContractGraph{Graph: models.Graph{Nodes: map[string]models.GraphNode{"s1": {ID: "s1"}}}}
				cache.EXPECT().GetContractGraph("c1", 1, "o1").Return(nil, false).Times(1)
				graphRepo.EXPECT().Get(gomock.Any(), "c1", 1, "o1").Return(graph, nil).Times(1)
				cache.EXPECT().SetContractGraph("c1", 1, "o1", graph).Times(1)
				return service.NewContractValidationServiceFromParts(graphRepo, nil, cache, zap.NewNop())
			},
		},
		{
			name:      "valkey miss and no postgres repo errors",
			version:   1,
			wantError: true,
			setup: func(t *testing.T, graphRepo *enginemocks.MockContractGraphRepository, cache *enginemocks.MockCacheManager) *service.ContractValidationService {
				t.Helper()
				cache.EXPECT().GetContractGraph("c1", 1, "o1").Return(nil, false).Times(1)
				graphRepo.EXPECT().Get(gomock.Any(), "c1", 1, "o1").Return(nil, errors.New("not found")).Times(1)
				return service.NewContractValidationServiceFromParts(graphRepo, nil, cache, zap.NewNop())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			graphRepo := enginemocks.NewMockContractGraphRepository(ctrl)
			cache := enginemocks.NewMockCacheManager(ctrl)
			svc := tc.setup(t, graphRepo, cache)

			_, err := svc.GetContractGraph("c1", tc.version, "o1")
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestContractValidationService_GetContractGraph_ResolvesLatestAndFallsBackToPostgres(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graphRepo := enginemocks.NewMockContractGraphRepository(ctrl)
	cache := enginemocks.NewMockCacheManager(ctrl)

	cache.EXPECT().GetContractGraph("c1", 7, "o1").Return(nil, false).Times(1)
	graphRepo.EXPECT().Get(gomock.Any(), "c1", 7, "o1").Return(nil, errors.New("not found")).Times(1)

	graph := &models.ContractGraph{Graph: models.Graph{Nodes: map[string]models.GraphNode{"s1": {ID: "s1"}}}}
	graphJSON, err := json.Marshal(graph)
	require.NoError(t, err)

	graphRepo.EXPECT().Save(gomock.Any(), "c1", 7, "o1", gomock.Any()).Return(nil).Times(1)
	cache.EXPECT().SetContractGraph("c1", 7, "o1", gomock.Any()).Times(1)

	svc := service.NewContractValidationServiceFromParts(
		graphRepo,
		fakeContractRepo{contract: &models.Contract{ID: "cid", LatestVersion: 7}, version: &models.ContractVersion{Graph: graphJSON}},
		cache,
		zap.NewNop(),
	)

	got, err := svc.GetContractGraph("c1", 0, "o1")
	require.NoError(t, err)
	require.Contains(t, got.Graph.Nodes, "s1")
}

func TestContractValidationService_GetContractGraph_PostgresEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		version   *models.ContractVersion
		wantError string
	}{
		{
			name:      "empty graph errors",
			version:   &models.ContractVersion{Graph: nil},
			wantError: "no graph found",
		},
		{
			name:      "invalid graph json errors",
			version:   &models.ContractVersion{Graph: []byte("{bad-json")},
			wantError: "failed to parse contract graph",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			graphRepo := enginemocks.NewMockContractGraphRepository(ctrl)
			cache := enginemocks.NewMockCacheManager(ctrl)

			cache.EXPECT().GetContractGraph("c1", 7, "o1").Return(nil, false).Times(1)
			graphRepo.EXPECT().Get(gomock.Any(), "c1", 7, "o1").Return(nil, errors.New("not found")).Times(1)

			svc := service.NewContractValidationServiceFromParts(
				graphRepo,
				fakeContractRepo{contract: &models.Contract{ID: "cid", LatestVersion: 7}, version: tc.version},
				cache,
				zap.NewNop(),
			)

			_, err := svc.GetContractGraph("c1", 0, "o1")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantError)
		})
	}
}

func TestContractValidationService_LoadContractGraphIntoCache_ResolvesVersionAndShortCircuitsIfCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	graphRepo := enginemocks.NewMockContractGraphRepository(ctrl)
	cache := enginemocks.NewMockCacheManager(ctrl)

	cached := &models.ContractGraph{Graph: models.Graph{Nodes: map[string]models.GraphNode{}}}
	cache.EXPECT().GetContractGraph("c1", 7, "o1").Return(cached, true).Times(1)

	svc := service.NewContractValidationServiceFromParts(
		graphRepo,
		fakeContractRepo{contract: &models.Contract{ID: "cid", LatestVersion: 7}},
		cache,
		zap.NewNop(),
	)

	v, err := svc.LoadContractGraphIntoCache("c1", 0, "o1")
	require.NoError(t, err)
	require.Equal(t, 7, v)
}

func TestContractValidationService_GetContractByNameAndCompany(t *testing.T) {
	t.Run("nil repo", func(t *testing.T) {
		svc := service.NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop())
		_, err := svc.GetContractByNameAndCompany("c1", "o1")
		require.ErrorIs(t, err, service.ErrContractRepoNotAvailable)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		svc := service.NewContractValidationServiceFromParts(nil, fakeContractRepo{getErr: errors.New("db down")}, nil, zap.NewNop())
		_, err := svc.GetContractByNameAndCompany("c1", "o1")
		require.Error(t, err)
	})
}
