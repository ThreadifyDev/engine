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
	apikeymocks "threadify-go/api/internal/service/mocks/service/apikey"
	authmocks "threadify-go/api/internal/service/mocks/service/auth"
	entityprofilemocks "threadify-go/api/internal/service/mocks/service/entityprofile"
	invitationmocks "threadify-go/api/internal/service/mocks/service/invitation"
	serviceaccountmocks "threadify-go/api/internal/service/mocks/service/serviceaccount"
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

	// Repositories
	UserRepo       *userrepomocks.MockUserRepository
	UserRoleRepo   *userrolerepomocks.MockUserRoleRepository
	CompanyRepo    *companyrepomocks.MockCompanyRepository
	AgentRepo      *agentrepomocks.MockAgentRepository
	APIKeyRepo     *apikeyrepomocks.MockAPIKeyRepository
	InvitationRepo *invitationrepomocks.MockTeamInvitationRepository

	BillingProvider *billingmocks.MockBillingProvider
	PlanRepo        *billingmocks.MockPlanRepository
	BillingSvc      *billingpkg.BillingService

	Logger *zap.Logger
}

func NewMockedHandlers(t *testing.T) *MockedHandlers {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	billingProvider := billingmocks.NewMockBillingProvider(ctrl)
	planRepo := billingmocks.NewMockPlanRepository(ctrl)
	billingCfg := &config.BillingConfig{
		SuccessURL: "http://ok",
		CancelURL:  "http://cancel",
	}
	billingSvc := billingpkg.NewBillingService(
		billingProvider,
		planRepo,
		&config.SubscriptionConfig{}, // placeholder
		billingCfg,
		zap.NewNop(),
	)

	return &MockedHandlers{
		Ctrl: ctrl,

		AuthSvc:           authmocks.NewMockAuthService(ctrl),
		APIKeySvc:         apikeymocks.NewMockAPIKeyService(ctrl),
		InvitationSvc:     invitationmocks.NewMockTeamInvitationService(ctrl),
		AgentSvc:          agentmocks.NewMockAgentService(ctrl),
		ServiceAccountSvc: serviceaccountmocks.NewMockServiceAccountService(ctrl),
		EntityProfileSvc:  entityprofilemocks.NewMockEntityProfileTypeService(ctrl),

		UserRepo:       userrepomocks.NewMockUserRepository(ctrl),
		UserRoleRepo:   userrolerepomocks.NewMockUserRoleRepository(ctrl),
		CompanyRepo:    companyrepomocks.NewMockCompanyRepository(ctrl),
		AgentRepo:      agentrepomocks.NewMockAgentRepository(ctrl),
		APIKeyRepo:     apikeyrepomocks.NewMockAPIKeyRepository(ctrl),
		InvitationRepo: invitationrepomocks.NewMockTeamInvitationRepository(ctrl),

		BillingProvider: billingProvider,
		PlanRepo:        planRepo,
		BillingSvc:      billingSvc,

		Logger: zap.NewNop(),
	}
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

	var req *http.Request
	var err error

	if body != nil {
		var buf bytes.Buffer
		err = json.NewEncoder(&buf).Encode(body)
		require.NoError(t, err)
		req, err = http.NewRequest(method, path, &buf)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, path, http.NoBody)
		require.NoError(t, err)
	}

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
		var buf bytes.Buffer
		err = json.NewEncoder(&buf).Encode(body)
		require.NoError(t, err)
		req, err = http.NewRequest(method, path, &buf)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, path, http.NoBody)
		require.NoError(t, err)
	}

	return req
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

func UnmarshalBody(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
