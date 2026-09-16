package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	shderrors "threadify-go/shared/errors"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/service/tests/common"
	"github.com/threadify/engine/pkg/validator"
)

func calculateContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestContractService_CreateContract(t *testing.T) {
	const (
		ownerID   = "owner-1"
		companyID = "company-1"
		yaml      = "contract_name: test-contract\nversion: 1\ndescription: test"
	)
	contract := &validator.Contract{
		ContractName: "test-contract",
		Version:      1,
		Description:  "test",
	}

	t.Run("successful creation with credit check and charge", func(t *testing.T) {
		deps := common.NewMockDeps(t)
		defer deps.Ctrl.Finish()

		ctx := context.Background()
		svc := deps.NewContractService()

		deps.Validator.EXPECT().Validate(yaml).Return(contract, &validator.ValidationResult{IsValid: true})
		deps.PlanSvc.EXPECT().CheckCreditAvailable(ctx, companyID, service.MeterContractExecution, int64(1)).Return(nil)
		deps.Validator.EXPECT().SerializeContract(contract).Return("{}", "{}", nil)
		deps.ContractRepo.EXPECT().CreateContractWithVersion(ctx, gomock.Any(), gomock.Any()).Return(nil)
		deps.PlanSvc.EXPECT().ChargeContract(ctx, companyID).Return(nil)

		status, _ := svc.CreateContract(ctx, ownerID, companyID, ownerID, yaml)

		require.Equal(t, 200, status)
	})

	t.Run("fails when insufficient credit", func(t *testing.T) {
		deps := common.NewMockDeps(t)
		defer deps.Ctrl.Finish()

		ctx := context.Background()
		svc := deps.NewContractService()

		deps.Validator.EXPECT().Validate(yaml).Return(contract, &validator.ValidationResult{IsValid: true})
		deps.PlanSvc.EXPECT().CheckCreditAvailable(ctx, companyID, service.MeterContractExecution, int64(1)).Return(service.ErrInsufficientCredit)

		status, resp := svc.CreateContract(ctx, ownerID, companyID, ownerID, yaml)

		require.Equal(t, 402, status)
		require.Contains(t, resp.(map[string]string)["message"], "insufficient credit balance")
	})
}

func TestContractService_PreviewContract_Table(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		validateOK  bool
		wantErr     bool
		wantInvalid bool
	}{
		{
			name:        "invalid contract returns validation result without error",
			yaml:        "bad",
			validateOK:  false,
			wantErr:     false,
			wantInvalid: true,
		},
		{
			name:        "graph build failure returns error",
			yaml:        "{bad-json",
			validateOK:  true,
			wantErr:     true,
			wantInvalid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()

			svc := deps.NewContractService()

			result := &validator.ValidationResult{IsValid: tc.validateOK}
			deps.Validator.EXPECT().Validate(tc.yaml).Return(&validator.Contract{ContractName: "c", Version: 1}, result)

			contract, graph, vr, err := svc.PreviewContract(tc.yaml)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, vr)
				return
			}
			require.Equal(t, result, vr)
			require.NoError(t, err)

			if tc.wantInvalid {
				require.Nil(t, contract)
				require.Nil(t, graph)
				return
			}
			require.NotNil(t, contract)
			require.Nil(t, vr.Errors)
		})
	}
}

