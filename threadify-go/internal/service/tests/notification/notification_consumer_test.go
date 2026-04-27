package service_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/types"
	"go.uber.org/zap"
)

type mockNATSClient struct {
	fetchFn func(subject, consumerName string, timeout time.Duration) (*types.NATSMessage, error)
}

func (m *mockNATSClient) FetchMessage(subject, consumerName string, timeout time.Duration) (*types.NATSMessage, error) {
	return m.fetchFn(subject, consumerName, timeout)
}

type mockNotificationHandler struct {
	handleFn func(notification models.ValidationNotification) error
}

func (m *mockNotificationHandler) HandleNotification(notification models.ValidationNotification) error {
	return m.handleFn(notification)
}

func TestNotificationConsumer_Subscribe(t *testing.T) {
	client := &mockNATSClient{
		fetchFn: func(subject, consumerName string, timeout time.Duration) (*types.NATSMessage, error) {
			return nil, errors.New("timeout")
		},
	}
	logger := zap.NewNop()
	nc := service.NewNotificationConsumer(client, nil, logger)
	defer nc.Stop()

	handler := &mockNotificationHandler{}

	t.Run("first subscription success", func(t *testing.T) {
		err := nc.Subscribe("t1", "u1", "owner", handler)
		assert.NoError(t, err)
		assert.Equal(t, 1, nc.GetActiveSubscriptions())
	})

	t.Run("duplicate subscription failure", func(t *testing.T) {
		err := nc.Subscribe("t1", "u1", "owner", handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already subscribed")
	})
}

func TestNotificationConsumer_Unsubscribe(t *testing.T) {
	client := &mockNATSClient{
		fetchFn: func(subject, consumerName string, timeout time.Duration) (*types.NATSMessage, error) {
			return nil, errors.New("timeout")
		},
	}
	logger := zap.NewNop()
	nc := service.NewNotificationConsumer(client, nil, logger)
	defer nc.Stop()

	handler := &mockNotificationHandler{}
	nc.Subscribe("t1", "u1", "owner", handler)

	t.Run("success", func(t *testing.T) {
		err := nc.Unsubscribe("t1", "u1")
		assert.NoError(t, err)
		assert.Equal(t, 0, nc.GetActiveSubscriptions())
	})

	t.Run("not found", func(t *testing.T) {
		err := nc.Unsubscribe("t1", "u1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no subscription found")
	})
}

func TestNotificationConsumer_Delivery(t *testing.T) {
	notif := models.ValidationNotification{
		NotificationID: "n1",
		ThreadID:       "t1",
		Message:        "test",
	}
	data, _ := json.Marshal(notif)

	delivered := make(chan models.ValidationNotification, 1)
	handler := &mockNotificationHandler{
		handleFn: func(notification models.ValidationNotification) error {
			delivered <- notification
			return nil
		},
	}

	fetchCount := 0
	client := &mockNATSClient{
		fetchFn: func(subject, consumerName string, timeout time.Duration) (*types.NATSMessage, error) {
			fetchCount++
			if fetchCount == 1 {
				return &types.NATSMessage{Data: data}, nil
			}
			// Block subsequent calls to avoid spin loop in test
			time.Sleep(100 * time.Millisecond)
			return nil, errors.New("timeout")
		},
	}

	logger := zap.NewNop()
	nc := service.NewNotificationConsumer(client, nil, logger)
	defer nc.Stop()

	err := nc.Subscribe("t1", "u1", "owner", handler)
	require.NoError(t, err)

	select {
	case got := <-delivered:
		assert.Equal(t, notif.NotificationID, got.NotificationID)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for notification delivery")
	}
}

func TestNotificationConsumer_Stop(t *testing.T) {
	client := &mockNATSClient{
		fetchFn: func(subject, consumerName string, timeout time.Duration) (*types.NATSMessage, error) {
			time.Sleep(50 * time.Millisecond)
			return nil, errors.New("stopped")
		},
	}
	nc := service.NewNotificationConsumer(client, nil, zap.NewNop())
	nc.Subscribe("t1", "u1", "owner", &mockNotificationHandler{})

	nc.Stop()
	assert.Equal(t, 0, nc.GetActiveSubscriptions())
}
