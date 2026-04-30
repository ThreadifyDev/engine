package entityprofile

import (
	"context"
	"strings"
	"testing"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/service/tests/common"
	shareddomain "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// normalizeTypes is a copy of the service's normalizeTypes function for testing
func normalizeTypes(types []string) []string {
	seen := make(map[string]struct{}, len(types))
	out := make([]string, 0, len(types))

	for _, t := range types {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}

	return out
}

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
				deps.EntityProfileTypeRepo.EXPECT().GetProfileTypesByCompanyID(gomock.Any(), companyID).Return([]*shareddomain.EntityProfileType{
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
		req       *domain.CreateEntityProfileTypeCmd
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			req: &domain.CreateEntityProfileTypeCmd{
				Name:        "Test Profile",
				Type:        []string{"test", "test_alt"},
				Description: "A test profile type",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "error",
			req: &domain.CreateEntityProfileTypeCmd{
				Name: "Error Profile",
				Type: []string{"error"},
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

func TestEntityProfileTypeService_UpdateEntityProfileType_AddTypes(t *testing.T) {
	const (
		companyID = "comp_123"
		typeID    = "type_123"
	)

	deps := common.NewMockDeps(t)
	svc := deps.NewEntityProfileTypeService()

	req := &domain.UpdateEntityProfileTypeCmd{
		Name: "Updated Name",
		Type: []string{"vip", "partner"},
	}

	deps.EntityProfileTypeRepo.EXPECT().
		UpdateProfileType(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, pt *shareddomain.EntityProfileType) error {
			assert.Equal(t, typeID, pt.ID)
			assert.Equal(t, companyID, pt.CompanyID)
			assert.Equal(t, "Updated Name", pt.Name)
			assert.Equal(t, []string{"vip", "partner"}, pt.Type)
			pt.Type = []string{"customer", "partner", "vip"}
			return nil
		})

	res, err := svc.UpdateEntityProfileType(context.Background(), companyID, typeID, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, []string{"customer", "partner", "vip"}, res.Type)
}

func TestEntityProfileTypeService_CreateEntityProfileType_MaxTypesEdgeCases(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name      string
		req       *domain.CreateEntityProfileTypeCmd
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "exactly max types - success",
			req: &domain.CreateEntityProfileTypeCmd{
				Name:        "Max Types Profile",
				Type:        []string{"type1", "type2", "type3", "type4", "type5"},
				Description: "Profile with exactly max types",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "exceeds max types - should pass validation at service layer",
			req: &domain.CreateEntityProfileTypeCmd{
				Name:        "Too Many Types Profile",
				Type:        []string{"type1", "type2", "type3", "type4", "type5", "type6"},
				Description: "Profile with too many types",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "duplicate types within max limit - success",
			req: &domain.CreateEntityProfileTypeCmd{
				Name:        "Duplicate Types Profile",
				Type:        []string{"type1", "type2", "type2", "type3", "type1"},
				Description: "Profile with duplicate types",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "empty and whitespace types filtered - success",
			req: &domain.CreateEntityProfileTypeCmd{
				Name:        "Whitespace Types Profile",
				Type:        []string{"type1", "", "  ", "type2", "\ttype3\t"},
				Description: "Profile with empty and whitespace types",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "no types - should pass validation at service layer",
			req: &domain.CreateEntityProfileTypeCmd{
				Name:        "No Types Profile",
				Type:        []string{"", "  ", "\t"},
				Description: "Profile with no valid types",
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()
			tt.setupMock(deps)

			res, err := svc.CreateEntityProfileType(context.Background(), companyID, tt.req)
			require.NoError(t, err)
			assert.NotNil(t, res)
			assert.Equal(t, tt.req.Name, res.Name)
			assert.Equal(t, companyID, res.CompanyID)
			// Verify type normalization
			expectedTypes := normalizeTypes(tt.req.Type)
			assert.Equal(t, expectedTypes, res.Type)
		})
	}
}

func TestEntityProfileTypeService_UpdateEntityProfileType_MaxTypesEdgeCases(t *testing.T) {
	const (
		companyID = "comp_123"
		typeID    = "type_123"
	)

	tests := []struct {
		name      string
		req       *domain.UpdateEntityProfileTypeCmd
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "exactly max types - success",
			req: &domain.UpdateEntityProfileTypeCmd{
				Name: "Updated Max Types",
				Type: []string{"type1", "type2", "type3", "type4", "type5"},
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().UpdateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "exceeds max types - should pass validation at service layer",
			req: &domain.UpdateEntityProfileTypeCmd{
				Name: "Too Many Types",
				Type: []string{"type1", "type2", "type3", "type4", "type5", "type6"},
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().UpdateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "duplicate types within max limit - success",
			req: &domain.UpdateEntityProfileTypeCmd{
				Name: "Duplicate Types Update",
				Type: []string{"vip", "partner", "vip", "customer"},
			},
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().UpdateProfileType(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()
			tt.setupMock(deps)

			res, err := svc.UpdateEntityProfileType(context.Background(), companyID, typeID, tt.req)
			require.NoError(t, err)
			assert.NotNil(t, res)
			assert.Equal(t, tt.req.Name, res.Name)
			expectedTypes := normalizeTypes(tt.req.Type)
			assert.Equal(t, expectedTypes, res.Type)
		})
	}
}

func TestEntityProfileTypeService_TypeNormalization(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "normal types",
			input:    []string{"customer", "vip", "partner"},
			expected: []string{"customer", "vip", "partner"},
		},
		{
			name:     "with whitespace and empty strings",
			input:    []string{"customer", "", "  ", "\tvip\t", "partner"},
			expected: []string{"customer", "vip", "partner"},
		},
		{
			name:     "duplicate types",
			input:    []string{"customer", "vip", "customer", "partner", "vip"},
			expected: []string{"customer", "vip", "partner"},
		},
		{
			name:     "mixed case duplicates",
			input:    []string{"Customer", "customer", "VIP", "vip"},
			expected: []string{"Customer", "customer", "VIP", "vip"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()

			req := &domain.CreateEntityProfileTypeCmd{
				Name:        "Test Profile",
				Type:        tt.input,
				Description: "Test description",
			}

			deps.EntityProfileTypeRepo.EXPECT().CreateProfileType(gomock.Any(), gomock.Any()).Return(nil)

			res, err := svc.CreateEntityProfileType(context.Background(), companyID, req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, res.Type)
		})
	}
}
func TestEntityProfileTypeService_ListMetricsTemplates(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(deps *common.MockedDeps)
		wantErr     bool
		expectedLen int
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().ListMetricsTemplates(gomock.Any()).Return([]*shareddomain.MetricsTemplate{
					{ID: "1", MetricsName: "Metric 1", Parameters: []string{"param1"}},
				}, nil)
			},
			expectedLen: 1,
		},
		{
			name: "error",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().ListMetricsTemplates(gomock.Any()).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
		{
			name: "empty results",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().ListMetricsTemplates(gomock.Any()).Return([]*shareddomain.MetricsTemplate{}, nil)
			},
			expectedLen: 0,
		},
		{
			name: "multiple templates",
			setupMock: func(deps *common.MockedDeps) {
				deps.EntityProfileTypeRepo.EXPECT().ListMetricsTemplates(gomock.Any()).Return([]*shareddomain.MetricsTemplate{
					{ID: "1", MetricsName: "Metric 1", Parameters: []string{"param1"}},
					{ID: "2", MetricsName: "Metric 2", Parameters: []string{"param1", "param2"}},
				}, nil)
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewEntityProfileTypeService()
			tt.setupMock(deps)

			res, err := svc.ListMetricsTemplates(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, res, tt.expectedLen)
				if tt.expectedLen > 0 {
					assert.Equal(t, "Metric 1", res[0].MetricsName)
				}
			}
		})
	}
}