func TestContractService_UpdateContract_Table(t *testing.T) {
	const (
		contractID = "cid"
		ownerID    = "o1"
		createdBy  = "o1"
		companyID  = "c1"
	)

	baseExisting := func(latest int, contentHash *string) *domain.Contract {
		return &domain.Contract{
			ID:            contractID,
			CompanyID:     companyID,
			OwnerID:       ownerID,
			LatestVersion: latest,
			ContentHash:   contentHash,
		}
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, deps *common.MockedDependencies, yaml string)
		yaml  string
		want  int
	}{
		{
			name: "not found or not owner",
			yaml: "yaml",
			want: 404,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(nil, errors.New("nope"))
			},
		},
		{
			name: "invalid contract yaml",
			yaml: "yaml",
			want: 400,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2}, &validator.ValidationResult{IsValid: false, Errors: []validator.ValidationError{{Message: "bad"}}})
			},
		},
		{
			name: "version not greater than latest",
			yaml: "yaml",
			want: 400,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(2, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
			},
		},
		{
			name: "insufficient credit when updating",
			yaml: "yaml",
			want: 402,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(service.ErrInsufficientCredit)
			},
		},
		{
			name: "serialize fails",
			yaml: "yaml",
			want: 500,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
				deps.Validator.EXPECT().SerializeContract(gomock.Any()).Return("", "", errors.New("boom"))
			},
		},
		{
			name: "content unchanged",
			yaml: "yaml",
			want: 400,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				unchanged := calculateContentHash(`{"a":"b"}`)
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, &unchanged), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
				deps.Validator.EXPECT().SerializeContract(gomock.Any()).Return("{}", `{"a":"b"}`, nil)
			},
		},
		{
			name: "graph build fails",
			yaml: "yaml",
			want: 500,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
				deps.Validator.EXPECT().SerializeContract(gomock.Any()).Return("{}", "{bad-json", nil)
				deps.ContractRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(baseExisting(2, nil), nil)
			},
		},
		{
			name: "charge failure does not undo persisted version",
			yaml: "yaml",
			want: 200,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2, Description: "d"}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
				deps.Validator.EXPECT().SerializeContract(gomock.Any()).Return("{}", "{}", nil)
				deps.ContractRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(baseExisting(2, nil), nil)
				// Charging happens after persistence; a billing failure is logged.
				gomock.InOrder(
					deps.ContractRepo.EXPECT().CreateVersion(gomock.Any(), gomock.Any()).Return(nil),
					deps.PlanSvc.EXPECT().ChargeContractVersion(gomock.Any(), companyID).Return(errors.New("nope")),
				)
			},
		},
		{
			name: "create version fails",
			yaml: "yaml",
			want: 500,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2, Description: "d"}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
				deps.Validator.EXPECT().SerializeContract(gomock.Any()).Return("{}", "{}", nil)
				deps.ContractRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(baseExisting(2, nil), nil)
				deps.ContractRepo.EXPECT().CreateVersion(gomock.Any(), gomock.Any()).Return(errors.New("db"))
			},
		},
		{
			name: "success",
			yaml: "yaml",
			want: 200,
			setup: func(t *testing.T, deps *common.MockedDependencies, yaml string) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), contractID, companyID).Return(baseExisting(1, nil), nil)
				deps.Validator.EXPECT().Validate(yaml).Return(&validator.Contract{Version: 2, Description: "d"}, &validator.ValidationResult{IsValid: true})
				deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), companyID, service.MeterContractExecution, int64(1)).Return(nil)
				deps.Validator.EXPECT().SerializeContract(gomock.Any()).Return("{}", "{}", nil)
				deps.ContractRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(baseExisting(2, nil), nil)
				gomock.InOrder(
					deps.ContractRepo.EXPECT().CreateVersion(gomock.Any(), gomock.Any()).Return(nil),
					deps.PlanSvc.EXPECT().ChargeContractVersion(gomock.Any(), companyID).Return(nil),
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()

			svc := deps.NewContractService()
			tc.setup(t, deps, tc.yaml)

			status, _ := svc.UpdateContract(context.Background(), contractID, companyID, createdBy, tc.yaml)
			require.Equal(t, tc.want, status)
		})
	}
}

func TestContractService_GetContract_AccessControlAndVersionLookup(t *testing.T) {
	tests := []struct {
		name    string
		isOwner bool
		public  bool
		version *int
		want    int
	}{
		{name: "private non-owner denied", isOwner: false, public: false, want: 403},
		{name: "public non-owner allowed latest", isOwner: false, public: true, want: 200},
		{name: "owner allowed specific version", isOwner: true, public: false, version: common.Ptr(2), want: 200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()

			reqID := "req"
			if tc.isOwner {
				reqID = "o1"
			}

			contract := &domain.Contract{ID: "cid", CompanyID: "o1", OwnerID: "creator", IsPublic: tc.public}
			deps.ContractRepo.EXPECT().GetByID(gomock.Any(), "cid").Return(contract, nil)

			if tc.want == 200 {
				if tc.version != nil {
					deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", *tc.version).Return(&domain.ContractVersion{Version: *tc.version}, nil)
				} else {
					deps.ContractRepo.EXPECT().GetLatestVersion(gomock.Any(), "cid").Return(&domain.ContractVersion{Version: 9}, nil)
				}
			}

			svc := deps.NewContractService()
			status, _ := svc.GetContract(context.Background(), "cid", reqID, tc.version)
			require.Equal(t, tc.want, status)
		})
	}
}

func TestContractService_DeleteContract_Table(t *testing.T) {
	tests := []struct {
		name  string
		setup func(deps *common.MockedDependencies)
		want  int
	}{
		{
			name: "not found",
			want: 404,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(nil, errors.New("nope"))
			},
		},
		{
			name: "already deleted",
			want: 400,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{IsDeleted: true}, nil)
			},
		},
		{
			name: "soft delete error",
			want: 500,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{}, nil)
				deps.ContractRepo.EXPECT().SoftDelete(gomock.Any(), "cid").Return(errors.New("db"))
			},
		},
		{
			name: "success",
			want: 200,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{}, nil)
				deps.ContractRepo.EXPECT().SoftDelete(gomock.Any(), "cid").Return(nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()
			tc.setup(deps)
			svc := deps.NewContractService()
			status, _ := svc.DeleteContract(context.Background(), "cid", "o1")
			require.Equal(t, tc.want, status)
		})
	}
}

