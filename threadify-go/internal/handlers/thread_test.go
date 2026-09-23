package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	shareddomain "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
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
				resp := &domain.ConnectResponse{
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
					Return(&shareddomain.CreditAccount{}, nil)

				d.NotificationRouter.EXPECT().
					HandleSubscribe(gomock.Any(), "global", "", nil).
					Return(nil)

				d.NotificationRouter.EXPECT().
					HandleConnect(gomock.Any(), testUserID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantResp: &dto.ConnectResponse{
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
				resp := &domain.ConnectResponse{
					Action: ActionConnect,
					Status: StatusError,
				}
				d.ThreadSvc.EXPECT().
					HandleConnect(gomock.Any(), gomock.Any()).
					Return(resp)
			},
			wantResp: &dto.ConnectResponse{
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
					Return(&shareddomain.CreditAccount{}, nil)

				resp := &domain.StartThreadResponse{
					Action:   ActionStartThread,
					Status:   StatusSuccess,
					ThreadID: testThreadID,
				}
				d.ThreadSvc.EXPECT().
					HandleStartThread(gomock.Any(), gomock.Any(), testUserID, testCompanyID).
					Return(resp)
			},
			wantResp: &dto.StartThreadResponse{
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
					Return(&shareddomain.CreditAccount{}, nil)

				resp := &domain.RecordEventResponse{
					Action: ActionRecordThreadEvent,
					Status: StatusSuccess,
				}
				d.ThreadSvc.EXPECT().
					HandleRecordEvent(gomock.Any(), gomock.Any(), testUserID, testCompanyID).
					Return(resp)
			},
			wantResp: &dto.RecordEventResponse{
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
					Return(&shareddomain.CreditAccount{}, nil)

				resp := &domain.InvitePartyResponse{
					Action: ActionInviteParty,
					Status: StatusSuccess,
				}
				d.ThreadSvc.EXPECT().
					HandleInviteParty(gomock.Any(), gomock.Any(), testUserID, testCompanyID, gomock.Any()).
					Return(resp, nil)
			},
			wantResp: &dto.InvitePartyResponse{
				Action: ActionInviteParty,
				Status: StatusSuccess,
			},
		},
		{
			name:   "joinThread_success",
			action: ActionJoinThread,
			msg:    map[string]interface{}{"action": ActionJoinThread, "threadToken": "token123"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&shareddomain.CreditAccount{}, nil)

				resp := &domain.JoinThreadResponse{
					Action:   ActionJoinThread,
					Status:   StatusSuccess,
					ThreadID: testThreadID,
				}
				d.ThreadSvc.EXPECT().
					HandleJoinThread(gomock.Any(), gomock.Any(), testUserID, testCompanyID).
					Return(resp, nil)
			},
			wantResp: &dto.JoinThreadResponse{
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
			wantResp: dto.ErrorResponse{
				Action:  ActionStartThread,
				Status:  StatusError,
				Message: "Insufficient credits",
				Details: "insufficient credit balance",
			},
		},
		{
			name:   "subscribe_no_router",
			action: ActionSubscribe,
			msg:    map[string]interface{}{"action": ActionSubscribe, "stepName": "Step1"},
			setupMock: func(d *MockedEngineHandlers) {
				d.PlanSvc.EXPECT().
					CheckBalancePositive(gomock.Any(), testCompanyID).
					Return(&shareddomain.CreditAccount{}, nil)
			},
			wantResp: dto.ErrorResponse{
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
					Return(&shareddomain.CreditAccount{}, nil)
			},
			wantResp: dto.ErrorResponse{
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

			var nRouter domain.NotificationRouter
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
			wantResp: dto.ErrorResponse{
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

func TestWebSocketThreadKeyRequestAndStoredMetadata(t *testing.T) {
	d := NewMockedEngineHandlers(t)
	d.PlanSvc.EXPECT().CheckBalancePositive(gomock.Any(), testCompanyID).Return(&shareddomain.CreditAccount{}, nil)
	version := 3
	d.ThreadSvc.EXPECT().HandleStartThread(gomock.Any(), gomock.Any(), testUserID, testCompanyID).
		DoAndReturn(func(_ context.Context, cmd *domain.StartThreadCmd, _, _ string) *domain.StartThreadResponse {
			assert.Equal(t, "thread", cmd.Action)
			assert.Equal(t, "session-1", cmd.ThreadKey)
			assert.Empty(t, cmd.ContractName, "resuming must not require the contract")
			assert.Equal(t, "worker-service", cmd.ServiceName)
			return &domain.StartThreadResponse{Action: ActionStartThread, Status: StatusSuccess, ThreadID: testThreadID, ThreadKey: "session-1", Label: "Original", ContractName: "agent", ContractVersion: &version, Refs: map[string]string{"customerId": "customer"}}
		})
	h := NewWebSocketHandler(d.ThreadSvc, d.StepEventSvc, d.InvitationSvc, d.NotificationConsumer, d.NotificationRouter, d.PlanSvc, d.ValkeyClient, d.LuaScriptManager, &config.WebSocketConfig{}, d.Logger)
	session := &WSSession{conn: &MockWSConnection{}, ownerID: testUserID, companyID: testCompanyID, ctx: context.Background()}
	msg := map[string]interface{}{"action": "thread", "threadKey": "session-1", "serviceName": "worker-service", "requestId": "request-1"}
	raw, err := json.Marshal(msg)
	assert.NoError(t, err)
	response := correlatedResponse(h.handleMessage("thread", msg, raw, session), msg).(map[string]interface{})
	assert.Equal(t, "thread", response["action"])
	assert.Equal(t, "request-1", response["requestId"])
	assert.Equal(t, "session-1", response["threadKey"])
	assert.Equal(t, "agent", response["contractName"])
	assert.EqualValues(t, 3, response["contractVersion"])
	assert.Equal(t, "Original", response["label"])
	assert.Equal(t, []string{testThreadID}, session.threadIDs)
}
