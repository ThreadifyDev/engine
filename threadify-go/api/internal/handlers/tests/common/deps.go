package common

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentrepomocks "threadify-go/api/internal/service/mocks/repository/agent"
	apikeyrepomocks "threadify-go/api/internal/service/mocks/repository/apikey"
	companyrepomocks "threadify-go/api/internal/service/mocks/repository/company"
	invitationrepomocks "threadify-go/api/internal/service/mocks/repository/invitation"
	userrepomocks "threadify-go/api/internal/service/mocks/repository/user"
	userrolerepomocks "threadify-go/api/internal/service/mocks/repository/userrole"
	agentmocks "threadify-go/api/internal/service/mocks/service/agent"
	apikeymocks "threadify-go/api/internal/service/mocks/service/api_key"
	authmocks "threadify-go/api/internal/service/mocks/service/auth"
	entityprofilemocks "threadify-go/api/internal/service/mocks/service/entity_profile_type"
	serviceaccountmocks "threadify-go/api/internal/service/mocks/service/service_account"
	invitationmocks "threadify-go/api/internal/service/mocks/service/team_invitation"
	usermocks "threadify-go/api/internal/service/mocks/service/user"
	sharedauth "threadify-go/shared/auth"
	billingpkg "threadify-go/shared/billing"
	billingmocks "threadify-go/shared/billing/mocks"
	"threadify-go/shared/config"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type MockedHandlers struct {
	Ctrl *gomock.Controller

	AuthSvc           *authmocks.MockAuthService
	APIKeySvc         *apikeymocks.MockAPIKeyService
	InvitationSvc     *invitationmocks.MockTeamInvitationService
	AgentSvc          *agentmocks.MockAgentService
	ServiceAccountSvc *serviceaccountmocks.MockServiceAccountService
	EntityProfileSvc  *entityprofilemocks.MockEntityProfileTypeService
	UserSvc           *usermocks.MockUserService

	// Repositories
	UserRepo       *userrepomocks.MockUserRepository
	UserRoleRepo   *userrolerepomocks.MockUserRoleRepository
	CompanyRepo    *companyrepomocks.MockCompanyRepository
	AgentRepo      *agentrepomocks.MockAgentRepository
	APIKeyRepo     *apikeyrepomocks.MockAPIKeyRepository
	InvitationRepo *invitationrepomocks.MockTeamInvitationRepository

	BillingProvider *billingmocks.MockBillingProvider
	PlanRepo        *billingmocks.MockPlanRepository

	Logger *zap.Logger
}

func NewMockedHandlers(t *testing.T) *MockedHandlers {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	return &MockedHandlers{
		Ctrl: ctrl,

		AuthSvc:           authmocks.NewMockAuthService(ctrl),
		APIKeySvc:         apikeymocks.NewMockAPIKeyService(ctrl),
		InvitationSvc:     invitationmocks.NewMockTeamInvitationService(ctrl),
		AgentSvc:          agentmocks.NewMockAgentService(ctrl),
		ServiceAccountSvc: serviceaccountmocks.NewMockServiceAccountService(ctrl),
		EntityProfileSvc:  entityprofilemocks.NewMockEntityProfileTypeService(ctrl),
		UserSvc:           usermocks.NewMockUserService(ctrl),

		UserRepo:       userrepomocks.NewMockUserRepository(ctrl),
		UserRoleRepo:   userrolerepomocks.NewMockUserRoleRepository(ctrl),
		CompanyRepo:    companyrepomocks.NewMockCompanyRepository(ctrl),
		AgentRepo:      agentrepomocks.NewMockAgentRepository(ctrl),
		APIKeyRepo:     apikeyrepomocks.NewMockAPIKeyRepository(ctrl),
		InvitationRepo: invitationrepomocks.NewMockTeamInvitationRepository(ctrl),

		BillingProvider: billingmocks.NewMockBillingProvider(ctrl),
		PlanRepo:        billingmocks.NewMockPlanRepository(ctrl),

		Logger: zap.NewNop(),
	}
}

func (m *MockedHandlers) NewBillingService() *billingpkg.BillingService {
	return billingpkg.NewBillingService(
		m.BillingProvider,
		m.PlanRepo,
		&config.SubscriptionConfig{},
		&config.BillingConfig{
			SuccessURL: "http://ok",
			CancelURL:  "http://cancel",
		},
		m.Logger,
	)
}

type RouterOption func(*gin.Engine)

func WithRecovery() RouterOption {
	return func(r *gin.Engine) {
		r.Use(gin.Recovery())
	}
}

func SetupTestRouter(opts ...RouterOption) *gin.Engine {
	r := gin.New()
	for _, o := range opts {
		o(r)
	}
	return r
}

func DoRequest(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := BuildRequest(t, method, path, body)
	return DoRequestFromReq(t, r, req)
}

func DoRequestFromReq(t *testing.T, r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func BuildRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var req *http.Request
	var err error

	if body != nil {
		buf := encodeBody(t, body)
		req, err = http.NewRequest(method, path, buf)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, path, http.NoBody)
		require.NoError(t, err)
	}

	return req
}

func encodeBody(t *testing.T, body any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	return &buf
}

func UnmarshalBody(t *testing.T, data []byte, v any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(data, v))
}

type AuthIDs struct {
	CompanyID string
	UserID    string
}

func WithAuthContext(ids AuthIDs) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(sharedauth.CtxCompanyID, ids.CompanyID)
		c.Set(sharedauth.CtxUserID, ids.UserID)
		c.Next()
	}
}

func WithNoAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
