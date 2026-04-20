package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wsConnectAndAuth is a helper that dials, connects and authenticates in one step.
func wsConnectAndAuth(t *testing.T, apiKey string) *websocket.Conn {
	t.Helper()
	conn := dialWS(t, "/threads")
	t.Cleanup(func() { conn.Close() })

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "connect",
		"apiKey": apiKey,
	})
	resp := readWSWithTimeout(t, conn, 5*time.Second)
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

	resp := doWithAuth(t, "POST", "/v1/contracts", []byte(yaml), "text/plain", token)
	require.Equal(t, 200, resp.StatusCode, "failed to create contract: %s", resp.Body)
	return contractName
}

func TestWebSocket_Auth_InvalidAPIKey(t *testing.T) {
	conn := dialWS(t, "/threads")
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "connect",
		"apiKey": "tf_invalid_key_" + uuid.NewString(),
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"], "error response must include a message")
}

func TestWebSocket_Auth_EmptyAPIKey(t *testing.T) {
	conn := dialWS(t, "/threads")
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "connect",
		"apiKey": "",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_Auth_UnauthenticatedAction(t *testing.T) {
	conn := dialWS(t, "/threads")
	defer conn.Close()

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": "any_contract",
		"role":         "actor1",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "authenticated",
		"unauthenticated action must mention authentication requirement")
}

func TestWebSocket_UnknownAction(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "totally_unknown_action",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Unknown action",
		"unknown action must be reported clearly")
	assert.Equal(t, "totally_unknown_action", resp["action"],
		"error response must echo back the unknown action")
}

func TestWebSocket_MalformedJSON(t *testing.T) {
	conn := dialWS(t, "/threads")
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(1, []byte(`{"action": "connect", "apiKey": `)))

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Invalid JSON payload")
}

