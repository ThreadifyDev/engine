package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Uses the same disposable binary/database fixture as the other workflow checks.
func testThreadKeySDK(t *testing.T, base, key, contract string) map[string]string {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", filepath.Join(root, "threadify-sdk/tests/live-thread-key.mjs"))
	cmd.Env = append(os.Environ(), "THREADIFY_ENGINE_URL="+base, "THREADIFY_API_KEY="+key, "THREADIFY_CONTRACT="+contract)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	require.NoError(t, err, strings.ReplaceAll(stderr.String(), key, "[redacted]"))
	var result map[string]string
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), fmt.Sprintf("SDK output: %s", stdout.String()))
	return result
}
