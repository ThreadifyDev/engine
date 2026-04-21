package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepA",
		"eventTypes": []interface{}{"step.success"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 10*time.Second)
	require.Equal(t, "success", subResp["status"])

	// Give the server a moment to register the subscription.
	time.Sleep(250 * time.Millisecond)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	deadline := time.Now().Add(15 * time.Second)
	var recordResp map[string]interface{}
	var notification map[string]interface{}
	for recordResp == nil || notification == nil {
		if time.Now().After(deadline) {
			break
		}
		msg := enginetest.ReadWSWithTimeout(t, conn, time.Until(deadline))
		if msg["action"] == "recordThreadEvent" {
			recordResp = msg
			continue
		}
		if msg["action"] == "notification" {
			payload, ok := msg["notification"].(map[string]interface{})
			if !ok {
				continue
			}
			if payload["threadId"] == threadID &&
				payload["stepName"] == "stepA" &&
				payload["notificationType"] == "step.success" {
				notification = msg
				continue
			}
		}
		if ackToken, ok := msg["ackToken"].(string); ok && ackToken != "" {
			enginetest.SendWSJSON(t, conn, map[string]interface{}{
				"action":   "ack_notification",
				"ackToken": ackToken,
			})
		}
	}
	require.NotNil(t, recordResp, "should receive recordThreadEvent response")
	require.NotNil(t, notification, "should receive execution.success notification")
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	time.Sleep(500 * time.Millisecond)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepB",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})

	assert.Equal(t, "notification", notification["action"])
	assert.NotEmpty(t, notification["ackToken"])

	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok, "notification payload should be a map")

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepA", notifPayload["stepName"])
	assert.Equal(t, "step", notifPayload["source"])
	assert.Equal(t, "step.success", notifPayload["notificationType"])
	assert.Equal(t, "success", notifPayload["stepStatus"])
	assert.Equal(t, "none", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "info", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "execution success")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepA",
		"eventTypes": []interface{}{"step.failed"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(2 * time.Second)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "failed",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp := enginetest.ReadWSAction(t, conn, "recordThreadEvent", 10*time.Second)
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	notification := enginetest.ReadWSUntil(t, conn, 10*time.Second, func(msg map[string]interface{}) bool {
		if msg["action"] != "notification" {
			return false
		}
		payload, ok := msg["notification"].(map[string]interface{})
		if !ok {
			return false
		}
		return payload["threadId"] == threadID &&
			payload["stepName"] == "stepA" &&
			payload["notificationType"] == "step.failed"
	})

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepA", notifPayload["stepName"])
	assert.Equal(t, "step", notifPayload["source"])
	assert.Equal(t, "step.failed", notifPayload["notificationType"])
	assert.Equal(t, "failed", notifPayload["stepStatus"])
	assert.Equal(t, "none", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "info", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "execution failed")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepA",
		"eventTypes": []interface{}{"rule.passed"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(2 * time.Second)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp := enginetest.ReadWSAction(t, conn, "recordThreadEvent", 10*time.Second)
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	notification := enginetest.ReadWSUntil(t, conn, 10*time.Second, func(msg map[string]interface{}) bool {
		if msg["action"] != "notification" {
			return false
		}
		payload, ok := msg["notification"].(map[string]interface{})
		if !ok {
			return false
		}
		return payload["threadId"] == threadID &&
			payload["stepName"] == "stepA" &&
			payload["notificationType"] == "rule.passed"
	})

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepA", notifPayload["stepName"])
	assert.Equal(t, "rule", notifPayload["source"])
	assert.Equal(t, "rule.passed", notifPayload["notificationType"])
	assert.Equal(t, "success", notifPayload["stepStatus"])
	assert.Equal(t, "passed", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "completed successfully")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "global",
		"eventTypes": []interface{}{"step.cancelled"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(2 * time.Second)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "cancelled",
		"reason":   "test cancellation",
	})
	endResp := enginetest.ReadWSAction(t, conn, "threadEnd", 10*time.Second)
	require.Equal(t, "success", endResp["status"])

	notification := enginetest.ReadWSUntil(t, conn, 10*time.Second, func(msg map[string]interface{}) bool {
		if msg["action"] != "notification" {
			return false
		}
		payload, ok := msg["notification"].(map[string]interface{})
		if !ok {
			return false
		}
		return payload["threadId"] == threadID &&
			payload["stepName"] == "global" &&
			payload["notificationType"] == "step.cancelled"
	})

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "global", notifPayload["stepName"])
	assert.Equal(t, "step", notifPayload["source"])
	assert.Equal(t, "step.cancelled", notifPayload["notificationType"])
	assert.Equal(t, "cancelled", notifPayload["stepStatus"])
	assert.Equal(t, "none", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "info", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "cancelled")
	assert.Contains(t, notifPayload["message"], "test cancellation")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}