func TestContractService_GetContractVersion_GraphParseFallback(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	contract := &domain.Contract{ID: "cid", CompanyID: "o1", OwnerID: "creator", IsPublic: true, Name: "c"}
	deps.ContractRepo.EXPECT().GetByID(gomock.Any(), "cid").Return(contract, nil)

	// invalid json in graph field -> mapper passes it through as raw message
	deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", 1).Return(&domain.ContractVersion{Graph: []byte("{bad-json")}, nil)

	svc := deps.NewContractService()
	status, resp := svc.GetContractVersion(context.Background(), "cid", 1, "x")
	require.Equal(t, 200, status)
	versionDTO, ok := resp.(*dto.ContractVersion)
	require.True(t, ok)
	require.Equal(t, "c", versionDTO.ContractName)
	require.Equal(t, json.RawMessage("{bad-json"), versionDTO.Graph)
}

func TestContractService_GetContractVersion_ParsesGraphAndShapesResponse(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	contract := &domain.Contract{ID: "cid", CompanyID: "o1", OwnerID: "creator", IsPublic: true, Name: "c"}
	deps.ContractRepo.EXPECT().GetByID(gomock.Any(), "cid").Return(contract, nil)

	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{"s1": {ID: "s1"}}}}
	graphJSON, err := json.Marshal(graph)
	require.NoError(t, err)

	deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", 1).Return(&domain.ContractVersion{ID: "v1", Version: 1, ContractID: "cid", Graph: graphJSON}, nil)

	svc := deps.NewContractService()
	status, resp := svc.GetContractVersion(context.Background(), "cid", 1, "x")
	require.Equal(t, 200, status)
	versionDTO, ok := resp.(*dto.ContractVersion)
	require.True(t, ok)
	require.Equal(t, "v1", versionDTO.ID)
	require.Equal(t, "c", versionDTO.ContractName)
	require.NotNil(t, versionDTO.Graph)
}

func TestContractService_DeleteContractVersion_Table(t *testing.T) {
	tests := []struct {
		name  string
		setup func(deps *common.MockedDependencies)
		want  int
	}{
		{
			name: "not owner",
			want: 404,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(nil, shderrors.ErrContractNotFound)
			},
		},
		{
			name: "version not found",
			want: 404,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{}, nil)
				deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", 1).Return(nil, shderrors.ErrContractNotFound)
			},
		},
		{
			name: "already deleted",
			want: 400,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{}, nil)
				deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", 1).Return(&domain.ContractVersion{IsDeleted: true}, nil)
			},
		},
		{
			name: "soft delete fails",
			want: 500,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{}, nil)
				deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", 1).Return(&domain.ContractVersion{}, nil)
				deps.ContractRepo.EXPECT().SoftDeleteVersion(gomock.Any(), "cid", 1).Return(errors.New("db"))
			},
		},
		{
			name: "success",
			want: 200,
			setup: func(deps *common.MockedDependencies) {
				deps.ContractRepo.EXPECT().GetByIDAndCompany(gomock.Any(), "cid", "o1").Return(&domain.Contract{}, nil)
				deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "cid", 1).Return(&domain.ContractVersion{ID: "v1", Version: 1, ContractID: "cid"}, nil)
				deps.ContractRepo.EXPECT().SoftDeleteVersion(gomock.Any(), "cid", 1).Return(nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()
			tc.setup(deps)
			svc := deps.NewContractService()
			status, _ := svc.DeleteContractVersion(context.Background(), "cid", 1, "o1")
			require.Equal(t, tc.want, status)
		})
	}
}

func TestPrivateContractRejectsOtherCompany(t *testing.T) {
	for _, operation := range []string{"contract", "versions", "version"} {
		t.Run(operation, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()
			deps.ContractRepo.EXPECT().GetByID(gomock.Any(), "cid").Return(&domain.Contract{ID: "cid", CompanyID: "company-a", OwnerID: "creator"}, nil)
			svc := deps.NewContractService()
			var status int
			switch operation {
			case "contract":
				status, _ = svc.GetContract(context.Background(), "cid", "company-b", nil)
			case "versions":
				status, _ = svc.GetAllContractVersions(context.Background(), "cid", "company-b", 20, 0)
			case "version":
				status, _ = svc.GetContractVersion(context.Background(), "cid", 1, "company-b")
			}
			require.Equal(t, 403, status)
		})
	}
}
