package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_ThreadNotifications_Success(t *testing.T) {
	user := setupTestUser(t)
	contractName := "notif_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	insertNotif := func(id, severity, msg string) {
		payload := map[string]interface{}{
			"notification_id":   id,
			"source":            "validation",
			"notification_type": "rule_violated",
			"severity":          severity,
			"message":           msg,
			"details":           "{}",
		}
		payloadJSON, _ := json.Marshal(payload)

		_, err := env.Postgres.Pool.Exec(context.Background(), `
			INSERT INTO thread_activities (id, thread_id, activity_type, step_id, status, payload, recorded_at, actor, actor_service)
			VALUES ($1, $2, 'validation_result', $3, 'violated', $4, NOW(), 'test_actor', 'test_service')
		`, uuid.NewString(), threadID, "step1:idemp1", payloadJSON)
		require.NoError(t, err)
	}

	insertNotif("notif_1", "critical", "Critical Error 1")
	insertNotif("notif_2", "critical", "Critical Error 2")
	insertNotif("notif_3", "warning", "Warning 1")

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `query($id: ID!) { 
			thread(id: $id) { 
				notificationSummary { 
					totalNotifications 
					criticalCount 
					warningCount 
					hasCritical 
					hasWarnings 
				} 
			} 
		}`,
		Variables: map[string]interface{}{"id": threadID},
	})

	require.Empty(t, gqlResp.Errors)
	thread, ok := gqlResp.Data["thread"].(map[string]interface{})
	require.True(t, ok)
	summary := thread["notificationSummary"].(map[string]interface{})

	require.Equal(t, 3.0, summary["totalNotifications"])
	require.Equal(t, 2.0, summary["criticalCount"])
	require.Equal(t, 1.0, summary["warningCount"])
	require.True(t, summary["hasCritical"].(bool))
	require.True(t, summary["hasWarnings"].(bool))

	// 3. Query Full Notifications
	_, gqlResp = doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `query($id: ID!) { 
			thread(id: $id) { 
				notifications { 
					notificationId 
					severity 
					message 
				} 
			} 
		}`,
		Variables: map[string]interface{}{"id": threadID},
	})

	require.Empty(t, gqlResp.Errors)
	thread, ok = gqlResp.Data["thread"].(map[string]interface{})
	require.True(t, ok)
	notifications := thread["notifications"].([]interface{})
	require.Len(t, notifications, 3)

	_, gqlResp = doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `query($id: ID!, $severity: [String!]) { 
			thread(id: $id) { 
				notifications(options: { severity: $severity }) { 
					notificationId 
					severity 
				} 
			} 
		}`,
		Variables: map[string]interface{}{
			"id":       threadID,
			"severity": []string{"critical"},
		},
	})

	require.Empty(t, gqlResp.Errors)
	thread, ok = gqlResp.Data["thread"].(map[string]interface{})
	require.True(t, ok)
	notifications = thread["notifications"].([]interface{})
	require.Len(t, notifications, 2)
	for _, it := range notifications {
		n := it.(map[string]interface{})
		require.Equal(t, "critical", n["severity"])
	}
}
