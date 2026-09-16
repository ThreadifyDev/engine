package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/redis/go-redis/v9"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	collectpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Runs within the disposable binary fixture, also through the two-replica proxy.
func prepareOTelCorrelationSmoke(t *testing.T, url, key, valkeyAddr string, pool *pgxpool.Pool) func() {
	t.Helper()
	ref := "correlation-" + uuid.NewString()
	client := &http.Client{Timeout: 15 * time.Second}
	attr := func(k, v string) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
	}
	post := func(index int, suffix, contract string) (int64, error) {
		trace, _ := hex.DecodeString(fmt.Sprintf("%032x", index))
		span, _ := hex.DecodeString(fmt.Sprintf("%016x", index))
		attrs := []*commonpb.KeyValue{attr("workflow.run_id", ref)}
		if contract != "" {
			attrs = append(attrs, attr("threadify.contract", contract))
		}
		payload, err := proto.Marshal(&collectpb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: trace, SpanId: span, Name: "http.tool", StartTimeUnixNano: uint64(time.Now().UnixNano()), EndTimeUnixNano: uint64(time.Now().UnixNano() + 1000), Attributes: attrs}}}}}}})
		if err != nil {
			return 0, err
		}
		req, err := http.NewRequest("POST", url+"/v1/traces"+suffix, bytes.NewReader(payload))
		if err != nil {
			return 0, err
		}
		req.Header.Set("X-API-Key", key)
		req.Header.Set("Content-Type", "application/x-protobuf")
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode != 200 {
			return 0, fmt.Errorf("HTTP %d: %q", resp.StatusCode, body)
		}
		var result collectpb.ExportTraceServiceResponse
		if err := proto.Unmarshal(body, &result); err != nil {
			return 0, err
		}
		return result.GetPartialSuccess().GetRejectedSpans(), nil
	}
	send := func(i int, suffix, contract string, rejected int64) {
		t.Helper()
		got, err := post(i, suffix, contract)
		if err != nil || got != rejected {
			t.Fatalf("OTLP %d rejected=%d want=%d: %v", i, got, rejected, err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rejected, err := post(i, "", "")
			if err == nil && rejected != 0 {
				err = fmt.Errorf("rejected %d spans", rejected)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	await := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatal("correlation persistence did not converge")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	var threadID string
	await(func() bool {
		return pool.QueryRow(context.Background(), "SELECT thread_id FROM thread_refs WHERE ref_key='threadify.external_ref' AND ref_value=$1", ref).Scan(&threadID) == nil
	})
	verify := func(steps int) {
		t.Helper()
		await(func() bool {
			var n int
			_ = pool.QueryRow(context.Background(), "SELECT count(*) FROM thread_step_states WHERE thread_id=$1", threadID).Scan(&n)
			return n == steps
		})
		var count int
		var status string
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM thread_refs WHERE ref_key='threadify.external_ref' AND ref_value=$1", ref).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("reference created %d threads", count)
		}
		if err := pool.QueryRow(context.Background(), "SELECT status FROM threads WHERE id=$1", threadID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "active" {
			t.Fatalf("shared root unexpectedly completed thread: %s", status)
		}
	}
	verify(12)
	// Exact retries are idempotent and a conflicting contract cannot append a step.
	send(1, "", "", 0)
	send(20, "", "different-contract", 1)
	// URL opt-out preserves one trace -> one thread, despite the matching workflow attribute.
	send(101, "?use_workflow_run_id=false", "", 0)
	send(102, "?use_workflow_run_id=false", "", 0)
	await(func() bool {
		var n int
		_ = pool.QueryRow(context.Background(), "SELECT count(DISTINCT thread_id) FROM thread_refs WHERE ref_key='otel_trace_id' AND ref_value=ANY($1)", []string{fmt.Sprintf("%032x", 101), fmt.Sprintf("%032x", 102)}).Scan(&n)
		return n == 2
	})
	script, err := sdkSmokeScript("live-otel-correlation.mjs")
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("node", script, url, key, ref, threadID).CombinedOutput()
	if err != nil {
		t.Fatalf("live OTEL SDK: %v\n%s", err, output)
	}
	t.Logf("live OTEL SDK: %s", output)
	verify(15)
	runHarnestCorrelationSmoke(t, url, key, threadID, pool)
	// Simulate expiry of only this reference mapping in the disposable Valkey.
	cache := redis.NewClient(&redis.Options{Addr: valkeyAddr})
	digest := sha256.Sum256([]byte(ref))
	cacheKey := fmt.Sprintf("otel:trace:8bf9099d-2ff9-4d88-a2eb-acb114679909:ref:%x", digest)
	if err := cache.Del(context.Background(), cacheKey).Err(); err != nil {
		t.Fatal(err)
	}
	cache.Close()
	return func() {
		send(30, "", "", 0)
		send(1, "", "", 0)
		verify(16)
		t.Log("OTLP/SDK correlation persisted across restart")
	}
}
