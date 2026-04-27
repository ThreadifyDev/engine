package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	agentCompanyID = "comp_agent_123"
	agentUserID    = "user_agent_abc"
)

func newAgentRouter(deps *common.MockedHandlers, routes func(*handlers.AgentHandler, *gin.Engine)) *gin.Engine {
	h := handlers.NewAgentHandler(deps.AgentSvc)
	r := common.SetupTestRouter()
	r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: agentCompanyID, UserID: agentUserID}))
	routes(h, r)
	return r
}

func TestAgentHandler_GetConversations(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					GetConversations(gomock.Any(), agentCompanyID).
					Return([]models.AgentConversation{{ID: "conv_1", Title: "Conv 1"}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service_error",
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					GetConversations(gomock.Any(), agentCompanyID).
					Return(nil, errors.New("db failure"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			r := newAgentRouter(deps, func(h *handlers.AgentHandler, r *gin.Engine) {
				r.GET("/conversations", h.GetConversations)
			})
			w := common.DoRequest(t, r, "GET", "/conversations", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAgentHandler_GetConversationMessages(t *testing.T) {
	const convID = "conv_abc"

	tests := []struct {
		name       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					GetMessagesForUser(gomock.Any(), agentCompanyID, convID).
					Return([]*models.AgentMessage{{ID: "m1", Content: "Hello"}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					GetMessagesForUser(gomock.Any(), agentCompanyID, convID).
					Return(nil, service.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "forbidden",
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					GetMessagesForUser(gomock.Any(), agentCompanyID, convID).
					Return(nil, service.ErrForbidden)
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			r := newAgentRouter(deps, func(h *handlers.AgentHandler, r *gin.Engine) {
				r.GET("/conversations/:id/messages", h.GetConversation)
			})
			w := common.DoRequest(t, r, "GET", "/conversations/"+convID+"/messages", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAgentHandler_Chat(t *testing.T) {
	tests := []struct {
		name          string
		body          any
		setAuthHeader bool
		setupMock     func(*common.MockedHandlers)
		wantStatus    int
	}{
		{
			name:          "success_stream",
			body:          map[string]string{"message": "hi", "conversation_id": "c1"},
			setAuthHeader: true,
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					ChatStream(gomock.Any(), "Bearer token", agentUserID, agentCompanyID, "c1", "hi", gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx, auth, u, c, conv, msg, skill interface{}, onEvent models.StreamHandler) error {
						onEvent("message", "reply")
						return nil
					})
			},
			wantStatus: http.StatusOK,
		},
		{

			name:          "missing_authorization_header",
			body:          map[string]string{"message": "hi"},
			setAuthHeader: false,
			setupMock:     func(d *common.MockedHandlers) {},
			wantStatus:    http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)

			r := newAgentRouter(deps, func(h *handlers.AgentHandler, r *gin.Engine) {
				r.POST("/chat", h.Chat)
			})

			req := common.BuildRequest(t, "POST", "/chat", tt.body)
			if tt.setAuthHeader {
				req.Header.Set("Authorization", "Bearer token")
			}

			w := common.DoRequestFromReq(t, r, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAgentHandler_DeleteConversation(t *testing.T) {
	const convID = "c1"
	tests := []struct {
		name       string
		convID     string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name:   "success",
			convID: convID,
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					DeleteConversation(gomock.Any(), agentCompanyID, convID).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "not_found",
			convID: convID,
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					DeleteConversation(gomock.Any(), agentCompanyID, convID).
					Return(service.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "forbidden",
			convID: convID,
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					DeleteConversation(gomock.Any(), agentCompanyID, convID).
					Return(service.ErrForbidden)
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			r := newAgentRouter(deps, func(h *handlers.AgentHandler, r *gin.Engine) {
				r.DELETE("/conversations/:id", h.DeleteConversation)
			})
			w := common.DoRequest(t, r, "DELETE", "/conversations/"+tt.convID, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAgentHandler_ContinueConversation(t *testing.T) {
	const (
		convID = "conv_123"
	)

	tests := []struct {
		name       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]interface{}{"message": "next question", "user_id": agentUserID},
			setupMock: func(d *common.MockedHandlers) {
				d.AgentSvc.EXPECT().
					ContinueConversation(gomock.Any(), agentUserID, agentCompanyID, convID).
					Return("new_conv", "title", "summary", nil)
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)

			r := newAgentRouter(deps, func(h *handlers.AgentHandler, r *gin.Engine) {
				r.POST("/conversations/:id/continue", h.ContinueConversation)
			})

			w := common.DoRequest(t, r, "POST", "/conversations/"+convID+"/continue", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
