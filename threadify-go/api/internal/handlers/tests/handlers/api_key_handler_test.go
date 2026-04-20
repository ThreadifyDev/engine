package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	"threadify-go/api/internal/models"
	apiservice "threadify-go/api/internal/service"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func newAPIKeyRouter(deps *common.MockedHandlers, companyID, userID string) *gin.Engine {
	h := handlers.NewAPIKeyHandler(deps.APIKeySvc, deps.UserRepo)
	r := common.SetupTestRouter()
	r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
	r.POST("/api-keys", h.CreateAPIKey)
	r.GET("/api-keys", h.ListAPIKeys)
	r.DELETE("/api-keys/:id", h.RevokeAPIKey)
	return r
}

func TestAPIKeyHandler_CreateAPIKey(t *testing.T) {
	const (
		companyID = "comp_123"
		userID    = "user_1"
	)

	tests := []struct {
		name       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]string{"name": "production-key", "service_account_id": "sa_123"},
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					CreateAPIKey(gomock.Any(), userID, companyID, gomock.Any()).
					Return(&apiservice.CreateAPIKeyResponse{
						APIKey: &models.APIKey{ID: "key_1"},
						Key:    "th_abc123",
					}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing_name",
			body:       map[string]string{"service_account_id": "sa_123"},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "key_limit_reached",
			body: map[string]string{"name": "new-key"},
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					CreateAPIKey(gomock.Any(), userID, companyID, gomock.Any()).
					Return(nil, serror.NewDomainError("Key limit reached", http.StatusForbidden))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "service_error",
			body: map[string]string{"name": "key"},
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					CreateAPIKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("db failure"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newAPIKeyRouter(deps, companyID, userID), "POST", "/api-keys", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAPIKeyHandler_ListAPIKeys(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					ListAPIKeys(gomock.Any(), companyID).
					Return([]*models.APIKey{{ID: "key_1", Name: "Key 1"}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service_error",
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					ListAPIKeys(gomock.Any(), companyID).
					Return(nil, errors.New("db disconnect"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "forbidden_access",
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					ListAPIKeys(gomock.Any(), companyID).
					Return(nil, serror.NewDomainError("Access denied", http.StatusForbidden))
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newAPIKeyRouter(deps, companyID, "user_1"), "GET", "/api-keys", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAPIKeyHandler_RevokeAPIKey(t *testing.T) {
	const (
		companyID = "comp_123"
		keyID     = "key_456"
	)

	tests := []struct {
		name       string
		keyID      string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name:  "success",
			keyID: keyID,
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().
					RevokeAPIKey(gomock.Any(), keyID, companyID).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:  "not_found",
			keyID: keyID,
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().RevokeAPIKey(gomock.Any(), keyID, companyID).
					Return(serror.NewDomainError("Key not found", http.StatusNotFound))
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:  "service_error",
			keyID: keyID,
			setupMock: func(d *common.MockedHandlers) {
				d.APIKeySvc.EXPECT().RevokeAPIKey(gomock.Any(), keyID, companyID).
					Return(errors.New("internal"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newAPIKeyRouter(deps, companyID, "user_1"), "DELETE", "/api-keys/"+tt.keyID, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
