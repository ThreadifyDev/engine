package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type MockedEngineHandlers struct {
	Ctrl *gomock.Controller

	ContractSvc          *enginemocks.MockContractService
	ThreadSvc            *enginemocks.MockThreadService
	StepEventSvc         *enginemocks.MockStepEventProcessor
	InvitationSvc        *enginemocks.MockInvitationTokenService
	NotificationConsumer *enginemocks.MockNotificationConsumer
	NotificationRouter   *enginemocks.MockNotificationRouter
	PlanSvc              *enginemocks.MockPlanService
	ValkeyClient         *enginemocks.MockValkeyClient
	LuaScriptManager     *enginemocks.MockLuaScriptManager
	WebhookProvider      *enginemocks.MockWebhookProvider
	BillingWebhookSvc    *enginemocks.MockBillingWebhookService

	Logger *zap.Logger
}

func NewMockedEngineHandlers(t *testing.T) *MockedEngineHandlers {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	return &MockedEngineHandlers{
		Ctrl: ctrl,

		ContractSvc:          enginemocks.NewMockContractService(ctrl),
		ThreadSvc:            enginemocks.NewMockThreadService(ctrl),
		StepEventSvc:         enginemocks.NewMockStepEventProcessor(ctrl),
		InvitationSvc:        enginemocks.NewMockInvitationTokenService(ctrl),
		NotificationConsumer: enginemocks.NewMockNotificationConsumer(ctrl),
		NotificationRouter:   enginemocks.NewMockNotificationRouter(ctrl),
		PlanSvc:              enginemocks.NewMockPlanService(ctrl),
		ValkeyClient:         enginemocks.NewMockValkeyClient(ctrl),
		LuaScriptManager:     enginemocks.NewMockLuaScriptManager(ctrl),
		WebhookProvider:      enginemocks.NewMockWebhookProvider(ctrl),
		BillingWebhookSvc:    enginemocks.NewMockBillingWebhookService(ctrl),

		Logger: zap.NewNop(),
	}
}

func SetupTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	return r
}

func DoRequest(t *testing.T, r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
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

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func BuildRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	var req *http.Request
	var err error

	if body != nil {
		switch v := body.(type) {
		case string:
			req, err = http.NewRequest(method, path, bytes.NewBufferString(v))
		default:
			var buf bytes.Buffer
			err = json.NewEncoder(&buf).Encode(body)
			require.NoError(t, err)
			req, err = http.NewRequest(method, path, &buf)
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		req, err = http.NewRequest(method, path, http.NoBody)
	}
	require.NoError(t, err)
	return req
}

func DoRequestFromReq(t *testing.T, r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type AuthIDs struct {
	CompanyID string
	UserID    string
}

func WithAuthContext(ids AuthIDs) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ids.CompanyID != "" {
			c.Set(sharedauth.CtxCompanyID, ids.CompanyID)
		}
		if ids.UserID != "" {
			c.Set(sharedauth.CtxUserID, ids.UserID)
		}
		c.Next()
	}
}

type MockWSConnection struct {
	LastMessage interface{}
	WriteErr    error
}

func (m *MockWSConnection) WriteJSON(v interface{}) error {
	m.LastMessage = v
	return m.WriteErr
}

type MockWSMutex struct{}

func (m *MockWSMutex) Lock()   {}
func (m *MockWSMutex) Unlock() {}
