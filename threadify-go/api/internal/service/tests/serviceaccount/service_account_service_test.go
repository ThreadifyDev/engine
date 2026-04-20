package serviceaccount

import (
	"context"
	"testing"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/service/tests/common"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountService_CreateServiceAccount(t *testing.T) {
	const (
		companyID = "comp_123"
		userID    = "user_123"
	)

	tests := []struct {
		name      string
		req       *service.CreateServiceAccountRequest
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			req: &service.CreateServiceAccountRequest{
				Name: "Test SA",
				Role: "standard_service",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.ServiceAccountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
				deps.UserRoleRepo.EXPECT().AssignRoleToServiceAccount(gomock.Any(), gomock.Any(), "standard_service", userID).Return(nil)
			},
		},
		{
			name: "invalid_role",
			req: &service.CreateServiceAccountRequest{
				Name: "Evil SA",
				Role: "admin",
			},
			setupMock: func(deps *common.MockedDeps) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewServiceAccountService()
			tt.setupMock(deps)

			res, err := svc.CreateServiceAccount(context.Background(), companyID, userID, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, res)
			}
		})
	}
}

func TestServiceAccountService_GetServiceAccount(t *testing.T) {
	const (
		companyID = "comp_123"
		saID      = "sa_123"
	)

	tests := []struct {
		name      string
		id        string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			id:   saID,
			setupMock: func(deps *common.MockedDeps) {
				deps.ServiceAccountRepo.EXPECT().FindByID(gomock.Any(), saID).Return(&models.ServiceAccount{ID: saID, CompanyID: companyID}, nil)
			},
		},
		{
			name: "not_found",
			id:   saID,
			setupMock: func(deps *common.MockedDeps) {
				deps.ServiceAccountRepo.EXPECT().FindByID(gomock.Any(), saID).Return(nil, nil)
			},
			wantErr: true,
		},
		{
			name: "unauthorized_cross_company",
			id:   saID,
			setupMock: func(deps *common.MockedDeps) {
				deps.ServiceAccountRepo.EXPECT().FindByID(gomock.Any(), saID).Return(&models.ServiceAccount{ID: saID, CompanyID: "other_comp"}, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewServiceAccountService()
			tt.setupMock(deps)

			res, err := svc.GetServiceAccount(context.Background(), tt.id, companyID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, res)
			}
		})
	}
}
