package entityprofile

import (
	"context"
	"testing"

	threadifymodels "threadify-go/api/internal/models"
	"threadify-go/api/internal/service/tests/common"
	sharedmodels "threadify-go/shared/models"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityProfileTypeService_ListEntityProfileTypes(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().GetProfileTypesByCompanyID(gomock.Any(), companyID).Return([]*sharedmodels.EntityProfileType{
					{ID: "1", Name: "Type 1"},
				}, nil)
			},
		},
		{
			name: "error",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().GetProfileTypesByCompanyID(gomock.Any(), companyID).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()
			tt.setupMock(deps)

			res, err := svc.ListEntityProfileTypes(context.Background(), companyID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, res, 1)
			}
		})
	}
}

func TestEntityProfileTypeService_ArchiveEntityProfileType(t *testing.T) {
	const (
		companyID = "comp_123"
		typeID    = "type_123"
	)

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().ArchiveProfileType(gomock.Any(), companyID, typeID).Return(nil)
			},
		},
		{
			name: "error",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().ArchiveProfileType(gomock.Any(), companyID, typeID).Return(assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()
			tt.setupMock(deps)

			err := svc.ArchiveEntityProfileType(context.Background(), companyID, typeID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEntityProfileTypeService_CreateEntityProfileType(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name      string
		req       *threadifymodels.CreateEntityProfileTypeRequest
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			req: &threadifymodels.CreateEntityProfileTypeRequest{
				Name:        "Test Profile",
				Type:        "test",
				Description: "A test profile type",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "error",
			req: &threadifymodels.CreateEntityProfileTypeRequest{
				Name: "Error Profile",
				Type: "error",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()
			tt.setupMock(deps)

			res, err := svc.CreateEntityProfileType(context.Background(), companyID, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, res)
				assert.Equal(t, tt.req.Name, res.Name)
				assert.Equal(t, companyID, res.CompanyID)
			}
		})
	}
}
