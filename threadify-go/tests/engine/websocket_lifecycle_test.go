package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebSocket_ExecutionSuccess_NotificationReceived verifies that a step recorded with status "success"
// emits an execution.success notification that can be subscribed to.
func TestWebSocket_ExecutionSuccess_NotificationReceived(t *testing.T) {
	user := setupTestUser(t)

	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
			{"id": "stepB", "owner": "actor1", "type": "managed"},
		},
		[]map[string]interface{}{{"from": "stepA", "to": []string{"stepB"}}},
		[]string{"stepA"}, []string{"stepB"},
	)

	conn := wsConnectAndAuth(t, user.ApiKey)
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepA",
		"eventTypes": []interface{}{"execution.success"},
	})
	subResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	// Allow consumer filter update to propagate
	time.Sleep(2 * time.Second)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var notification map[string]interface{}
	err := conn.ReadJSON(&notification)
	require.NoError(t, err, "should receive execution.success notification")

	assert.Equal(t, "notification", notification["action"])
	assert.NotEmpty(t, notification["ackToken"])

	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok, "notification payload should be a map")

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepA", notifPayload["stepName"])
	assert.Equal(t, "execution", notifPayload["source"])
	assert.Equal(t, "execution.success", notifPayload["notificationType"])
	assert.Equal(t, "success", notifPayload["stepStatus"])
	assert.Equal(t, "none", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "info", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "execution success")

	// Ack if token present
	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		sendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := readWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}

func TestWebSocket_ExecutionFailed_NotificationReceived(t *testing.T) {
	user := setupTestUser(t)

	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
			{"id": "stepB", "owner": "actor1", "type": "managed"},
		},
		[]map[string]interface{}{{"from": "stepA", "to": []string{"stepB"}}},
		[]string{"stepA"}, []string{"stepB"},
	)

	conn := wsConnectAndAuth(t, user.ApiKey)
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepA",
		"eventTypes": []interface{}{"execution.failed"},
	})
	subResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(2 * time.Second)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "failed",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var notification map[string]interface{}
	err := conn.ReadJSON(&notification)
	require.NoError(t, err, "should receive execution.failed notification")

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepA", notifPayload["stepName"])
	assert.Equal(t, "execution", notifPayload["source"])
	assert.Equal(t, "execution.failed", notifPayload["notificationType"])
	assert.Equal(t, "failed", notifPayload["stepStatus"])
	assert.Equal(t, "none", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "info", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "execution failed")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		sendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := readWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}

func TestWebSocket_ValidationPassed_NotificationReceived(t *testing.T) {
	user := setupTestUser(t)

	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
			{"id": "stepB", "owner": "actor1", "type": "managed"},
		},
		[]map[string]interface{}{{"from": "stepA", "to": []string{"stepB"}}},
		[]string{"stepA"}, []string{"stepB"},
	)

	conn := wsConnectAndAuth(t, user.ApiKey)
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepA",
		"eventTypes": []interface{}{"validation.passed"},
	})
	subResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(2 * time.Second)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var notification map[string]interface{}
	err := conn.ReadJSON(&notification)
	require.NoError(t, err, "should receive validation.passed notification")

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepA", notifPayload["stepName"])
	assert.Equal(t, "validation", notifPayload["source"])
	assert.Equal(t, "validation.passed", notifPayload["notificationType"])
	assert.Equal(t, "success", notifPayload["stepStatus"])
	assert.Equal(t, "passed", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "completed successfully")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		sendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := readWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}

func TestWebSocket_ThreadCancelled_NotificationReceived(t *testing.T) {
	user := setupTestUser(t)

	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
			{"id": "stepB", "owner": "actor1", "type": "managed"},
		},
		[]map[string]interface{}{{"from": "stepA", "to": []string{"stepB"}}},
		[]string{"stepA"}, []string{"stepB"},
	)

	conn := wsConnectAndAuth(t, user.ApiKey)
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "global",
		"eventTypes": []interface{}{"thread.cancelled"},
	})
	subResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(2 * time.Second)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "cancelled",
		"reason":   "test cancellation",
	})
	endResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", endResp["status"])

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var notification map[string]interface{}
	err := conn.ReadJSON(&notification)
	require.NoError(t, err, "should receive thread.cancelled notification")

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "global", notifPayload["stepName"])
	assert.Equal(t, "thread", notifPayload["source"])
	assert.Equal(t, "thread.cancelled", notifPayload["notificationType"])
	assert.Equal(t, "cancelled", notifPayload["stepStatus"])
	assert.Equal(t, "none", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "info", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "cancelled")
	assert.Contains(t, notifPayload["message"], "test cancellation")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		sendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := readWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}
