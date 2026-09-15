package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func testSDKParity(t *testing.T, base, key, company string, pool *pgxpool.Pool) {
	python := os.Getenv("THREADIFY_E2E_PYTHON")
	goClient := os.Getenv("THREADIFY_E2E_GO_SDK_BINARY")
	if python == "" || goClient == "" {
		t.Skip("set THREADIFY_E2E_PYTHON and THREADIFY_E2E_GO_SDK_BINARY to exercise both SDKs")
	}
	goKey := "td_" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	goAccount, goKeyID := uuid.NewString(), uuid.NewString()
	hash := sha256.Sum256([]byte(goKey))
	_, err := pool.Exec(context.Background(), "INSERT INTO service_accounts(id,company_id,name) VALUES($1,$2,'Go SDK parity')", goAccount, company)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), "INSERT INTO api_keys(id,service_account_id,company_id,key_hash,key_prefix,name,expires_at) VALUES($1,$2,$3,$4,$5,'SDK parity',NOW()+interval '30 minutes')", goKeyID, goAccount, company, hex.EncodeToString(hash[:]), goKey[:10])
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), "INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account','standard_service',$1)", goAccount)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "UPDATE api_keys SET is_active=false,revoked_at=NOW() WHERE id=$1", goKeyID)
	})
	activeKey := key
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	name := "sdk_parity_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	source := `Feature: ` + name + `
Rule: Approval
 When step "approval" is submitted
 Then owner must be "python"
 And this step is an entry point
Rule: Charge
 When step "charge" is submitted
 Then owner must be "go"
 And step "approval" must succeed before each invocation
 And content "amount" must be a number greater than 0
Rule: Finish
 When step "finish" is submitted
 Then owner must be "python"
 And step "charge" must have succeeded
 And this step is terminal
`
	req, err := http.NewRequest("POST", base+"/v1/contracts", strings.NewReader(source))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "text/plain")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode, string(body))
	environment := append(os.Environ(), "THREADIFY_ENGINE_URL="+base, "THREADIFY_API_KEY="+key, "THREADIFY_CONTRACT="+name+":1", "THREADIFY_PARITY_ID="+name, "PYTHONPATH="+filepath.Join(root, "threadify-sdk-python"))
	run := func(program string, args ...string) map[string]string {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, program, args...)
		for _, value := range environment {
			if !strings.HasPrefix(value, "THREADIFY_API_KEY=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "THREADIFY_API_KEY="+activeKey)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		require.NoError(t, err, strings.ReplaceAll(strings.ReplaceAll(stderr.String(), key, "[redacted]"), goKey, "[redacted]"))
		var result map[string]string
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), stdout.String())
		return result
	}
	script := filepath.Join(root, "threadify-sdk-python", "tests", "live", "parity.py")
	created := run(python, script, "create")
	require.NotEmpty(t, created["thread_id"])
	environment = append(environment, "THREADIFY_THREAD_ID="+created["thread_id"])
	activeKey = goKey
	charged := run(goClient)
	activeKey = key
	require.Equal(t, created["thread_id"], charged["thread_id"])
	require.NotEmpty(t, charged["step_id"])
	environment = append(environment, "THREADIFY_STEP_ID="+charged["step_id"])
	finished := run(python, script, "finish")
	require.Equal(t, "completed", finished["status"])
	require.Equal(t, created["thread_id"], finished["thread_id"])
	t.Logf("Python → Go → Python contract thread %s completed; Go event %s validated by both SDKs", finished["thread_id"], charged["step_id"])
}
