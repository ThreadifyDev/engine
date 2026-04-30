package apikey

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/service/tests/common"
	"threadify-go/shared/rbac"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestRBACLoader(t *testing.T) *rbac.Loader {
	permissionsJSON := `{
		"app_level": [],
		"runtime_level": []
	}`
	rolesJSON := `{
		"app_level": {},
		"api_level": {
			"standard_service": {
				"name": "standard_service",
				"permissions": ["api.read"]
			}
		},
		"runtime_level": {}
	}`

	tmpDir := t.TempDir()
	pPath := filepath.Join(tmpDir, "permissions.json")
	rPath := filepath.Join(tmpDir, "roles.json")

	require.NoError(t, os.WriteFile(pPath, []byte(permissionsJSON), 0644))
	require.NoError(t, os.WriteFile(rPath, []byte(rolesJSON), 0644))

	loader, err := rbac.NewLoader(pPath, rPath)
	require.NoError(t, err)
	return loader
}

func TestAPIKeyService_CreateAPIKey(t *testing.T) {
	const (
		userID    = "user_123"
		companyID = "comp_123"
	)

	loader := createTestRBACLoader(t)

	tests := []struct {
		name      string
		req       *domain.CreateAPIKeyCmd
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success_with_existing_sa",
			req: &domain.CreateAPIKeyCmd{
				Name:             "Test Key",
				ServiceAccountID: strPtr("sa_123"),
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.ServiceAccountRepo.EXPECT().FindByID(gomock.Any(), "sa_123").Return(&domain.ServiceAccount{ID: "sa_123", CompanyID: companyID}, nil)
				deps.APIKeyRepoInternal.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "invalid_sa_role",
			req: &domain.CreateAPIKeyCmd{
				Name:                 "Invalid Role Key",
				CreateServiceAccount: true,
				ServiceAccountRole:   strPtr("invalid_role"),
			},
			setupMock: func(deps *common.MockedDeps) {},
			wantErr:   true,
		},
		{
			name: "sa_not_found",
			req: &domain.CreateAPIKeyCmd{
				Name:             "Missing SA",
				ServiceAccountID: strPtr("non_existent"),
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.ServiceAccountRepo.EXPECT().FindByID(gomock.Any(), "non_existent").Return(nil, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAPIKeyService(loader)

			tt.setupMock(deps)

			resp, err := svc.CreateAPIKey(context.Background(), userID, companyID, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
			}
		})
	}
}

func TestAPIKeyService_RevokeAPIKey(t *testing.T) {
	const (
		companyID = "comp_123"
		keyID     = "key_123"
	)

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.APIKeyRepoInternal.EXPECT().FindByID(gomock.Any(), keyID).Return(&domain.APIKey{ID: keyID, CompanyID: companyID}, nil)
				deps.APIKeyRepoInternal.EXPECT().Revoke(gomock.Any(), keyID).Return(nil)
			},
		},
		{
			name: "not_found",
			setupMock: func(deps *common.MockedDeps) {
				deps.APIKeyRepoInternal.EXPECT().FindByID(gomock.Any(), keyID).Return(nil, nil)
			},
			wantErr: true,
		},
		{
			name: "unauthorized",
			setupMock: func(deps *common.MockedDeps) {
				deps.APIKeyRepoInternal.EXPECT().FindByID(gomock.Any(), keyID).Return(&domain.APIKey{ID: keyID, CompanyID: "other"}, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAPIKeyService(nil)
			tt.setupMock(deps)

			err := svc.RevokeAPIKey(context.Background(), keyID, companyID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAPIKeyService_ValidateAPIKey(t *testing.T) {
	const key = "test_key"

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "revoked_key",
			setupMock: func(deps *common.MockedDeps) {
				now := time.Now()
				deps.APIKeyRepoInternal.EXPECT().FindByHash(gomock.Any(), gomock.Any()).Return(&domain.APIKey{RevokedAt: &now}, nil)
			},
			wantErr: true,
		},
		{
			name: "expired_key",
			setupMock: func(deps *common.MockedDeps) {
				past := time.Now().Add(-time.Hour)
				deps.APIKeyRepoInternal.EXPECT().FindByHash(gomock.Any(), gomock.Any()).Return(&domain.APIKey{ExpiresAt: &past}, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAPIKeyService(nil)
			tt.setupMock(deps)

			res, err := svc.ValidateAPIKey(context.Background(), key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, res)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
