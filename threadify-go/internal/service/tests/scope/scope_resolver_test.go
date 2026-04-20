package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"github.com/threadify/engine/internal/service/tests/common"
	"go.uber.org/zap"
)

var testScopeResolverCfg = &config.Config{
	NotificationSystem: config.NotificationSystemConfig{
		DefaultScope: "owner",
		Scopes: map[string]config.ScopeConfig{
			"owner":       {Permissions: []string{"*"}},
			"participant": {Permissions: []string{"read"}},
			"observer":    {Permissions: []string{"read_limited"}},
		},
	},
}

func TestScopeResolver_ValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		wantErr   bool
		wantIsErr error
	}{
		{
			name:      "missing default scope",
			cfg:       &config.Config{NotificationSystem: config.NotificationSystemConfig{Scopes: map[string]config.ScopeConfig{}}},
			wantErr:   true,
			wantIsErr: service.ErrConfigMissingDefaultScope,
		},
		{
			name: "default scope missing from scopes map",
			cfg: &config.Config{NotificationSystem: config.NotificationSystemConfig{
				DefaultScope: "owner",
				Scopes:       map[string]config.ScopeConfig{"other": {Permissions: []string{"a"}}},
			}},
			wantErr: true,
		},
		{
			name: "scope has no permissions",
			cfg: &config.Config{NotificationSystem: config.NotificationSystemConfig{
				DefaultScope: "owner",
				Scopes:       map[string]config.ScopeConfig{"owner": {}},
			}},
			wantErr: true,
		},
		{
			name:    "valid config",
			cfg:     testScopeResolverCfg,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := service.NewScopeResolver(tc.cfg, nil, nil, zap.NewNop())
			err := r.ValidateConfig()
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantIsErr != nil {
					require.ErrorIs(t, err, tc.wantIsErr)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

type scopeResolveCase struct {
	name string

	threadID      string
	userID        string
	role          string
	isCreator     bool
	explicitScope *string

	expectThreadCall bool
	thread           *models.Thread
	threadErr        error

	expectGraphCall bool
	contract        *models.ContractGraph
	contractErr     error

	want    string
	wantErr bool
}

func TestScopeResolver_ResolveScope_Creator(t *testing.T) {
	r := service.NewScopeResolver(testScopeResolverCfg, nil, nil, zap.NewNop())
	got, err := r.ResolveScope(context.Background(), "t1", "u1", "merchant", true, nil)
	require.NoError(t, err)
	require.Equal(t, "owner", got)
}

func TestScopeResolver_ResolveScope_ExplicitScope(t *testing.T) {
	tests := []struct {
		name          string
		explicitScope string
		want          string
		wantErr       bool
	}{
		{
			name:          "empty string means no scope",
			explicitScope: "",
			want:          "",
		},
		{
			name:          "unrecognised scope errors",
			explicitScope: "nope",
			wantErr:       true,
		},
		{
			name:          "valid scope is returned as-is",
			explicitScope: "observer",
			want:          "observer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := service.NewScopeResolver(testScopeResolverCfg, nil, nil, zap.NewNop())
			got, err := r.ResolveScope(context.Background(), "t1", "u1", "", false, common.Ptr(tc.explicitScope))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestScopeResolver_ResolveScope_Lookup(t *testing.T) {
	tests := []scopeResolveCase{
		{
			name:             "thread repo error falls back to system default",
			expectThreadCall: true,
			threadErr:        errors.New("db unavailable"),
			want:             "owner",
		},
		{
			name:             "thread has no contract — falls back to system default",
			expectThreadCall: true,
			thread:           &models.Thread{ID: "t1", CompanyID: "c1"},
			want:             "owner",
		},
		{
			name:             "contract repo error falls back to system default",
			expectThreadCall: true,
			thread:           &models.Thread{ID: "t1", CompanyID: "c1", ContractName: "cn", ContractVersion: common.Ptr(1)},
			expectGraphCall:  true,
			contractErr:      errors.New("db unavailable"),
			want:             "owner",
		},
		{
			name:             "role_default in contract is used when valid",
			role:             "merchant",
			expectThreadCall: true,
			thread:           &models.Thread{ID: "t1", CompanyID: "c1", ContractName: "cn", ContractVersion: common.Ptr(1)},
			expectGraphCall:  true,
			contract: &models.ContractGraph{NotificationConfig: &models.NotificationConfig{
				RoleDefaults: map[string]string{"merchant": "participant"},
			}},
			want: "participant",
		},
		{
			name:             "invalid role_default ignored — contract default used",
			role:             "merchant",
			expectThreadCall: true,
			thread:           &models.Thread{ID: "t1", CompanyID: "c1", ContractName: "cn", ContractVersion: common.Ptr(1)},
			expectGraphCall:  true,
			contract: &models.ContractGraph{NotificationConfig: &models.NotificationConfig{
				RoleDefaults: map[string]string{"merchant": "nope"},
				DefaultScope: "observer",
			}},
			want: "observer",
		},
		{
			name:             "invalid contract default ignored — system default used",
			role:             "merchant",
			expectThreadCall: true,
			thread:           &models.Thread{ID: "t1", CompanyID: "c1", ContractName: "cn", ContractVersion: common.Ptr(1)},
			expectGraphCall:  true,
			contract: &models.ContractGraph{NotificationConfig: &models.NotificationConfig{
				DefaultScope: "nope",
			}},
			want: "owner",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			threadRepo := enginemocks.NewMockThreadRepository(ctrl)
			graphRepo := enginemocks.NewMockContractGraphRepository(ctrl)

			if tc.expectThreadCall {
				threadRepo.EXPECT().
					Get(gomock.Any(), "t1").
					Return(tc.thread, tc.threadErr).
					Times(1)
			}
			if tc.expectGraphCall {
				graphRepo.EXPECT().
					Get(gomock.Any(), tc.thread.ContractName, *tc.thread.ContractVersion, tc.thread.CompanyID).
					Return(tc.contract, tc.contractErr).
					Times(1)
			}

			r := service.NewScopeResolver(testScopeResolverCfg, graphRepo, threadRepo, zap.NewNop())
			got, err := r.ResolveScope(context.Background(), "t1", "u1", tc.role, false, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