func TestWebSocket_StartThread_NonexistentContract(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": "does_not_exist_" + uuid.NewString(),
		"role":         "actor1",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "nonexistent_role",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_StartThread_MissingContractName(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "startThread",
		"role":   "actor1",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

// ── recordThreadEvent ─────────────────────────────────────────────────────────

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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	sendWSJSON(t, conn, map[string]interface{}{
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

	resp := readWSWithTimeout(t, conn, 5*time.Second)
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Try to record stepB before stepA
	sendWSJSON(t, conn, map[string]interface{}{
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

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"recording a step before its entry point should fail")
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_RecordEvent_FakeThreadID(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
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

	resp := readWSWithTimeout(t, conn, 5*time.Second)
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
	sendWSJSON(t, conn1, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn1, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Record stepA correctly first
	sendWSJSON(t, conn1, map[string]interface{}{
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
	readWSWithTimeout(t, conn1, 5*time.Second)

	// actor1 tries to record stepB which belongs to actor2
	sendWSJSON(t, conn1, map[string]interface{}{
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

	resp := readWSWithTimeout(t, conn1, 5*time.Second)
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
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

	sendWSJSON(t, conn, recordStepA)
	firstResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", firstResp["status"])

	// Record the same step again
	sendWSJSON(t, conn, recordStepA)
	dupResp := readWSWithTimeout(t, conn, 5*time.Second)
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":   "inviteParty",
		"threadId": threadID,
		"role":     "nonexistent_role",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"inviting a role not in the contract must fail")
	assert.NotEmpty(t, resp["message"])
}

// Failed
func TestWebSocket_InviteParty_DuplicateRole(t *testing.T) {
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	invite := map[string]interface{}{
		"action":   "inviteParty",
		"threadId": threadID,
		"role":     "actor2",
	}

	sendWSJSON(t, conn, invite)
	firstResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", firstResp["status"])

	// Invite same role again
	sendWSJSON(t, conn, invite)
	dupResp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", dupResp["status"],
		"inviting the same role twice must be rejected")
	assert.NotEmpty(t, dupResp["message"])
}

func TestWebSocket_JoinThread_NonexistentThread(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":      "joinThread",
		"threadId":    uuid.NewString(),
		"threadToken": "fake_token",
		"role":        "actor2",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":      "joinThread",
		"threadId":    threadID,
		"threadToken": "invalid_token_" + uuid.NewString(),
		"role":        "actor2",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.NotEmpty(t, resp["message"])
}

// failed
func TestWebSocket_JoinThread_WrongRole(t *testing.T) {
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Invite actor2 and get token
	sendWSJSON(t, conn, map[string]interface{}{
		"action":   "inviteParty",
		"threadId": threadID,
		"role":     "actor2",
	})
	inviteResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", inviteResp["status"])
	inviteToken := inviteResp["threadToken"].(string)

	// Try to join with wrong role using the actor2 token
	db := newDBHelpers(env.Postgres.Pool)
	user2ID := uuid.NewString()
	user2Email := "user2_" + uuid.NewString()[:8] + "@example.com"
	db.CreateTestUser(t, user2ID, user2Email, user.CompanyID)
	saID2 := uuid.NewString()
	db.CreateTestServiceAccount(t, saID2, "SA2", user.CompanyID)
	user2ApiKey := db.CreateTestAPIKey(t, saID2, user.CompanyID, "tf_")
	db.AssignRole(t, saID2, "service_account", "actor2")

	conn2 := wsConnectAndAuth(t, user2ApiKey)
	sendWSJSON(t, conn2, map[string]interface{}{
		"action":      "joinThread",
		"threadId":    threadID,
		"threadToken": inviteToken,
		"role":        "actor1", // wrong role — token is for actor2
	})

	resp := readWSWithTimeout(t, conn2, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"joining with wrong role must be rejected")
	assert.NotEmpty(t, resp["message"])
}

func TestWebSocket_ThreadEnd_MissingThreadID(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "threadEnd",
		"status": "cancelled",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "invalid_status",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Status must be 'cancelled' or 'completed'")
}

// failed
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

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	endMsg := map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "cancelled",
	}

	sendWSJSON(t, conn, endMsg)
	firstResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", firstResp["status"])

	// End the same thread again
	sendWSJSON(t, conn, endMsg)
	secondResp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", secondResp["status"],
		"ending an already-ended thread must be rejected")
	assert.NotEmpty(t, secondResp["message"])
}

// failed
func TestWebSocket_ThreadEnd_NotOwner(t *testing.T) {
	user := setupTestUser(t)
	contractName := wsSetupContract(t, user.Token,
		[]string{"actor1"},
		[]map[string]interface{}{
			{"id": "stepA", "owner": "actor1", "type": "managed"},
		},
		[]map[string]interface{}{},
		[]string{"stepA"}, []string{"stepA"},
	)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         "actor1",
	})
	startResp := readWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", startResp["status"])
	threadID := startResp["threadId"].(string)

	// Create a second user and try to end user1's thread
	db := newDBHelpers(env.Postgres.Pool)
	user2ID := uuid.NewString()
	user2Email := "user2_" + uuid.NewString()[:8] + "@example.com"
	db.CreateTestUser(t, user2ID, user2Email, user.CompanyID)
	saID2 := uuid.NewString()
	db.CreateTestServiceAccount(t, saID2, "SA2", user.CompanyID)
	user2ApiKey := db.CreateTestAPIKey(t, saID2, user.CompanyID, "tf_")
	db.AssignRole(t, saID2, "service_account", "actor1")

	conn2 := wsConnectAndAuth(t, user2ApiKey)
	sendWSJSON(t, conn2, map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "cancelled",
	})

	resp := readWSWithTimeout(t, conn2, 5*time.Second)
	assert.Equal(t, "error", resp["status"],
		"non-owner must not be able to end another user's thread")
	assert.NotEmpty(t, resp["message"])
}

// ── subscribe / unsubscribe ───────────────────────────────────────────────────

func TestWebSocket_Subscribe_InvalidFormat(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	for _, badStep := range []string{"@@", "@", "contract@", "@step"} {
		sendWSJSON(t, conn, map[string]interface{}{
			"action":   "subscribe",
			"stepName": badStep,
		})
		resp := readWSWithTimeout(t, conn, 5*time.Second)
		assert.Equal(t, "error", resp["status"],
			"invalid subscribe format %q should return error", badStep)
		assert.Contains(t, resp["message"], "Invalid format",
			"error message should describe the format requirement")
	}
}

func TestWebSocket_Unsubscribe_MissingStepName(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action":   "unsubscribe",
		"stepName": "",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "error", resp["status"])
	assert.Contains(t, resp["message"], "Step name is required")
}

// ── closeConnection ───────────────────────────────────────────────────────────

func TestWebSocket_CloseConnection(t *testing.T) {
	user := setupTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	sendWSJSON(t, conn, map[string]interface{}{
		"action": "closeConnection",
	})

	resp := readWSWithTimeout(t, conn, 5*time.Second)
	assert.Equal(t, "success", resp["status"])
	assert.Equal(t, "closeConnection", resp["action"])
	assert.Contains(t, resp["message"], "closed")

	// Connection should be closed after this — further reads should fail
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "connection should be closed after closeConnection action")
}
