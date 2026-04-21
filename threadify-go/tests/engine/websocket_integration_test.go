package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

// wsConnectAndAuth is a helper that dials, connects and authenticates in one step.
func wsConnectAndAuth(t *testing.T, apiKey string) *websocket.Conn {
	t.Helper()
	conn := httpc.DialWS(t, "/threads")
	t.Cleanup(func() { conn.Close() })

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "connect",
		"apiKey": apiKey,
	})
	resp := enginetest.ReadWSWithTimeout(t, conn, 10*time.Second)
	require.Equal(t, "success", resp["status"], "auth failed: %v", resp)
	return conn
}

// wsSetupContract creates a contract and returns its name.
func wsSetupContract(t *testing.T, token string, parties []string, steps []map[string]interface{}, transitions []map[string]interface{}, entryPoints, terminalSteps []string) string {
	t.Helper()
	contractName := "ws_contract_" + uuid.NewString()[:8]

	partiesYAML := ""
	for _, p := range parties {
		partiesYAML += p + ", "
	}

	stepsYAML := ""
	for _, s := range steps {
		stepsYAML += fmt.Sprintf("  - id: %s\n    owner: %s\n    type: %s\n", s["id"], s["owner"], s["type"])
	}

	transitionsYAML := ""
	for _, tr := range transitions {
		toList := ""
		for _, to := range tr["to"].([]string) {
			toList += to + ", "
		}
		transitionsYAML += fmt.Sprintf("  - from: %s\n    to: [%s]\n", tr["from"], toList)
	}

	entryYAML := ""
	for _, e := range entryPoints {
		entryYAML += e + ", "
	}
	terminalYAML := ""
	for _, te := range terminalSteps {
		terminalYAML += te + ", "
	}

	yaml := fmt.Sprintf(`
contract_name: %s
version: 1
description: test contract
parties: [%s]
steps:
%s
transitions:
%s
entry_points: [%s]
terminal_steps: [%s]
`, contractName, partiesYAML, stepsYAML, transitionsYAML, entryYAML, terminalYAML)

	resp := httpc.DoWithAuth(t, "POST", "/v1/contracts", []byte(yaml), "text/plain", token)
	require.Equal(t, 200, resp.StatusCode, "failed to create contract: %s", resp.Body)
	return contractName
}

func wsSetupContractWithValidation(t *testing.T, token string, parties []string, steps []map[string]interface{}, transitions []map[string]interface{}, entryPoints, terminalSteps []string, maxDuration string) string {
	t.Helper()
	contractName := "ws_contract_" + uuid.NewString()[:8]

	partiesYAML := ""
	for _, p := range parties {
		partiesYAML += p + ", "
	}

	stepsYAML := ""
	for _, s := range steps {
		stepsYAML += fmt.Sprintf("  - id: %s\n    owner: %s\n    type: %s\n", s["id"], s["owner"], s["type"])
	}

	transitionsYAML := ""
	for _, tr := range transitions {
		toList := ""
		for _, to := range tr["to"].([]string) {
			toList += to + ", "
		}
		transitionsYAML += fmt.Sprintf("  - from: %s\n    to: [%s]\n", tr["from"], toList)
	}

	entryYAML := ""
	for _, e := range entryPoints {
		entryYAML += e + ", "
	}
	terminalYAML := ""
	for _, te := range terminalSteps {
		terminalYAML += te + ", "
	}

	validationYAML := ""
	if maxDuration != "" {
		validationYAML = fmt.Sprintf("validation:\n  max_duration: %s\n", maxDuration)
	}

	yaml := fmt.Sprintf(`
contract_name: %s
version: 1
description: test contract
parties: [%s]
steps:
%s
transitions:
%s
entry_points: [%s]
terminal_steps: [%s]
%s`, contractName, partiesYAML, stepsYAML, transitionsYAML, entryYAML, terminalYAML, validationYAML)

	resp := httpc.DoWithAuth(t, "POST", "/v1/contracts", []byte(yaml), "text/plain", token)
	require.Equal(t, 200, resp.StatusCode, "failed to create contract: %s", resp.Body)
	return contractName
}

func TestWebSocket_Auth_InvalidAPIKey(t *testing.T) {
	conn := httpc.DialWS(t, "/threads")
	defer conn.Close()

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "connect",
		"apiKey": "tf_invalid_key_" + uuid.NewString(),
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"], "error response must include a message")
}

func TestWebSocket_Auth_EmptyAPIKey(t *testing.T) {
	conn := httpc.DialWS(t, "/threads")
	defer conn.Close()

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "connect",
		"apiKey": "",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_Auth_UnauthenticatedAction(t *testing.T) {
	conn := httpc.DialWS(t, "/threads")
	defer conn.Close()

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": "any_contract",
		"role":         "actor1",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "authenticated",
		"unauthenticated action must mention authentication requirement")
}

