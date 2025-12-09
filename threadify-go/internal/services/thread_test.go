package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/internal/models"
)

// MockClientQueue is a mock implementation of ClientQueue
type MockClientQueue struct {
	mock.Mock
}

func (m *MockClientQueue) AddClient(client *models.ConnectedClient) error {
	args := m.Called(client)
	return args.Error(0)
}

func (m *MockClientQueue) GetClient(ownerID string) (*models.ConnectedClient, error) {
	args := m.Called(ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ConnectedClient), args.Error(1)
}

func (m *MockClientQueue) RemoveClient(ownerID string) error {
	args := m.Called(ownerID)
	return args.Error(0)
}

func TestNewThreadService(t *testing.T) {
	mockQueue := new(MockClientQueue)
	service := NewThreadService(mockQueue)

	assert.NotNil(t, service)
	assert.Equal(t, mockQueue, service.queue)
}

func TestThreadService_HandleConnect(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.ConnectRequest{
			ApiKey:           "test-api-key",
			OwnerID:          "owner123",
			SubscribedEvents: []string{"event1", "event2"},
		}

		mockQueue.On("AddClient", mock.MatchedBy(func(client *models.ConnectedClient) bool {
			return client.OwnerID == "owner123" && client.ApiKey == "test-api-key"
		})).Return(nil)

		response := service.HandleConnect(req)

		assert.Equal(t, "connect", response.Action)
		assert.Equal(t, "success", response.Status)
		assert.Equal(t, "Connected successfully", response.Message)
		assert.Equal(t, "owner123", response.OwnerID)
		assert.Equal(t, []string{"event1", "event2"}, response.SubscribedEvents)
		mockQueue.AssertExpectations(t)
	})

	t.Run("missing API key", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.ConnectRequest{
			ApiKey:  "",
			OwnerID: "owner123",
		}

		response := service.HandleConnect(req)

		assert.Equal(t, "connect", response.Action)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "API key is required", response.Message)
	})

	t.Run("missing owner ID", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.ConnectRequest{
			ApiKey:  "test-api-key",
			OwnerID: "",
		}

		response := service.HandleConnect(req)

		assert.Equal(t, "connect", response.Action)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Owner ID is required", response.Message)
	})

	t.Run("queue add client fails", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.ConnectRequest{
			ApiKey:  "test-api-key",
			OwnerID: "owner123",
		}

		mockQueue.On("AddClient", mock.Anything).Return(assert.AnError)

		response := service.HandleConnect(req)

		assert.Equal(t, "connect", response.Action)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Failed to register client", response.Message)
		mockQueue.AssertExpectations(t)
	})
}

func TestThreadService_HandleStartThread(t *testing.T) {
	t.Run("successful thread start", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.StartThreadRequest{
			ContractID: "contract123",
		}

		response := service.HandleStartThread(req, "owner123")

		assert.Equal(t, "startThread", response.Action)
		assert.Equal(t, "success", response.Status)
		assert.Equal(t, "Thread started successfully", response.Message)
		assert.NotEmpty(t, response.ThreadID)
		assert.Equal(t, "contract123", response.ContractID)
	})

	t.Run("not authenticated", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.StartThreadRequest{
			ContractID: "contract123",
		}

		response := service.HandleStartThread(req, "")

		assert.Equal(t, "startThread", response.Action)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Not authenticated. Please connect first.", response.Message)
	})

	t.Run("thread ID is unique", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.StartThreadRequest{
			ContractID: "contract123",
		}

		response1 := service.HandleStartThread(req, "owner123")
		response2 := service.HandleStartThread(req, "owner123")

		assert.NotEqual(t, response1.ThreadID, response2.ThreadID)
	})
}

func TestThreadService_HandleRecordEvent(t *testing.T) {
	t.Run("successful event recording", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.RecordEventRequest{
			ThreadID: "thread123",
		}

		response := service.HandleRecordEvent(req, "owner123")

		assert.Equal(t, "recordThreadEvent", response.Action)
		assert.Equal(t, "success", response.Status)
		assert.Equal(t, "Event recorded successfully", response.Message)
		assert.Equal(t, "thread123", response.ThreadID)
	})

	t.Run("not authenticated", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.RecordEventRequest{
			ThreadID: "thread123",
		}

		response := service.HandleRecordEvent(req, "")

		assert.Equal(t, "recordThreadEvent", response.Action)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Not authenticated. Please connect first.", response.Message)
	})

	t.Run("missing thread ID", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		req := &models.RecordEventRequest{
			ThreadID: "",
		}

		response := service.HandleRecordEvent(req, "owner123")

		assert.Equal(t, "recordThreadEvent", response.Action)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Thread ID is required", response.Message)
	})
}

func TestThreadService_HandleClose(t *testing.T) {
	t.Run("successful close with owner ID", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		mockQueue.On("RemoveClient", "owner123").Return(nil)

		response := service.HandleClose("owner123")

		assert.Equal(t, "closeConnection", response.Action)
		assert.Equal(t, "success", response.Status)
		assert.Equal(t, "Connection closed successfully", response.Message)
		mockQueue.AssertExpectations(t)
	})

	t.Run("close without owner ID", func(t *testing.T) {
		mockQueue := new(MockClientQueue)
		service := NewThreadService(mockQueue)

		response := service.HandleClose("")

		assert.Equal(t, "closeConnection", response.Action)
		assert.Equal(t, "success", response.Status)
		assert.Equal(t, "Connection closed successfully", response.Message)
		// Should not call RemoveClient
		mockQueue.AssertNotCalled(t, "RemoveClient")
	})
}

func TestThreadService_ConnectLifecycle(t *testing.T) {
	// Test a full lifecycle: connect -> start thread -> record event -> close
	mockQueue := new(MockClientQueue)
	service := NewThreadService(mockQueue)

	// 1. Connect
	connectReq := &models.ConnectRequest{
		ApiKey:           "test-api-key",
		OwnerID:          "owner123",
		SubscribedEvents: []string{"event1"},
	}
	mockQueue.On("AddClient", mock.Anything).Return(nil)
	connectResp := service.HandleConnect(connectReq)
	assert.Equal(t, "success", connectResp.Status)

	// 2. Start thread
	startReq := &models.StartThreadRequest{
		ContractID: "contract123",
	}
	startResp := service.HandleStartThread(startReq, "owner123")
	assert.Equal(t, "success", startResp.Status)
	threadID := startResp.ThreadID

	// 3. Record event
	recordReq := &models.RecordEventRequest{
		ThreadID: threadID,
	}
	recordResp := service.HandleRecordEvent(recordReq, "owner123")
	assert.Equal(t, "success", recordResp.Status)

	// 4. Close
	mockQueue.On("RemoveClient", "owner123").Return(nil)
	closeResp := service.HandleClose("owner123")
	assert.Equal(t, "success", closeResp.Status)

	mockQueue.AssertExpectations(t)
}