func TestWebSocket_ThreadCompleted_NotificationReceived(t *testing.T) {
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "stepB",
		"eventTypes": []interface{}{"rule.passed"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	time.Sleep(3 * time.Second)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepA",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp := enginetest.ReadWSAction(t, conn, "recordThreadEvent", 10*time.Second)
	require.Equal(t, "success", recordResp["status"])
	require.Equal(t, "recordThreadEvent", recordResp["action"])

	time.Sleep(500 * time.Millisecond)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"stepName":   "stepB",
		"type":       "managed",
		"status":     "success",
		"startedAt":  time.Now().Add(-1 * time.Second).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	recordResp2 := enginetest.ReadWSAction(t, conn, "recordThreadEvent", 10*time.Second)
	require.Equal(t, "success", recordResp2["status"])
	require.Equal(t, "recordThreadEvent", recordResp2["action"])

	deadline := time.Now().Add(8 * time.Second)
	var notification map[string]interface{}
	var notifPayload map[string]interface{}
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		require.NoError(t, err, "should receive a notification")

		if msg["action"] != "notification" {
			continue
		}
		payload, ok := msg["notification"].(map[string]interface{})
		if !ok {
			continue
		}
		if payload["threadId"] == threadID &&
			payload["stepName"] == "stepB" &&
			payload["notificationType"] == "rule.passed" {
			notification = msg
			notifPayload = payload
			break
		}
		if ackToken, ok := msg["ackToken"].(string); ok && ackToken != "" {
			enginetest.SendWSJSON(t, conn, map[string]interface{}{
				"action":   "ack_notification",
				"ackToken": ackToken,
			})
		}
	}
	require.NotNil(t, notifPayload, "should have received rule.passed notification for stepB")

	assert.Equal(t, "notification", notification["action"])
	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "stepB", notifPayload["stepName"])
	assert.Equal(t, "rule", notifPayload["source"])
	assert.Equal(t, "rule.passed", notifPayload["notificationType"])
	assert.Equal(t, "success", notifPayload["stepStatus"])
	assert.Equal(t, "passed", notifPayload["status"])
	assert.Equal(t, "", notifPayload["violationType"])
	assert.Equal(t, "", notifPayload["severity"])
	msg, _ := notifPayload["message"].(string)
	assert.True(t,
		strings.Contains(msg, "completed successfully") || strings.Contains(msg, "Thread completed successfully"),
		"message should indicate successful completion, got: %s", msg,
	)

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}

func TestWebSocket_RuleViolated_NotificationReceived(t *testing.T) {
	user := setupTestUser(t)

	contractName := wsSetupContractWithValidation(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
		},
		[]map[string]interface{}{
			{"from": "stepA", "to": []string{"stepA"}},
		},
		[]string{"stepA"}, []string{"stepA"},
		"1s", // max_duration: 1 second
	)

	conn := wsConnectAndAuth(t, user.ApiKey)
	defer conn.Close()

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "global",
		"eventTypes": []interface{}{"rule.violated.timeout"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"])

	// Allow consumer filter update to propagate before starting the thread.
	// Without this sleep the timeout notification fires before the NATS
	// subscription is active and the test misses it.
	time.Sleep(2 * time.Second)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	notification := enginetest.ReadWSUntil(t, conn, 10*time.Second, func(msg map[string]interface{}) bool {
		if msg["action"] != "notification" {
			return false
		}
		payload, ok := msg["notification"].(map[string]interface{})
		if !ok {
			return false
		}
		return payload["threadId"] == threadID &&
			payload["stepName"] == "global" &&
			payload["notificationType"] == "rule.violated.timeout"
	})

	assert.Equal(t, "notification", notification["action"])
	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, threadID, notifPayload["threadId"])
	assert.Equal(t, "global", notifPayload["stepName"])
	assert.Equal(t, "rule", notifPayload["source"])
	assert.Equal(t, "rule.violated.timeout", notifPayload["notificationType"])
	assert.Equal(t, "violated", notifPayload["status"])
	assert.Equal(t, "timeout", notifPayload["violationType"])
	assert.Equal(t, "critical", notifPayload["severity"])
	assert.Contains(t, notifPayload["message"], "max_duration")

	if ackToken, ok := notification["ackToken"].(string); ok && ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"])
	}
}