func TestWebSocket_UnknownAction(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "totally_unknown_action",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Unknown action",
		"unknown action must be reported clearly")
	assert.Equal(t, "totally_unknown_action", resp["action"],
		"error response must echo back the unknown action")
}

func TestWebSocket_MalformedJSON(t *testing.T) {
	conn := httpc.DialWS(t, "/threads")
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(1, []byte(`{"action": "connect", "apiKey": `)))

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Invalid JSON payload")
}

func TestWebSocket_StartThread_NonexistentContract(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": "does_not_exist_" + uuid.NewString(),
		"role":         "actor1",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Failed to load contract",
		"nonexistent contract must return descriptive error")
}

func TestWebSocket_StartThread_InvalidRole(t *testing.T) {
	user := setupTestUser(t)
	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{{"id": "stepA", "owner": "actor1", "type": "managed"}},
		[]map[string]interface{}{},
		[]string{"stepA"}, []string{"stepA"},
	)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "nonexistent_role",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_RecordEvent_MissingContext(t *testing.T) {
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepA",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		// no context
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Context is required")
}

func TestWebSocket_RecordEvent_OutOfOrder(t *testing.T) {
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Try to record stepB before stepA
	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepB",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"recording a step before its entry point should fail")
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_RecordEvent_FakeThreadID(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   uuid.NewString(),
		"status":     "success",
		"stepName":   "stepA",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "not found")
}

func TestWebSocket_RecordEvent_WrongRole(t *testing.T) {
	user := setupTestUser(t)
	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1", "actor2"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
			{"id": "stepB", "owner": "actor2", "type": "managed"},
		},
		[]map[string]interface{}{{"from": "stepA", "to": []string{"stepB"}}},
		[]string{"stepA"}, []string{"stepB"},
	)

	// actor1 starts thread
	conn1 := wsConnectAndAuth(t, user.ApiKey)
	enginetest.SendWSJSON(t, conn1, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn1, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Record stepA correctly first
	enginetest.SendWSJSON(t, conn1, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepA",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	enginetest.ReadWSWithTimeout(t, conn1, 5*time.Second)

	// actor1 tries to record stepB which belongs to actor2
	enginetest.SendWSJSON(t, conn1, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepB",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})

	resp := enginetest.ReadWSWithTimeout(t, conn1, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"actor1 must not be able to record a step owned by actor2")
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_RecordEvent_DuplicateStep(t *testing.T) {
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	recordStepA := map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepA",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	}

	enginetest.SendWSJSON(t, conn, recordStepA)
	firstResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", firstResp["status"])

	// Record the same step again
	enginetest.SendWSJSON(t, conn, recordStepA)
	dupResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", dupResp["status"],
		"duplicate step recording must be rejected")
	assert.NotEmpty(t, dupResp["message"])
}

func TestWebSocket_InviteParty_NonexistentRole(t *testing.T) {
	user := setupTestUser(t)
	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{{"id": "stepA", "owner": "actor1", "type": "managed"}},
		[]map[string]interface{}{},
		[]string{"stepA"}, []string{"stepA"},
	)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":   "inviteParty",
		"threadId": threadID,
		"role":     "nonexistent_role",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"inviting a role not in the contract must fail")
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_JoinThread_NonexistentThread(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":      "joinThread",
		"threadId":    uuid.NewString(),
		"threadToken": "fake_token",
		"role":        "actor2",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_JoinThread_InvalidToken(t *testing.T) {
	user := setupTestUser(t)
	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1", "actor2"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
			{"id": "stepB", "owner": "actor2", "type": "managed"},
		},
		[]map[string]interface{}{{"from": "stepA", "to": []string{"stepB"}}},
		[]string{"stepA"}, []string{"stepB"},
	)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":      "joinThread",
		"threadId":    threadID,
		"threadToken": "invalid_token_" + uuid.NewString(),
		"role":        "actor2",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_ThreadEnd_MissingThreadID(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "threadEnd",
		"status": "cancelled",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Thread ID is required")
}

func TestWebSocket_ThreadEnd_InvalidStatus(t *testing.T) {
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "invalid_status",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Status must be 'cancelled' or 'completed'")
}

func TestWebSocket_ThreadEnd_AlreadyEnded(t *testing.T) {
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

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	endMsg := map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "cancelled",
	}

	enginetest.SendWSJSON(t, conn, endMsg)
	firstResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", firstResp["status"])

	// End the same thread again
	enginetest.SendWSJSON(t, conn, endMsg)
	secondResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	t.Logf("secondResp %s", secondResp)
	assert.Equal(t, "error", secondResp["status"],
		"ending an already-ended thread must be rejected")
	assert.NotEmpty(t, secondResp["message"])
}

