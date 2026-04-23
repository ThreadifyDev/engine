package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	billingmodels "threadify-go/shared/models"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/types"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

const testThreadID = "thread_111"

func TestWebSocketHandler_HandleMessage(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		msg       map[string]interface{}
		setupMock func(d *MockedEngineHandlers)
		wantResp  interface{}
	}{
		{
			name:   "connect_success",
			action: ActionConnect,
			msg:    map[string]interface{}{"action": ActionConnect, "apiKey": "key_123"},
			setupMock: func(d *MockedEngineHandlers) {
				resp := &models.ConnectResponse{
					Action:    ActionConnect,
					Status:    StatusSuccess,
					OwnerID:   testUserID,
					CompanyID: testCompanyID,
				}
				d.ThreadSvc.EXPECT().
					HandleConnect(gomock.Any(), gomock.Any()).
					Return(resp)

				d.PlanSvc.EXPECT().
					GetCurrentLimits(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)

				d.NotificationRouter.EXPECT().
					HandleConnect(gomock.Any(), testUserID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantResp: &models.ConnectResponse{
				Action:    ActionConnect,
				Status:    StatusSuccess,
				OwnerID:   testUserID,
				CompanyID: testCompanyID,
			},
		},
		{
			name:   "connect_failure_auth",
			action: ActionConnect,
			msg:    map[string]interface{}{"action": ActionConnect, "apiKey": "bad_key"},
			setupMock: func(d *MockedEngineHandlers) {
				resp := &models.ConnectResponse{
					Action: ActionConnect,
					Status: StatusError,
				}
				d.ThreadSvc.EXPECT().
					HandleConnect(gomock.Any(), gomock.Any()).
					Return(resp)
			},
			wantResp: &models.ConnectResponse{
				Action: ActionConnect,
				Status: StatusError,
			},
		},
		{
			name:   "startThread_success",
			action: ActionStartThread,
			msg:    map[string]interface{}{"action": ActionStartThread, "contractName": "Test"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)

				resp := &models.StartThreadResponse{
					Action:   ActionStartThread,
					Status:   StatusSuccess,
					ThreadID: testThreadID,
				}
				d.ThreadSvc.EXPECT().
					HandleStartThread(gomock.Any(), gomock.Any(), testUserID, testCompanyID).
					Return(resp)
			},
			wantResp: &models.StartThreadResponse{
				Action:   ActionStartThread,
				Status:   StatusSuccess,
				ThreadID: testThreadID,
			},
		},
		{
			name:   "recordEvent_success",
			action: ActionRecordThreadEvent,
			msg:    map[string]interface{}{"action": ActionRecordThreadEvent, "threadId": testThreadID, "stepName": "Step1"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)

				resp := &models.RecordEventResponse{
					Action: ActionRecordThreadEvent,
					Status: StatusSuccess,
				}
				d.ThreadSvc.EXPECT().
					HandleRecordEvent(gomock.Any(), gomock.Any(), testUserID, testCompanyID).
					Return(resp)
			},
			wantResp: &models.RecordEventResponse{
				Action: ActionRecordThreadEvent,
				Status: StatusSuccess,
			},
		},
		{
			name:   "inviteParty_success",
			action: ActionInviteParty,
			msg:    map[string]interface{}{"action": ActionInviteParty, "role": "member"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)

				resp := &models.InvitePartyResponse{
					Action: ActionInviteParty,
					Status: StatusSuccess,
				}
				d.ThreadSvc.EXPECT().
					HandleInviteParty(gomock.Any(), testUserID, testCompanyID, gomock.Any()).
					Return(resp, nil)
			},
			wantResp: &models.InvitePartyResponse{
				Action: ActionInviteParty,
				Status: StatusSuccess,
			},
		},
		{
			name:   "joinThread_success",
			action: ActionJoinThread,
			msg:    map[string]interface{}{"action": ActionJoinThread, "token": "token123"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)

				resp := &models.JoinThreadResponse{
					Action:   ActionJoinThread,
					Status:   StatusSuccess,
					ThreadID: testThreadID,
				}
				d.ThreadSvc.EXPECT().
					HandleJoinThread(gomock.Any(), testUserID, testCompanyID).
					Return(resp, nil)
			},
			wantResp: &models.JoinThreadResponse{
				Action:   ActionJoinThread,
				Status:   StatusSuccess,
				ThreadID: testThreadID,
			},
		},
		{
			name:   "enforceCredits_insufficient",
			action: ActionStartThread,
			msg:    map[string]interface{}{"action": ActionStartThread},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(nil, service.ErrInsufficientCredit)
			},
			wantResp: models.ErrorResponse{
				Action:  ActionStartThread,
				Status:  StatusError,
				Message: "Payment required: Your credit balance is exhausted. Please top up to continue.",
			},
		},
		{
			name:   "subscribe_no_router",
			action: ActionSubscribe,
			msg:    map[string]interface{}{"action": ActionSubscribe, "stepName": "Step1"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)
			},
			wantResp: models.ErrorResponse{
				Action:  ActionSubscribe,
				Status:  StatusError,
				Message: "Notification router not available",
			},
		},
		{
			name:   "unknown_action",
			action: "unknown",
			msg:    map[string]interface{}{"action": "unknown"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&billingmodels.CreditAccount{}, nil)
			},
			wantResp: models.ErrorResponse{
				Action:  "unknown",
				Status:  StatusError,
				Message: "Unknown action: unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			var nRouter types.NotificationRouter
			if tt.name != "subscribe_no_router" {
				nRouter = d.NotificationRouter
			}

			h := NewWebSocketHandler(
				d.ThreadSvc,
				d.StepEventSvc,
				d.InvitationSvc,
				d.NotificationConsumer,
				nRouter,
				d.PlanSvc,
				d.ValkeyClient,
				d.LuaScriptManager,
				&config.RateLimitConfig{},
				&config.WebSocketConfig{},
				d.Logger,
			)

			session := &WSSession{
				conn:      &MockWSConnection{},
				ownerID:   testUserID,
				companyID: testCompanyID,
				ctx:       context.Background(),
			}

			msgBytes, _ := json.Marshal(tt.msg)
			resp := h.handleMessage(tt.action, tt.msg, msgBytes, session)

			assert.Equal(t, tt.wantResp, resp)
		})
	}
}

