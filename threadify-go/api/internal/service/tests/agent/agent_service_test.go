package agent

import (
	"context"
	"testing"

	"threadify-go/api/internal/service/tests/common"
	"threadify-go/shared/management/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentService_GetConversations(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().GetConversations(gomock.Any(), companyID).Return([]domain.AgentConversation{
					{ID: "conv_1", Title: "Conv 1"},
				}, nil)
			},
		},
		{
			name: "repo_error",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().GetConversations(gomock.Any(), companyID).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAgentService("http://engine", "key", 10, 1000, 500)
			tt.setupMock(deps)

			res, err := svc.GetConversations(context.Background(), companyID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, res, 1)
			}
		})
	}
}

func TestAgentService_GetMessagesForUser(t *testing.T) {
	const (
		companyID = "comp_123"
		convID    = "conv_1"
	)

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().GetConversations(gomock.Any(), companyID).Return([]domain.AgentConversation{
					{ID: convID},
				}, nil)
				deps.AgentRepo.EXPECT().GetMessages(gomock.Any(), convID).Return([]*domain.AgentMessage{
					{ID: "msg_1", Content: "Hello"},
				}, nil)
			},
		},
		{
			name: "unauthorized_ownership",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().GetConversations(gomock.Any(), companyID).Return([]domain.AgentConversation{
					{ID: "other_conv"},
				}, nil)
			},
			wantErr: true,
		},
		{
			name: "repo_error_get_messages",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().GetConversations(gomock.Any(), companyID).Return([]domain.AgentConversation{
					{ID: convID},
				}, nil)
				deps.AgentRepo.EXPECT().GetMessages(gomock.Any(), convID).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAgentService("http://engine", "key", 10, 1000, 500)
			tt.setupMock(deps)

			res, err := svc.GetMessagesForUser(context.Background(), companyID, convID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, res, 1)
			}
		})
	}
}

func TestAgentService_DeleteConversation(t *testing.T) {
	const (
		convID = "conv_1"
		userID = "user_1"
	)

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().DeleteConversation(gomock.Any(), convID, userID).Return(nil)
			},
		},
		{
			name: "error",
			setupMock: func(deps *common.MockedDeps) {
				deps.AgentRepo.EXPECT().DeleteConversation(gomock.Any(), convID, userID).Return(assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAgentService("http://engine", "key", 10, 1000, 500)
			tt.setupMock(deps)

			err := svc.DeleteConversation(context.Background(), convID, userID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