func TestWebSocket_Subscribe_InvalidFormat(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	for _, badStep := range []string{"@@", "@", "contract@", "@step"} {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "subscribe",
			"stepName": badStep,
		})
		resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "error", resp["status"],
			"invalid subscribe format %q should return error", badStep)
		assert.Contains(t, resp["message"], "Invalid format",
			"error message should describe the format requirement")
	}
}

func TestWebSocket_Unsubscribe_MissingStepName(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":   "unsubscribe",
		"stepName": "",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Step name is required")
}

func TestWebSocket_CloseConnection(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "closeConnection",
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "success", resp["status"])
	assert.Equal(t, "closeConnection", resp["action"])
	assert.Contains(t, resp["message"], "closed")

	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "connection should be closed after closeConnection action")
}

func TestWebSocket_ThreadTimeout_NotificationReceived(t *testing.T) {
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
		"2s", // max_duration: 2 seconds
	)

	conn := wsConnectAndAuth(t, user.ApiKey)
	defer conn.Close()

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "subscribe",
		"stepName":   "global",
		"eventTypes": []interface{}{"rule.violated.timeout"},
	})
	subResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", subResp["status"], "subscribe should succeed")

	// Allow consumer filter update to propagate. This is sensitive when running
	// the full suite and the timeout event can fire before the subscription is active.
	time.Sleep(4 * time.Second)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)
	require.NotEmpty(t, threadID)

	notification := enginetest.ReadWSUntil(t, conn, 30*time.Second, func(msg map[string]interface{}) bool {
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

	assert.Equal(t, "notification", notification["action"], "action should be 'notification'")
	assert.NotEmpty(t, notification["ackToken"], "ackToken should be present")

	notifPayload, ok := notification["notification"].(map[string]interface{})
	require.True(t, ok, "notification payload should be a map")

	assert.Equal(t, threadID, notifPayload["threadId"], "notification should be for the correct thread")
	assert.Equal(t, "rule", notifPayload["source"], "source should be 'rule'")
	assert.Equal(t, "rule.violated.timeout", notifPayload["notificationType"], "notificationType should indicate timeout violation")
	assert.Equal(t, "timeout", notifPayload["violationType"], "violationType should be 'timeout'")
	assert.Equal(t, "critical", notifPayload["severity"], "severity should be 'critical'")
	assert.Equal(t, "global", notifPayload["stepName"], "stepName should be 'global' for thread-level timeouts")
	assert.Equal(t, "violated", notifPayload["status"], "status should be 'violated'")
	assert.Contains(t, notifPayload["message"], "max_duration", "message should mention max_duration")

	details, ok := notifPayload["details"].(map[string]interface{})
	require.True(t, ok, "details should be a map")
	assert.Equal(t, "max_duration", details["timeout_type"], "timeout_type should be 'max_duration'")
	assert.NotEmpty(t, details["timeout"], "timeout value should be present")

	ackToken, _ := notification["ackToken"].(string)
	if ackToken != "" {
		enginetest.SendWSJSON(t, conn, map[string]interface{}{
			"action":   "ack_notification",
			"ackToken": ackToken,
		})
		ackResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "success", ackResp["status"], "ack should succeed")
	}
}

func TestWebSocket_RecordEvent_AfterThreadEnded(t *testing.T) {
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
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Record stepA — non-terminal, should succeed.
	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepA",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	stepAResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", stepAResp["status"], "stepA record should succeed")

	// Wait for stepA validation and state propagation to complete before
	// sending stepB. Without this the server may reject stepB as out-of-order.
	time.Sleep(500 * time.Millisecond)

	// Record stepB — the terminal step, should succeed and auto-complete the thread.
	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepB",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	stepBResp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", stepBResp["status"], "stepB (terminal) record should succeed")

	// Wait for the thread's completed status to be persisted before probing it.
	time.Sleep(500 * time.Millisecond)

	// Attempt to record stepB again on a now-completed thread — must be rejected.
	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":     "recordThreadEvent",
		"threadId":   threadID,
		"status":     "success",
		"stepName":   "stepB",
		"type":       "managed",
		"actor":      "actor1",
		"startedAt":  time.Now().Add(-time.Minute).Format(time.RFC3339),
		"finishedAt": time.Now().Format(time.RFC3339),
		"context":    map[string]string{},
	})
	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	t.Logf("resp %v", resp)
	assert.Equal(t, "error", resp["status"], "recording a step after thread is ended must be rejected")
	assert.NotEmpty(t, resp["message"])
}
