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
	"github.com/stretchr/testify/require"
	collectpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Opt-in black-box checks against an already running standalone binary. Only
// identity and profile-type configuration are seeded; all contracts,
// threads, events, and entity profiles must be created through engine requests.
func TestStandaloneWorkflows(t *testing.T) {
	dir := standaloneDirectory(t)
	v := viper.New()
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, v.ReadInConfig())
	require.Equal(t, "127.0.0.1", v.GetString("server.host"), "live checks require an explicitly local instance")
	base := fmt.Sprintf("http://127.0.0.1:%d", v.GetInt("server.port"))
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, v.GetString("postgres.url"))
	require.NoError(t, err)
	defer pool.Close()
	client := &http.Client{Timeout: 15 * time.Second}
	company, account, keyID := v.GetString("registry.company_id"), uuid.NewString(), uuid.NewString()
	if company == "" {
		company = uuid.NewString()
	}
	random := make([]byte, 32)
	_, err = rand.Read(random)
	require.NoError(t, err)
	key := "td_" + base64.RawURLEncoding.EncodeToString(random)
	hash := sha256.Sum256([]byte(key))
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO companies(id,name) VALUES($1,'Standalone E2E') ON CONFLICT(id) DO NOTHING", []any{company}},
		{"INSERT INTO service_accounts(id,company_id,name) VALUES($1,$2,'E2E fixture')", []any{account, company}},
		{"INSERT INTO api_keys(id,service_account_id,company_id,key_hash,key_prefix,name,expires_at) VALUES($1,$2,$3,$4,$5,'E2E temporary key',NOW()+interval '15 minutes')", []any{keyID, account, company, hex.EncodeToString(hash[:]), key[:10]}},
		{"INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account','admin',$1),($1,'service_account','standard_service',$1)", []any{account}},
	} {
		_, err = tx.Exec(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(ctx))
	defer func() {
		_, err := pool.Exec(ctx, "UPDATE api_keys SET is_active=false,revoked_at=NOW() WHERE id=$1", keyID)
		require.NoError(t, err)
	}()

	request := func(t *testing.T, method, path, contentType string, body []byte, auth bool) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", contentType)
		if auth {
			req.Header.Set("X-API-Key", key)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, data
	}
	gql := func(t *testing.T, query string, variables map[string]any) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"query": query, "variables": variables})
		code, data := request(t, "POST", "/graphql", "application/json", body, true)
		require.Equal(t, 200, code, string(data))
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		require.Empty(t, result["errors"], string(data))
		return result["data"].(map[string]any)
	}
	poll := func(t *testing.T, sql string, want int, args ...any) {
		t.Helper()
		var count int
		require.Eventually(t, func() bool {
			err := pool.QueryRow(ctx, sql, args...).Scan(&count)
			require.NoError(t, err)
			return count == want
		}, 45*time.Second, 250*time.Millisecond, "SQL count expected %d, last %d", want, count)
	}
	connect := func(t *testing.T) *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial(strings.Replace(base, "http://", "ws://", 1)+"/threads", nil)
		require.NoError(t, err)
		t.Cleanup(func() { ws.Close() })
		require.NoError(t, ws.WriteJSON(map[string]any{"action": "connect", "apiKey": key, "serviceName": "e2e-client"}))
		ws.SetReadDeadline(time.Now().Add(10 * time.Second))
		var reply map[string]any
		require.NoError(t, ws.ReadJSON(&reply))
		require.Equal(t, "success", reply["status"], reply)
		return ws
	}
	send := func(t *testing.T, ws *websocket.Conn, payload map[string]any) map[string]any {
		t.Helper()
		require.NoError(t, ws.WriteJSON(payload))
		ws.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			var reply map[string]any
			require.NoError(t, ws.ReadJSON(&reply))
			if reply["action"] == payload["action"] {
				return reply
			}
		}
	}
	evidence := map[string]any{"company_id": company, "base_url": base, "checks": map[string]bool{}}
	checks := evidence["checks"].(map[string]bool)
	t.Run("sdk_parity", func(t *testing.T) { testSDKParity(t, base, key, company, pool) })
	t.Run("management_cli", func(t *testing.T) { testManagementCLI(t, base, key, company, pool) })
	t.Run("health", func(t *testing.T) {
		code, data := request(t, "GET", "/health", "", nil, false)
		require.Equal(t, 200, code)
		var health map[string]string
		require.NoError(t, json.Unmarshal(data, &health))
		for _, k := range []string{"postgres", "valkey", "nats", "persistence"} {
			require.Equal(t, "ok", health[k], k)
		}
		checks[t.Name()] = true
	})

	contractName := "e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	contractYAML := fmt.Sprintf(`contract_name: %s
