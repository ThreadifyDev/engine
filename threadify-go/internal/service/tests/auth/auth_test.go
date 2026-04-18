package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
)

func TestAuthService_ValidateApiKey(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (*service.AuthService, func())
		key      string
		wantErr  error
		wantInfo *service.UserInfo
	}{
		{
			name: "nil repo returns ErrDatabaseNotConfigured",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				svc := service.NewAuthService(nil, 1)
				return svc, svc.Stop
			},
			key:     "k1",
			wantErr: service.ErrDatabaseNotConfigured,
		},
		{
			name: "repo returns nil result returns ErrApiKeyInvalidOrInactive",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				ctrl := gomock.NewController(t)
				repo := enginemocks.NewMockAuthRepository(ctrl)
				svc := service.NewAuthService(repo, 60)
				repo.EXPECT().
					ValidateAPIKey(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)
				return svc, func() { svc.Stop(); ctrl.Finish() }
			},
			key:     "k1",
			wantErr: service.ErrApiKeyInvalidOrInactive,
		},
		{
			name: "expired key returns ErrApiKeyExpired",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				ctrl := gomock.NewController(t)
				repo := enginemocks.NewMockAuthRepository(ctrl)
				svc := service.NewAuthService(repo, 60)
				expired := time.Now().Add(-time.Minute)
				repo.EXPECT().
					ValidateAPIKey(gomock.Any(), gomock.Any()).
					Return(&models.AuthInfo{OwnerID: "o1", CompanyID: "c1", Role: "r1", ExpiresAt: &expired}, nil).
					Times(1)
				return svc, func() { svc.Stop(); ctrl.Finish() }
			},
			key:     "k1",
			wantErr: service.ErrApiKeyExpired,
		},
		{
			name: "valid key returns UserInfo and caches result",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				ctrl := gomock.NewController(t)
				repo := enginemocks.NewMockAuthRepository(ctrl)
				svc := service.NewAuthService(repo, 60)
				repo.EXPECT().
					ValidateAPIKey(gomock.Any(), gomock.Any()).
					Return(&models.AuthInfo{OwnerID: "o1", CompanyID: "c1", Role: "r1"}, nil).
					Times(1)
				return svc, func() { svc.Stop(); ctrl.Finish() }
			},
			key:      "k1",
			wantInfo: &service.UserInfo{OwnerID: "o1", CompanyID: "c1", Role: "r1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, cleanup := tc.setup(t)
			defer cleanup()

			info, err := svc.ValidateApiKey(tc.key)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantInfo, info)

			// second call must be served from cache (mock expects Times(1))
			info2, err := svc.ValidateApiKey(tc.key)
			require.NoError(t, err)
			require.Equal(t, info, info2)
		})
	}
}

func TestAuthService_ValidateApiKey_SingleflightDedupesConcurrentRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := enginemocks.NewMockAuthRepository(ctrl)
	svc := service.NewAuthService(repo, 60)
	defer svc.Stop()

	var calls atomic.Int32
	release := make(chan struct{})

	repo.EXPECT().
		ValidateAPIKey(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string) (*models.AuthInfo, error) {
			calls.Add(1)
			<-release
			return &models.AuthInfo{OwnerID: "o1", CompanyID: "c1", Role: "r1"}, nil
		}).
		Times(1)

	var wg sync.WaitGroup
	wg.Add(2)

	var got1, got2 *service.UserInfo
	var err1, err2 error

	go func() { defer wg.Done(); got1, err1 = svc.ValidateApiKey("k1") }()
	go func() { defer wg.Done(); got2, err2 = svc.ValidateApiKey("k1") }()

	require.Eventually(t, func() bool { return calls.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
	close(release)
	wg.Wait()

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, got1, got2)
}

func TestAuthService_VerifyToken_RequiresVerifier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc := service.NewAuthService(enginemocks.NewMockAuthRepository(ctrl), 60)
	defer svc.Stop()

	_, err := svc.VerifyToken(context.Background(), "token")
	require.ErrorIs(t, err, service.ErrJwtVerificationNotConfigured)
}

func TestAuthService_GetUserRoles(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (*service.AuthService, func())
		wantErr error
	}{
		{
			name: "nil repo returns ErrDatabaseNotConfigured",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				svc := service.NewAuthService(nil, 1)
				return svc, svc.Stop
			},
			wantErr: service.ErrDatabaseNotConfigured,
		},
		{
			name: "repo error is returned",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				ctrl := gomock.NewController(t)
				repo := enginemocks.NewMockAuthRepository(ctrl)
				svc := service.NewAuthService(repo, 60)
				repo.EXPECT().
					GetUserRoles(gomock.Any(), "p1", "user").
					Return(nil, errors.New("db down")).
					Times(1)
				return svc, func() { svc.Stop(); ctrl.Finish() }
			},
			wantErr: errors.New("db down"),
		},
		{
			name: "valid result is cached and capped to jwt expiry",
			setup: func(t *testing.T) (*service.AuthService, func()) {
				ctrl := gomock.NewController(t)
				repo := enginemocks.NewMockAuthRepository(ctrl)
				svc := service.NewAuthService(repo, 60)
				repo.EXPECT().
					GetUserRoles(gomock.Any(), "p1", "user").
					Return([]string{"read"}, nil).
					Times(1)
				return svc, func() { svc.Stop(); ctrl.Finish() }
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, cleanup := tc.setup(t)
			defer cleanup()

			ctx := context.Background()
			jwtExpiry := time.Now().Add(50 * time.Millisecond)

			roles, err := svc.GetUserRoles(ctx, "p1", "user", jwtExpiry)
			if tc.wantErr != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"read"}, roles)

			roles2, err := svc.GetUserRoles(ctx, "p1", "user", jwtExpiry)
			require.NoError(t, err)
			require.Equal(t, roles, roles2)
		})
	}
}
