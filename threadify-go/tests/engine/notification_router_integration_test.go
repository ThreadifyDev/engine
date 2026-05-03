package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/handlers"
	"go.uber.org/zap"
)

type mockWSConnection struct {
	writes [][]byte
	mu     sync.Mutex
}

func (m *mockWSConnection) WriteJSON(v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	m.writes = append(m.writes, data)
	return nil
}

type mockWSMutex struct {
	mu sync.Mutex
}

func (m *mockWSMutex) Lock()   { m.mu.Lock() }
func (m *mockWSMutex) Unlock() { m.mu.Unlock() }

type wsEnvelope struct {
	Action       string                        `json:"action"`
	AckToken     string                        `json:"ackToken"`
	Notification domain.ValidationNotification `json:"notification"`
}

func natsURI() string {
	if uri := os.Getenv("NATS_URI"); uri != "" {
		return uri
	}
	return nats.DefaultURL
}

func TestNotificationRouter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	nc, err := nats.Connect(natsURI())
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	natsCfg := engineApp.NATSConfig()

	ctx := context.Background()

	_, err = js.Stream(ctx, natsCfg.StreamName)
	require.NoError(t, err, "stream %s should already exist (created by engineApp)", natsCfg.StreamName)

	router, err := handlers.NewNotificationRouter(nc, natsCfg, logger)
	require.NoError(t, err)

	ownerID := uuid.NewString()
	sessionID := uuid.NewString()
	mockConn := &mockWSConnection{}
	mockMutex := &mockWSMutex{}

	t.Run("HandleConnect", func(t *testing.T) {
		err := router.HandleConnect(sessionID, ownerID, 10, mockConn, mockMutex)
		assert.NoError(t, err)

		consumerName := fmt.Sprintf("owner-%s", ownerID)
		_, err = js.Consumer(ctx, natsCfg.StreamName, consumerName)
		assert.NoError(t, err)
	})

	t.Run("HandleSubscribe", func(t *testing.T) {
		err := router.HandleSubscribe(sessionID, "test_step", "test_contract", []string{"validation.violated.timeout"})
		assert.NoError(t, err)

		// Allow the consumer update to propagate.
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("RouteNotification", func(t *testing.T) {
		stepName := "test_step"
		contractName := "test_contract"
		threadID := uuid.NewString()

		notification := domain.ValidationNotification{
			ThreadID:         threadID,
			OwnerID:          ownerID,
			StepName:         stepName,
			ContractName:     contractName,
			Source:           domain.NotificationSourceRule,
			NotificationType: "validation.violated.timeout",
			Status:           "violated",
			ViolationType:    "timeout",
			Severity:         "critical",
			Timestamp:        time.Now(),
			Message:          "Test message",
		}

		data, err := json.Marshal(notification)
		require.NoError(t, err)

		msgSubject := fmt.Sprintf("notifications.user.%s.validation.violated.timeout.%s.%s", ownerID, contractName, stepName)
		_, err = js.Publish(ctx, msgSubject, data)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			mockConn.mu.Lock()
			defer mockConn.mu.Unlock()
			return len(mockConn.writes) > 0
		}, 5*time.Second, 50*time.Millisecond)

		mockConn.mu.Lock()
		raw := mockConn.writes[0]
		mockConn.mu.Unlock()

		t.Logf("raw WS message: %s", string(raw))

		var envelope wsEnvelope
		require.NoError(t, json.Unmarshal(raw, &envelope))

		assert.Equal(t, "notification", envelope.Action)
		assert.NotEmpty(t, envelope.AckToken, "ackToken should be present")

		n := envelope.Notification
		assert.Equal(t, threadID, n.ThreadID)
		assert.Equal(t, stepName, n.StepName)
		assert.Equal(t, contractName, n.ContractName)
		assert.Equal(t, "violated", n.Status)
		assert.Equal(t, domain.NotificationType("validation.violated.timeout"), n.NotificationType)

		require.NoError(t, router.HandleAck(envelope.AckToken))
	})

	t.Run("HandleDisconnect", func(t *testing.T) {
		err := router.HandleDisconnect(sessionID)
		assert.NoError(t, err)

		consumerName := fmt.Sprintf("owner-%s", ownerID)
		_, err = js.Consumer(ctx, natsCfg.StreamName, consumerName)
		assert.Error(t, err, "consumer should have been deleted after last session disconnects")
	})
}
