package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two independent service identities and WebSocket connections contribute through
// the public engine protocol. PostgreSQL is used only to seed identities and
// assert durable results; joins, threads and events use engine APIs.
func TestStandaloneMultiSystemJoin(t *testing.T) {
	runMultiSystemJoin(t, false)
}

func TestStandaloneContractMultiSystemJoin(t *testing.T) {
	runMultiSystemJoin(t, true)
}

func runMultiSystemJoin(t *testing.T, contractBased bool) {
	t.Helper()
	dir := standaloneDirectory(t)
	v := viper.New()
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, v.ReadInConfig())
	require.Equal(t, "127.0.0.1", v.GetString("server.host"))
	base := fmt.Sprintf("http://127.0.0.1:%d", v.GetInt("server.port"))
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, v.GetString("postgres.url"))
	require.NoError(t, err)
	defer pool.Close()
	company := os.Getenv("THREADIFY_E2E_COMPANY_ID")
	if company == "" {
		company = v.GetString("registry.company_id")
	}
	if company == "" {
		company = uuid.NewString()
	}
	_, err = pool.Exec(ctx, "INSERT INTO companies(id,name) VALUES($1,'Multi-system E2E') ON CONFLICT(id) DO NOTHING", company)
	require.NoError(t, err)
	type system struct{ ID, Key, Name string }
	names := []string{"orders-system", "warehouse-system"}
	if contractBased {
		names = append(names, "contract-test-provisioner")
	}
	systems := make([]system, len(names))
	for i, name := range names {
		random := make([]byte, 32)
		_, err = rand.Read(random)
		require.NoError(t, err)
		systems[i] = system{uuid.NewString(), "td_" + base64.RawURLEncoding.EncodeToString(random), name}
		s := systems[i]
		role := "standard_service"
		if i == 2 {
			role = "admin"
		}
		hash := sha256.Sum256([]byte(s.Key))
		keyID := uuid.NewString()
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{"INSERT INTO service_accounts(id,company_id,name) VALUES($1,$2,$3)", []any{s.ID, company, name}},
			{"INSERT INTO api_keys(id,service_account_id,company_id,key_hash,key_prefix,name,expires_at) VALUES($1,$2,$3,$4,$5,'Multi-system E2E temporary key',NOW()+interval '30 minutes')", []any{keyID, s.ID, company, hex.EncodeToString(hash[:]), s.Key[:10]}},
			{"INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account',$2,$1)", []any{s.ID, role}},
		} {
			_, err = tx.Exec(ctx, q.sql, q.args...)
			if err != nil {
				tx.Rollback(ctx)
			}
			require.NoError(t, err)
		}
		require.NoError(t, tx.Commit(ctx))
		defer func(id, principal, assignedRole string) {
			_, err := pool.Exec(ctx, "UPDATE api_keys SET is_active=false,revoked_at=NOW() WHERE id=$1", id)
			require.NoError(t, err)
			if assignedRole == "admin" {
				_, err = pool.Exec(ctx, "DELETE FROM user_roles WHERE principal_id=$1 AND principal_type='service_account' AND role_name='admin'", principal)
				require.NoError(t, err)
			}
		}(keyID, s.ID, role)
	}
	var provisioner system
	if contractBased {
		provisioner = systems[2]
		systems = systems[:2]
	}
	client := &http.Client{Timeout: 15 * time.Second}
	request := func(t *testing.T, s system, path, kind string, body []byte) map[string]any {
		t.Helper()
		req, err := http.NewRequest("POST", base+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("X-API-Key", s.Key)
		req.Header.Set("Content-Type", kind)
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, 200, res.StatusCode, string(data))
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		require.Empty(t, result["errors"], string(data))
		return result
	}
	exchange := func(ws *websocket.Conn, payload map[string]any) (map[string]any, error) {
		if err := ws.WriteJSON(payload); err != nil {
			return nil, err
		}
		ws.SetReadDeadline(time.Now().Add(15 * time.Second))
		for {
			var reply map[string]any
			if err := ws.ReadJSON(&reply); err != nil {
				return nil, err
			}
			if reply["action"] == payload["action"] {
				return reply, nil
			}
		}
	}
	send := func(t *testing.T, ws *websocket.Conn, payload map[string]any) map[string]any {
		t.Helper()
		reply, err := exchange(ws, payload)
		require.NoError(t, err)
		return reply
	}
	connect := func(t *testing.T, s system) *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial(strings.Replace(base, "http://", "ws://", 1)+"/threads", nil)
		require.NoError(t, err)
		t.Cleanup(func() { ws.Close() })
		reply := send(t, ws, map[string]any{"action": "connect", "apiKey": s.Key, "serviceName": s.Name})
		require.Equal(t, "success", reply["status"])
		require.Equal(t, s.ID, reply["ownerId"])
		return ws
	}
	poll := func(t *testing.T, sql string, want int, args ...any) {
		t.Helper()
		var count int
		ok := assert.Eventually(t, func() bool {
			err := pool.QueryRow(ctx, sql, args...).Scan(&count)
			require.NoError(t, err)
			return count == want
		}, 45*time.Second, 200*time.Millisecond, "want %d, last %d", want, count)
		if !ok {
			if len(args) > 0 {
				var diagnostic []byte
				_ = pool.QueryRow(ctx, "SELECT COALESCE(json_agg(json_build_object('step',step_name,'status',status,'actor',actor,'service',actor_service)), '[]'::json) FROM thread_step_states WHERE thread_id=$1", args[0]).Scan(&diagnostic)
				t.Logf("Persisted steps: %s", diagnostic)
			}
			t.FailNow()
		}
	}
	sequentialName, parallelName := "", ""
	contractIDs := map[string]string{}
	if contractBased {
		prefix := "multi_system_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		sequentialName, parallelName = prefix+"_sequential", prefix+"_parallel"
		createContract := func(name string, steps, transitions, includes []any, entry, terminal string) {
			declaration := map[string]any{"contract_name": name, "version": 1, "description": "Two-system contract E2E", "parties": []string{"orders", "warehouse"}, "steps": steps, "transitions": transitions, "entry_points": []string{entry}, "terminal_steps": []string{terminal}, "validation": map[string]any{"max_duration": "1h"}, "versioning": map[string]any{"threads_lock_to_version": true}}
			if len(includes) > 0 {
				declaration["includes"] = includes
			}
			body, err := json.Marshal(declaration)
			require.NoError(t, err) // JSON is valid YAML.
			result := request(t, provisioner, "/v1/contracts", "text/plain", body)
			contractIDs[name] = result["contract"].(map[string]any)["id"].(string)
		}
		steps, transitions := []any{}, []any{}
		ordered := []string{"order_received", "inventory_reserved", "dispatch_requested", "dispatched"}
		for i, step := range ordered {
			role := "orders"
			if i%2 == 1 {
				role = "warehouse"
			}
			steps = append(steps, map[string]any{"id": step, "owner": role, "type": "managed"})
			if i > 0 {
				transitions = append(transitions, map[string]any{"from": ordered[i-1], "to": []string{step}})
			}
		}
		moduleName := prefix + "_first_half"
		createContract(moduleName, steps[:2], transitions[:1], nil, ordered[0], ordered[1])
		createContract(sequentialName, steps[2:], transitions[1:], []any{map[string]any{"name": moduleName, "version": 1}}, ordered[0], ordered[3])
		steps = []any{map[string]any{"id": "order_received", "owner": "orders", "type": "managed"}, map[string]any{"id": "dispatched", "owner": "warehouse", "type": "managed"}}
		transitions = []any{}
		branches := []string{}
		for i, role := range []string{"orders", "warehouse"} {
			for j := 0; j < 4; j++ {
				step := fmt.Sprintf("system_%d_operation_%d", i, j)
				branches = append(branches, step)
				steps = append(steps, map[string]any{"id": step, "owner": role, "type": "managed", "depends_on": []string{"order_received"}})
			}
		}
		steps[1].(map[string]any)["depends_on"] = branches
		createContract(parallelName, steps, transitions, nil, "order_received", "dispatched")
	}
	event := func(id, step string) map[string]any {
		return map[string]any{"action": "recordThreadEvent", "threadId": id, "stepName": step, "type": "managed", "status": "success", "idempotencyKey": uuid.NewString(), "startedAt": time.Now().Add(-time.Millisecond).UTC().Format(time.RFC3339Nano), "finishedAt": time.Now().UTC().Format(time.RFC3339Nano), "context": map[string]string{"scenario": "multi-system-join"}}
	}
	verify := func(t *testing.T, id string, n int) {
		t.Helper()
		poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND status='success'", n, id)
		for _, s := range systems {
			poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND actor=$2 AND actor_service=$3", n/2, id, s.ID, s.Name)
		}
		poll(t, "SELECT count(*) FROM thread_access WHERE thread_id=$1 AND user_id IN ($2,$3) AND status='active'", 2, id, systems[0].ID, systems[1].ID)
		query := `query($id:ID!,$tid:String!){thread(id:$id){id status completedAt contractId contractName contractVersion validationResults{hasCriticalViolation criticalCount warningCount minorCount} steps{stepName status actor actorService verified verificationError}} verifyThreadIntegrity(threadId:$tid){verified totalEvents error}}`
		body, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]any{"id": id, "tid": id}})
		for _, s := range systems {
			result := request(t, s, "/graphql", "application/json", body)["data"].(map[string]any)
			chain := result["verifyThreadIntegrity"].(map[string]any)
			require.Equal(t, true, chain["verified"], chain)
			require.EqualValues(t, n, chain["totalEvents"])
			thread := result["thread"].(map[string]any)
			require.Equal(t, "completed", thread["status"])
			require.NotNil(t, thread["completedAt"])
			if contractBased {
				name := sequentialName
				if n == 10 {
					name = parallelName
				}
				require.Equal(t, name, thread["contractName"])
				require.Equal(t, contractIDs[name], thread["contractId"])
				require.EqualValues(t, 1, thread["contractVersion"])
				for _, raw := range thread["validationResults"].([]any) {
					result := raw.(map[string]any)
					require.Equal(t, false, result["hasCriticalViolation"], result)
					require.EqualValues(t, 0, result["criticalCount"], result)
					require.EqualValues(t, 0, result["warningCount"], result)
					require.EqualValues(t, 0, result["minorCount"], result)
				}
			}
			steps := thread["steps"].([]any)
			require.Len(t, steps, n)
			for _, raw := range steps {
				step := raw.(map[string]any)
				require.Equal(t, true, step["verified"], step)
			}
		}
	}
	evidence := map[string]any{"company_id": company, "systems": []map[string]string{{"id": systems[0].ID, "name": systems[0].Name}, {"id": systems[1].ID, "name": systems[1].Name}}, "threads": map[string]string{}, "contracts": contractIDs, "contract_based": contractBased}
	for _, mode := range []string{"direct_join", "invitation_join"} {
		t.Run(mode, func(t *testing.T) {
			a, b := connect(t, systems[0]), connect(t, systems[1])
			start := send(t, a, map[string]any{"action": "startThread", "contractName": sequentialName, "role": "orders", "label": "Multi-system E2E: " + mode})
			require.Equal(t, "success", start["status"], start)
			id := start["threadId"].(string)
			bad := send(t, b, map[string]any{"action": "joinThread", "threadToken": "invalid-e2e-token"})
			require.Equal(t, "error", bad["status"], bad)
			join := map[string]any{"action": "joinThread", "threadId": id, "role": "warehouse"}
			if mode == "invitation_join" {
				invite := send(t, a, map[string]any{"action": "inviteParty", "role": "warehouse", "accessLevel": "participant", "expiresIn": "10m"})
				require.Equal(t, "success", invite["status"])
				join = map[string]any{"action": "joinThread", "threadToken": invite["threadToken"]}
			}
			joined := send(t, b, join)
			require.Equal(t, "success", joined["status"], joined)
			require.Equal(t, id, joined["threadId"])
			require.Equal(t, "participant", joined["accessLevel"])
			if contractBased {
				wrongRole := send(t, b, map[string]any{"action": "joinThread", "threadId": id, "role": "undefined_party"})
				require.Equal(t, "error", wrongRole["status"], wrongRole)
				wrongOwner := send(t, b, event(id, "order_received"))
				require.Equal(t, "error", wrongOwner["status"], wrongOwner)
				require.Contains(t, wrongOwner["message"], "requires owner")
			}
			// A repeated join must not create duplicate participant rows.
			require.Equal(t, "success", send(t, b, join)["status"])
			for i, step := range []string{"order_received", "inventory_reserved", "dispatch_requested", "dispatched"} {
				ws := a
				if i%2 == 1 {
					ws = b
				}
				payload := event(id, step)
				reply := send(t, ws, payload)
				require.Equal(t, "success", reply["status"], reply)
				poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND step_name=$2 AND status='success'", 1, id, step)
				if i == 1 {
					require.NoError(t, b.Close())
					b = connect(t, systems[1])
					rejoined := send(t, b, map[string]any{"action": "joinThread", "threadId": id, "role": "warehouse"})
					require.Equal(t, "success", rejoined["status"], rejoined)
				}
			}
			if !contractBased {
				closed := send(t, a, map[string]any{"action": "threadEnd", "threadId": id, "status": "completed", "reason": "Both systems finished the shared workflow"})
				require.Equal(t, "success", closed["status"], closed)
			}
			poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='completed'", 1, id)
			verify(t, id, 4)
			evidence["threads"].(map[string]string)[mode] = id
			t.Logf("%s thread %s: 4 steps, 2 systems, completed, hash verified", mode, id)
		})
	}
	t.Run("concurrent_contributions", func(t *testing.T) {
		a, b := connect(t, systems[0]), connect(t, systems[1])
		start := send(t, a, map[string]any{"action": "startThread", "contractName": parallelName, "role": "orders", "label": "Multi-system E2E: concurrent contributions"})
		require.Equal(t, "success", start["status"], start)
		id := start["threadId"].(string)
		require.Equal(t, "success", send(t, b, map[string]any{"action": "joinThread", "threadId": id, "role": "warehouse"})["status"])
		operations := 5
		if contractBased {
			operations = 4
			started := send(t, a, event(id, "order_received"))
			require.Equal(t, "success", started["status"], started)
			poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND step_name='order_received' AND status='success'", 1, id)
		}
		ready := make(chan struct{})
		errors := make(chan error, 2)
		for i, ws := range []*websocket.Conn{a, b} {
			go func(i int, ws *websocket.Conn) {
				<-ready
				for j := 0; j < operations; j++ {
					reply, err := exchange(ws, event(id, fmt.Sprintf("system_%d_operation_%d", i, j)))
					if err != nil {
						errors <- err
						return
					}
					if reply["status"] != "success" {
						errors <- fmt.Errorf("system %d operation %d: %v", i, j, reply)
						return
					}
				}
				errors <- nil
			}(i, ws)
		}
		close(ready)
		for i := 0; i < 2; i++ {
			require.NoError(t, <-errors)
		}
		if contractBased {
			poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND status='success'", 9, id)
			completed := send(t, b, event(id, "dispatched"))
			require.Equal(t, "success", completed["status"], completed)
		} else {
			poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND status='success'", 10, id)
			closed := send(t, a, map[string]any{"action": "threadEnd", "threadId": id, "status": "completed", "reason": "Both systems finished their five operations"})
			require.Equal(t, "success", closed["status"], closed)
		}
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='completed'", 1, id)
		verify(t, id, 10)
		evidence["threads"].(map[string]string)["concurrent_contributions"] = id
		t.Logf("concurrent thread %s: 10 steps, 2 systems, completed, hash verified", id)
	})
	if !t.Failed() {
		data, err := json.MarshalIndent(evidence, "", "  ")
		require.NoError(t, err)
		evidencePath := filepath.Join(dir, "multi-system-verification.json")
		if contractBased {
			evidencePath = filepath.Join(dir, "contract-multi-system-verification.json")
		}
		if explicit := os.Getenv("THREADIFY_E2E_EVIDENCE_FILE"); explicit != "" {
			evidencePath = explicit
		}
		require.NoError(t, os.WriteFile(evidencePath, append(data, '\n'), 0600))
	}
}
