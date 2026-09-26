package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/pkg/validator"
)

const (
	testUserID     = "user_123"
	testCompanyID  = "comp_456"
	testContractID = "contract_789"
)

func TestContractHandler_GetAllContracts(t *testing.T) {
	tests := []struct {
		name       string
		authIDs    AuthIDs
		setupMock  func(d *MockedEngineHandlers)
		wantStatus int
	}{
		{
			name:    "success",
			authIDs: AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					GetAllContracts(gomock.Any(), testCompanyID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(http.StatusOK, map[string]interface{}{"contracts": []interface{}{}})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unauthorized_no_user_id",
			authIDs:    AuthIDs{CompanyID: testCompanyID},
			setupMock:  func(d *MockedEngineHandlers) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:    "service_error",
			authIDs: AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					GetAllContracts(gomock.Any(), testCompanyID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(http.StatusInternalServerError, map[string]string{"error": "internal error"})
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			h := NewContractHandler(d.ContractSvc, d.Logger)
			r := SetupTestRouter()
			r.GET("/contracts", WithAuthContext(tt.authIDs), h.GetAllContracts)

			w := DoRequest(t, r, http.MethodGet, "/contracts", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestContractHandler_CreateContract(t *testing.T) {
	validSource := "Feature: test_contract\nVersion: 1"

	tests := []struct {
		name       string
		authIDs    AuthIDs
		body       string
		setupMock  func(d *MockedEngineHandlers)
		wantStatus int
	}{
		{
			name:    "success",
			authIDs: AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			body:    validSource,
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					CreateContract(gomock.Any(), testUserID, testCompanyID, testUserID, gomock.Any()).
					Return(http.StatusOK, map[string]string{"id": testContractID})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_company_id",
			authIDs:    AuthIDs{UserID: testUserID},
			body:       validSource,
			setupMock:  func(d *MockedEngineHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:    "service_error",
			authIDs: AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			body:    validSource,
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					CreateContract(gomock.Any(), testUserID, testCompanyID, testUserID, gomock.Any()).
					Return(http.StatusInternalServerError, map[string]string{"error": "internal error"})
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			h := NewContractHandler(d.ContractSvc, d.Logger)
			r := SetupTestRouter()
			r.POST("/contracts", WithAuthContext(tt.authIDs), h.CreateContract)

			w := DoRequest(t, r, http.MethodPost, "/contracts", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestContractHandler_GetContract(t *testing.T) {
	tests := []struct {
		name       string
		contractID string
		authIDs    AuthIDs
		setupMock  func(d *MockedEngineHandlers)
		wantStatus int
	}{
		{
			name:       "success",
			contractID: testContractID,
			authIDs:    AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					GetContract(gomock.Any(), testContractID, testCompanyID, nil).
					Return(http.StatusOK, map[string]string{"id": testContractID})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			contractID: "missing_123",
			authIDs:    AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					GetContract(gomock.Any(), "missing_123", testCompanyID, nil).
					Return(http.StatusNotFound, map[string]string{"error": "not found"})
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "service_error",
			contractID: testContractID,
			authIDs:    AuthIDs{UserID: testUserID, CompanyID: testCompanyID},
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					GetContract(gomock.Any(), testContractID, testCompanyID, nil).
					Return(http.StatusInternalServerError, map[string]string{"error": "internal error"})
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			h := NewContractHandler(d.ContractSvc, d.Logger)
			r := SetupTestRouter()
			r.GET("/contracts/:id", WithAuthContext(tt.authIDs), h.GetContract)

			w := DoRequest(t, r, http.MethodGet, "/contracts/"+tt.contractID, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestContractHandler_PreviewContract(t *testing.T) {
	validSource := "Feature: test_contract"

	tests := []struct {
		name        string
		body        string
		setupMock   func(d *MockedEngineHandlers)
		wantStatus  int
		wantValid   bool
		wantWarning bool
	}{
		{
			name: "success_valid",
			body: validSource,
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					PreviewContract(gomock.Any(), "company", gomock.Any()).
					Return(nil, nil, &validator.ValidationResult{IsValid: true}, nil)
			},
			wantStatus: http.StatusOK,
			wantValid:  true,
		},
		{
			name: "semantic_warning",
			body: validSource,
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					PreviewContract(gomock.Any(), "company", gomock.Any()).
					Return(nil, nil, &validator.ValidationResult{IsValid: true, Warnings: []string{"Possible semantic conflict"}}, nil)
			},
			wantStatus:  http.StatusOK,
			wantValid:   true,
			wantWarning: true,
		},
		{
			name: "success_invalid_contract",
			body: "bad-source",
			setupMock: func(d *MockedEngineHandlers) {
				d.ContractSvc.EXPECT().
					PreviewContract(gomock.Any(), "company", gomock.Any()).
					Return(nil, nil, &validator.ValidationResult{IsValid: false, Errors: []validator.ValidationError{{Message: "bad"}}}, nil)
			},
			wantStatus: http.StatusOK,
			wantValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			h := NewContractHandler(d.ContractSvc, d.Logger)
			r := SetupTestRouter()
			r.POST("/preview", WithAuthContext(AuthIDs{UserID: "user", CompanyID: "company"}), h.PreviewContract)

			w := DoRequest(t, r, http.MethodPost, "/preview", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)

			var resp struct {
				Valid    bool     `json:"valid"`
				Warnings []string `json:"warnings"`
			}
			json.Unmarshal(w.Body.Bytes(), &resp)
			assert.Equal(t, tt.wantValid, resp.Valid)
			assert.Equal(t, tt.wantWarning, len(resp.Warnings) > 0)
		})
	}
}
