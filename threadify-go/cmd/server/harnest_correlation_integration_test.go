package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in because this uses the user's configured model provider, exclusively
// with synthetic data from the disposable Engine/database binary fixture.
func runHarnestCorrelationSmoke(t *testing.T, url, key, seedThreadID string, pool *pgxpool.Pool) {
	t.Helper()
	python := os.Getenv("THREADIFY_HARNEST_TEST_PYTHON")
	if python == "" {
		return
	}
	script, err := filepath.Abs("../../../examples/threadify-mcp-agent/tests/integration/run_correlation.py")
	if err != nil {
		t.Fatal(err)
	}
	outputDir := os.Getenv("THREADIFY_HARNEST_EVIDENCE_DIR")
	if outputDir == "" {
		outputDir = t.TempDir()
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"workflow", "trace"} {
		ref := "harnest-test-" + mode + "-" + uuid.NewString()
		evidencePath := filepath.Join(outputDir, mode+"-agent.json")
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Second)
		output, err := exec.CommandContext(ctx, python, script, url, key, seedThreadID, ref, mode, evidencePath).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("Harnest %s: %v\n%s", mode, err, output)
		}
		var result struct {
			Mode        string   `json:"mode"`
			WorkflowRef string   `json:"workflow_ref"`
			Threads     int      `json:"threads"`
			Traces      int      `json:"traces"`
			Steps       int      `json:"steps"`
			CustomSteps int      `json:"custom_steps"`
			ToolSteps   int      `json:"tool_steps"`
			ThreadIDs   []string `json:"thread_ids"`
			StepNames   []string `json:"step_names"`
		}
		result.Mode, result.WorkflowRef = mode, ref
		deadline := time.Now().Add(15 * time.Second)
		for {
			err = pool.QueryRow(context.Background(), `SELECT count(DISTINCT thread_id),count(DISTINCT latest_context->>'otel.trace_id'),count(*),count(*) FILTER(WHERE step_name IN ('agent.inspect.1','agent.inspect.2')),count(*) FILTER(WHERE step_name ILIKE '%get_thread%') FROM thread_step_states WHERE latest_context->>'workflow.run_id'=$1`, ref).Scan(&result.Threads, &result.Traces, &result.Steps, &result.CustomSteps, &result.ToolSteps)
			if err != nil {
				t.Fatal(err)
			}
			if result.CustomSteps == 2 && result.ToolSteps >= 2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("Harnest spans did not persist: %+v", result)
			}
			time.Sleep(100 * time.Millisecond)
		}
		if result.Traces < 2 {
			t.Fatalf("expected two actual agent traces: %+v", result)
		}
		if mode == "workflow" && result.Threads != 1 {
			t.Fatalf("workflow correlation split agent traces: %+v", result)
		}
		if mode == "trace" && result.Threads != result.Traces {
			t.Fatalf("opt-out should retain one thread per trace: %+v", result)
		}
		if err = pool.QueryRow(context.Background(), `SELECT array_agg(DISTINCT thread_id::text),array_agg(DISTINCT step_name) FROM thread_step_states WHERE latest_context->>'workflow.run_id'=$1`, ref).Scan(&result.ThreadIDs, &result.StepNames); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(outputDir, mode+"-engine.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		t.Logf("Harnest %s: %d traces -> %d threads, %d steps, %d get_thread spans; evidence %s", mode, result.Traces, result.Threads, result.Steps, result.ToolSteps, evidencePath)
		fmt.Printf("Harnest %s correlation verified\n", mode)
	}
}