version: 1
description: Standalone binary workflow verification
parties: [worker]
steps:
  - id: received
    owner: worker
    type: managed
  - id: completed
    owner: worker
    type: managed
transitions:
  - from: received
    to: [completed]
entry_points: [received]
terminal_steps: [completed]
validation:
  max_duration: 1h
versioning:
  threads_lock_to_version: true
`, contractName)
	var contractID string
	contractCreated := t.Run("contract_creation", func(t *testing.T) {
		code, data := request(t, "POST", "/v1/contracts", "text/plain", []byte(contractYAML), false)
		require.Equal(t, 401, code, string(data))
		code, data = request(t, "POST", "/v1/contracts", "text/plain", []byte(contractYAML), true)
		require.Equal(t, 200, code, string(data))
		var result struct {
			Contract struct {
				ID string `json:"id"`
			} `json:"contract"`
		}
		require.NoError(t, json.Unmarshal(data, &result))
		contractID = result.Contract.ID
		require.NotEmpty(t, contractID)
		poll(t, "SELECT count(*) FROM contracts c JOIN contract_versions v ON v.contract_id=c.id WHERE c.id=$1 AND v.version=1", 1, contractID)
		code, data = request(t, "GET", "/v1/contracts/"+contractID, "", nil, true)
		require.Equal(t, 200, code, string(data))
		require.Contains(t, string(data), contractName)
		evidence["contract_id"] = contractID
		code, data = request(t, "POST", "/v1/contracts", "text/plain", []byte(contractYAML), true)
		require.Equal(t, 400, code, "duplicate contract: %s", data)
		code, data = request(t, "POST", "/v1/contracts", "text/plain", []byte("contract_name: [invalid"), true)
		require.Equal(t, 400, code, "invalid contract: %s", data)
		checks[t.Name()] = true
	})

	t.Run("contract_thread_and_entity_profile", func(t *testing.T) {
		if !contractCreated {
			t.Skip("contract creation failed")
		}
		profileType, ref := uuid.NewString(), "customer_"+uuid.NewString()
		// Repeated runs share a licensed company, so each run needs its own type and ref key.
		slug := "e2e_customers_" + strings.ReplaceAll(profileType, "-", "")
		refName := "customer_" + strings.ReplaceAll(profileType, "-", "")
		_, err := pool.Exec(ctx, "INSERT INTO entity_profile_type(id,company_id,name,type,slug) VALUES($1,$2,$3,ARRAY[$4::text],$5)", profileType, company, "E2E Customers "+profileType[:8], refName, slug)
		require.NoError(t, err)
		// Omitted descriptions are SQL NULL; all UI-facing reads must support them.
		types := gql(t, `{entityProfileTypes{id description}}`, nil)["entityProfileTypes"].([]any)
		var createdType map[string]any
		for _, item := range types {
			candidate := item.(map[string]any)
			if candidate["id"] == profileType {
				createdType = candidate
				break
			}
		}
		require.NotNil(t, createdType)
		require.Equal(t, "", createdType["description"])
		ws := connect(t)
		reply := send(t, ws, map[string]any{"action": "startThread", "contractName": contractName, "role": "worker", "refs": map[string]string{refName: ref}, "label": "Contract and profile verification"})
		require.Equal(t, "success", reply["status"], reply)
		id := reply["threadId"].(string)
		for _, step := range []string{"received", "completed"} {
			reply = send(t, ws, map[string]any{"action": "recordThreadEvent", "threadId": id, "stepName": step, "status": "success", "type": "managed", "actor": "worker", "startedAt": time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano), "finishedAt": time.Now().UTC().Format(time.RFC3339Nano), "context": map[string]string{"source": "e2e"}})
			require.Equal(t, "success", reply["status"], reply)
			// Contract validation completes asynchronously after the receipt.
			poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND step_name=$2 AND status='success'", 1, id, step)
		}
		poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND status='success'", 2, id)
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND contract_name=$2 AND contract_version=1", 1, id, contractName)
		poll(t, "SELECT count(*) FROM entity_profile WHERE company_id=$1 AND entity_profile_type_id=$2 AND ref_key=$3", 1, company, profileType, ref)
		result := gql(t, `query($id: ID!){thread(id:$id){id contractName contractVersion steps{stepName status}}}`, map[string]any{"id": id})
		thread := result["thread"].(map[string]any)
		require.Equal(t, contractName, thread["contractName"])
		require.Len(t, thread["steps"], 2)
		result = gql(t, `query($ref:String!,$type:String!){entityProfile(refKey:$ref,type:$type){id refKey companyId profileType{id description}}}`, map[string]any{"ref": ref, "type": slug})
		profile, ok := result["entityProfile"].(map[string]any)
		require.True(t, ok, "%v", result)
		require.Equal(t, ref, profile["refKey"])
		require.Equal(t, company, profile["companyId"])
		require.Equal(t, "", profile["profileType"].(map[string]any)["description"])
		listed := gql(t, `query($type:String!){entityProfilesByType(type:$type){totalCount profileType{id description} items{id}}}`, map[string]any{"type": slug})["entityProfilesByType"].(map[string]any)
		require.EqualValues(t, 1, listed["totalCount"])
		require.Equal(t, "", listed["profileType"].(map[string]any)["description"])

		// A second thread referencing the same entity updates the existing profile.
		reply = send(t, ws, map[string]any{"action": "startThread", "role": "owner", "refs": map[string]string{refName: ref}, "label": "Repeated entity reference"})
		require.Equal(t, "success", reply["status"], reply)
		poll(t, "SELECT count(*) FROM thread_refs WHERE thread_id=$1 AND ref_key=$3 AND ref_value=$2", 1, reply["threadId"], ref, refName)
		poll(t, "SELECT count(*) FROM entity_profile WHERE company_id=$1 AND entity_profile_type_id=$2 AND ref_key=$3", 1, company, profileType, ref)
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='completed'", 1, id)
		evidence["contract_thread_id"] = id
		evidence["entity_profile_id"] = profile["id"]
		checks[t.Name()] = true
	})

	t.Run("otel_conversion_and_replay", func(t *testing.T) {
		traceID := make([]byte, 16)
		_, err := rand.Read(traceID)
		require.NoError(t, err)
		kv := func(k, v string) *commonpb.KeyValue {
			return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
		}
		// Deliberately historical: ingestion time must never replace producer time.
		now := time.Date(2025, time.January, 2, 10, 30, 0, 123456000, time.UTC)
		span1 := &tracepb.Span{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 8}, Name: "received", StartTimeUnixNano: uint64(now.UnixNano()), EndTimeUnixNano: uint64(now.Add(time.Second).UnixNano()), Attributes: []*commonpb.KeyValue{kv("threadify.context.region", "eu-west-2")}, Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK}}
		span2 := &tracepb.Span{TraceId: traceID, SpanId: []byte{2, 3, 4, 5, 6, 7, 8, 9}, ParentSpanId: span1.SpanId, Name: "completed", StartTimeUnixNano: uint64(now.Add(2 * time.Second).UnixNano()), EndTimeUnixNano: uint64(now.Add(3 * time.Second).UnixNano()), Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "provider rejected request"}, Events: []*tracepb.Span_Event{{Name: "provider_response", TimeUnixNano: uint64(now.Add(2500 * time.Millisecond).UnixNano()), Attributes: []*commonpb.KeyValue{kv("result", "rejected")}}}}
		payload := &collectpb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{kv("service.name", "worker-service"), kv("threadify.label", "OTEL conversion E2E"), kv("threadify.ref.order_id", "order-e2e")}}, ScopeSpans: []*tracepb.ScopeSpans{{Scope: &commonpb.InstrumentationScope{Name: "threadify-e2e"}, Spans: []*tracepb.Span{span2, span1}}}}}}
		data, err := proto.Marshal(payload)
		require.NoError(t, err)
		code, _ := request(t, "POST", "/v1/traces", "application/x-protobuf", data, false)
		require.Equal(t, 401, code)
		for i := 0; i < 2; i++ {
			code, response := request(t, "POST", "/v1/traces", "application/x-protobuf", data, true)
			require.Equal(t, 200, code)
			var result collectpb.ExportTraceServiceResponse
			require.NoError(t, proto.Unmarshal(response, &result))
			require.Zero(t, result.GetPartialSuccess().GetRejectedSpans(), result.GetPartialSuccess().GetErrorMessage())
		}
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(company+":"+hex.EncodeToString(traceID))).String()
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND company_id=$2", 1, id, company)
		poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1", 2, id)
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND created_at=$2", 1, id, now)
		poll(t, "SELECT count(*) FROM thread_activities WHERE thread_id=$1 AND activity_type='step_recorded' AND payload->>'step_name'='received' AND recorded_at=$2 AND started_at=$3 AND finished_at=$2", 1, id, now.Add(time.Second), now)
		// Storage arrival remains distinct from the event's historical timestamp.
		poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND step_name='received' AND created_at > $2", 1, id, now.Add(24*time.Hour))
		timing := gql(t, `query($id:ID!){thread(id:$id){startedAt completedAt steps{stepName startedAt finishedAt}}}`, map[string]any{"id": id})
		timingThread := timing["thread"].(map[string]any)
		reportedStart, err := time.Parse(time.RFC3339Nano, timingThread["startedAt"].(string))
		require.NoError(t, err)
		require.True(t, now.Equal(reportedStart), "GraphQL must retain the producer's fractional-second start")
		reportedEnd, err := time.Parse(time.RFC3339Nano, timingThread["completedAt"].(string))
		require.NoError(t, err)
		require.True(t, now.Add(time.Second).Equal(reportedEnd))

		poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND step_name='completed' AND status='failed'", 1, id)
		poll(t, "SELECT count(*) FROM step_substeps WHERE thread_id=$1 AND name='provider_response' AND payload->>'result'='rejected'", 1, id)
		poll(t, "SELECT count(*) FROM thread_refs WHERE thread_id=$1 AND ref_key='otel_trace_id' AND ref_value=$2", 1, id, hex.EncodeToString(traceID))
		var region, spanID string
		var start, finish time.Time
		require.NoError(t, pool.QueryRow(ctx, "SELECT latest_context->>'region',latest_context->>'otel.span_id',started_at,finished_at FROM thread_step_states WHERE thread_id=$1 AND step_name='received'", id).Scan(&region, &spanID, &start, &finish))
		require.Equal(t, "eu-west-2", region)
		require.Equal(t, hex.EncodeToString(span1.SpanId), spanID)
		require.True(t, now.Equal(start))
		require.True(t, now.Add(time.Second).Equal(finish))
		result := gql(t, `query($id:ID!){thread(id:$id){id contractName steps{stepName status idempotencyKey}}}`, map[string]any{"id": id})
		steps := result["thread"].(map[string]any)["steps"].([]any)
		require.Len(t, steps, 2)
		expected := map[string]string{"received": hex.EncodeToString(span1.SpanId), "completed": hex.EncodeToString(span2.SpanId)}
		for _, raw := range steps {
			step := raw.(map[string]any)
			require.Equal(t, "otel:"+hex.EncodeToString(traceID)+":"+expected[step["stepName"].(string)], step["idempotencyKey"])
		}
		// Invalid spans are reported through OTLP partial success, not silently accepted.
		invalid := proto.Clone(payload).(*collectpb.ExportTraceServiceRequest)
		invalid.ResourceSpans[0].ScopeSpans[0].Spans = []*tracepb.Span{{Name: "invalid", TraceId: []byte{1}}}
		invalidData, err := proto.Marshal(invalid)
		require.NoError(t, err)
		code, response := request(t, "POST", "/v1/traces", "application/x-protobuf", invalidData, true)
		require.Equal(t, 200, code)
		var rejected collectpb.ExportTraceServiceResponse
		require.NoError(t, proto.Unmarshal(response, &rejected))
		require.EqualValues(t, 1, rejected.GetPartialSuccess().GetRejectedSpans())
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='completed' AND completed_at=$2", 1, id, now.Add(time.Second))
		// The root has already ended. A delayed child still persists and extends
		// the HMAC chain without reopening the execution or changing its end.
		late := proto.Clone(span2).(*tracepb.Span)
		late.SpanId = []byte{3, 4, 5, 6, 7, 8, 9, 10}
		late.Name = "late-child"
		late.Events = nil
		delayed := proto.Clone(payload).(*collectpb.ExportTraceServiceRequest)
		delayed.ResourceSpans[0].ScopeSpans[0].Spans = []*tracepb.Span{late}
		lateData, err := proto.Marshal(delayed)
		require.NoError(t, err)
		for range 2 {
			code, response := request(t, "POST", "/v1/traces", "application/x-protobuf", lateData, true)
			require.Equal(t, 200, code)
			var accepted collectpb.ExportTraceServiceResponse
			require.NoError(t, proto.Unmarshal(response, &accepted))
			require.Zero(t, accepted.GetPartialSuccess().GetRejectedSpans(), accepted.GetPartialSuccess().GetErrorMessage())
		}
		poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1", 3, id)
		poll(t, "SELECT count(*) FROM thread_activities WHERE thread_id=$1 AND activity_type='step_recorded'", 3, id)
		poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='completed' AND completed_at=$2", 1, id, now.Add(time.Second))
		integrity := gql(t, `query($id:ID!,$tid:String!){verifyThreadIntegrity(threadId:$tid){verified totalEvents error} thread(id:$id){status completedAt steps{verified verificationError}}}`, map[string]any{"id": id, "tid": id})
		chain := integrity["verifyThreadIntegrity"].(map[string]any)
		require.Equal(t, true, chain["verified"])
		require.EqualValues(t, 3, chain["totalEvents"])
		require.Nil(t, chain["error"])
		for _, raw := range integrity["thread"].(map[string]any)["steps"].([]any) {
			require.Equal(t, true, raw.(map[string]any)["verified"])
		}
		t.Run("child_before_marked_failed_invocation", func(t *testing.T) {
			nextTrace := make([]byte, 16)
			_, err := rand.Read(nextTrace)
			require.NoError(t, err)
			invocation := proto.Clone(span1).(*tracepb.Span)
			invocation.TraceId = nextTrace
			invocation.ParentSpanId = []byte{8, 8, 8, 8, 8, 8, 8, 8} // HTTP parent is not exported.
			invocation.EndTimeUnixNano = uint64(now.Add(4 * time.Second).UnixNano())
			invocation.Attributes = append(invocation.Attributes, kv("threadify.run.complete", "true"))
			invocation.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "agent execution failed"}
			earlyChild := proto.Clone(span2).(*tracepb.Span)
			earlyChild.TraceId = nextTrace
			earlyChild.Events = nil
			nextID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(company+":"+hex.EncodeToString(nextTrace))).String()
			send := func(span *tracepb.Span) {
				batch := proto.Clone(payload).(*collectpb.ExportTraceServiceRequest)
				batch.ResourceSpans[0].ScopeSpans[0].Spans = []*tracepb.Span{span}
				data, err := proto.Marshal(batch)
				require.NoError(t, err)
				code, response := request(t, "POST", "/v1/traces", "application/x-protobuf", data, true)
				require.Equal(t, 200, code)
				var result collectpb.ExportTraceServiceResponse
				require.NoError(t, proto.Unmarshal(response, &result))
				require.Zero(t, result.GetPartialSuccess().GetRejectedSpans(), result.GetPartialSuccess().GetErrorMessage())
			}
			send(earlyChild)
			poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='active' AND completed_at IS NULL", 1, nextID)
			send(invocation)
			send(invocation)
			poll(t, "SELECT count(*) FROM threads WHERE id=$1 AND status='completed' AND completed_at=$2", 1, nextID, now.Add(4*time.Second))
			poll(t, "SELECT count(*) FROM thread_step_states WHERE thread_id=$1 AND status='failed'", 2, nextID)
			poll(t, "SELECT count(*) FROM thread_activities WHERE thread_id=$1 AND activity_type='step_recorded'", 2, nextID)
			verification := gql(t, `query($id:String!){verifyThreadIntegrity(threadId:$id){verified totalEvents}}`, map[string]any{"id": nextID})
			require.Equal(t, true, verification["verifyThreadIntegrity"].(map[string]any)["verified"])
		})
		evidence["otel_thread_id"] = id
		evidence["otel_trace_id"] = hex.EncodeToString(traceID)
		checks[t.Name()] = true
	})
	data, err := json.MarshalIndent(evidence, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workflow-verification.json"), append(data, '\n'), 0600))
	t.Log(string(data))
}