func TestWebSocketHandler_HandleThreadEnd(t *testing.T) {
	tests := []struct {
		name      string
		threadID  string
		setupMock func(d *MockedEngineHandlers)
		wantResp  interface{}
	}{
		{
			name:     "success",
			threadID: testThreadID,
			setupMock: func(d *MockedEngineHandlers) {
				d.ThreadSvc.EXPECT().
					EndThread(gomock.Any(), testThreadID, testUserID, service.ActorServiceRuleEngine, "completed", gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantResp: map[string]interface{}{
				"action":       ActionThreadEnd,
				"status":       StatusSuccess,
				"threadId":     testThreadID,
				"threadStatus": "completed",
				"message":      "Thread completed successfully",
			},
		},
		{
			name:     "service_error",
			threadID: testThreadID,
			setupMock: func(d *MockedEngineHandlers) {
				d.ThreadSvc.EXPECT().
					EndThread(gomock.Any(), testThreadID, testUserID, service.ActorServiceRuleEngine, "completed", gomock.Any(), gomock.Any()).
					Return(errors.New("end error"))
			},
			wantResp: models.ErrorResponse{
				Action:  ActionThreadEnd,
				Status:  StatusError,
				Message: "Failed to end thread: end error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockedEngineHandlers(t)
			tt.setupMock(d)

			h := NewWebSocketHandler(
				d.ThreadSvc,
				d.StepEventSvc,
				d.InvitationSvc,
				d.NotificationConsumer,
				d.NotificationRouter,
				d.PlanSvc,
				d.ValkeyClient,
				d.LuaScriptManager,
				&config.RateLimitConfig{},
				&config.WebSocketConfig{},
				d.Logger,
			)

			session := &WSSession{
				conn:      &MockWSConnection{},
				ownerID:   testUserID,
				companyID: testCompanyID,
				ctx:       context.Background(),
			}

			resp := h.handleThreadEnd(session, tt.threadID, "completed", "done")

			if tt.name == "success" {
				respMap := resp.(map[string]interface{})
				delete(respMap, "completedAt")
				delete(respMap, "cancelledAt")
				assert.Equal(t, tt.wantResp, respMap)
			} else {
				assert.Equal(t, tt.wantResp, resp)
			}
		})
	}
}
