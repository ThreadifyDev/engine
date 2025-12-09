package queue

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/internal/models"
)

// MockValkeyClient is a mock implementation of ValkeyClient interface
type MockValkeyClient struct {
	mock.Mock
}

func (m *MockValkeyClient) Set(key, value string, ttl time.Duration) error {
	args := m.Called(key, value, ttl)
	return args.Error(0)
}

func (m *MockValkeyClient) Get(key string) (string, error) {
	args := m.Called(key)
	return args.String(0), args.Error(1)
}

func (m *MockValkeyClient) Delete(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

func TestNewClientQueue(t *testing.T) {
	mockValkey := new(MockValkeyClient)
	ttlSeconds := 3600

	queue := NewClientQueue(mockValkey, ttlSeconds)

	assert.NotNil(t, queue)
	assert.Equal(t, mockValkey, queue.valkey)
	assert.Equal(t, time.Duration(ttlSeconds)*time.Second, queue.ttl)
}

func TestClientQueue_AddClient(t *testing.T) {
	t.Run("successful add", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		client := &models.ConnectedClient{
			OwnerID:          "owner123",
			ApiKey:           "api-key-123",
			ConnectedAt:      time.Now(),
			SubscribedEvents: []string{"event1", "event2"},
		}

		expectedKey := "client:owner123"
		mockValkey.On("Set", expectedKey, mock.Anything, 3600*time.Second).Return(nil)

		err := queue.AddClient(client)

		assert.NoError(t, err)
		mockValkey.AssertExpectations(t)
	})

	t.Run("valkey set fails", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		client := &models.ConnectedClient{
			OwnerID: "owner123",
			ApiKey:  "api-key-123",
		}

		mockValkey.On("Set", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("redis error"))

		err := queue.AddClient(client)

		assert.Error(t, err)
		mockValkey.AssertExpectations(t)
	})

	t.Run("verify serialization", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		now := time.Now()
		client := &models.ConnectedClient{
			OwnerID:          "owner123",
			ApiKey:           "api-key-123",
			ConnectedAt:      now,
			SubscribedEvents: []string{"event1"},
		}

		var capturedValue string
		mockValkey.On("Set", "client:owner123", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				capturedValue = args.String(1)
			}).Return(nil)

		err := queue.AddClient(client)
		assert.NoError(t, err)

		// Verify the serialized data
		var deserializedClient models.ConnectedClient
		err = json.Unmarshal([]byte(capturedValue), &deserializedClient)
		assert.NoError(t, err)
		assert.Equal(t, "owner123", deserializedClient.OwnerID)
		assert.Equal(t, "api-key-123", deserializedClient.ApiKey)
		assert.Equal(t, []string{"event1"}, deserializedClient.SubscribedEvents)
	})
}

func TestClientQueue_GetClient(t *testing.T) {
	t.Run("successful get", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		expectedClient := &models.ConnectedClient{
			OwnerID:          "owner123",
			ApiKey:           "api-key-123",
			ConnectedAt:      time.Now(),
			SubscribedEvents: []string{"event1"},
		}

		serialized, _ := json.Marshal(expectedClient)
		mockValkey.On("Get", "client:owner123").Return(string(serialized), nil)

		client, err := queue.GetClient("owner123")

		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "owner123", client.OwnerID)
		assert.Equal(t, "api-key-123", client.ApiKey)
		assert.Equal(t, []string{"event1"}, client.SubscribedEvents)
		mockValkey.AssertExpectations(t)
	})

	t.Run("client not found", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		mockValkey.On("Get", "client:owner456").Return("", errors.New("key not found"))

		client, err := queue.GetClient("owner456")

		assert.Error(t, err)
		assert.Nil(t, client)
		mockValkey.AssertExpectations(t)
	})

	t.Run("invalid JSON data", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		mockValkey.On("Get", "client:owner123").Return("invalid-json", nil)

		client, err := queue.GetClient("owner123")

		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "unmarshal client")
		mockValkey.AssertExpectations(t)
	})
}

func TestClientQueue_RemoveClient(t *testing.T) {
	t.Run("successful remove", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		mockValkey.On("Delete", "client:owner123").Return(nil)

		err := queue.RemoveClient("owner123")

		assert.NoError(t, err)
		mockValkey.AssertExpectations(t)
	})

	t.Run("delete fails", func(t *testing.T) {
		mockValkey := new(MockValkeyClient)
		queue := NewClientQueue(mockValkey, 3600)

		mockValkey.On("Delete", "client:owner123").Return(errors.New("redis error"))

		err := queue.RemoveClient("owner123")

		assert.Error(t, err)
		mockValkey.AssertExpectations(t)
	})
}

func TestClientQueue_KeyFormat(t *testing.T) {
	// Test that keys are formatted correctly
	mockValkey := new(MockValkeyClient)
	queue := NewClientQueue(mockValkey, 3600)

	testCases := []struct {
		ownerID     string
		expectedKey string
	}{
		{"owner123", "client:owner123"},
		{"user-456", "client:user-456"},
		{"abc_xyz", "client:abc_xyz"},
	}

	for _, tc := range testCases {
		t.Run(tc.ownerID, func(t *testing.T) {
			mockValkey.On("Delete", tc.expectedKey).Return(nil).Once()
			queue.RemoveClient(tc.ownerID)
			mockValkey.AssertExpectations(t)
		})
	}
}

func TestClientQueue_TTL(t *testing.T) {
	testCases := []struct {
		ttlSeconds  int
		expectedTTL time.Duration
	}{
		{3600, 3600 * time.Second},
		{7200, 7200 * time.Second},
		{60, 60 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(string(rune(tc.ttlSeconds)), func(t *testing.T) {
			mockValkey := new(MockValkeyClient)
			queue := NewClientQueue(mockValkey, tc.ttlSeconds)

			client := &models.ConnectedClient{
				OwnerID: "owner123",
				ApiKey:  "api-key",
			}

			mockValkey.On("Set", mock.Anything, mock.Anything, tc.expectedTTL).Return(nil)

			err := queue.AddClient(client)
			assert.NoError(t, err)
			mockValkey.AssertExpectations(t)
		})
	}
}
