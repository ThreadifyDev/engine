package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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

// This runs the shipped binary as a separate process for every CLI operation.
// Only the surrounding suite's identity fixture uses SQL to seed state.
func testManagementCLI(t *testing.T, base, key, company string, pool *pgxpool.Pool) {
	binary := os.Getenv("THREADIFY_E2E_CLI_BINARY")
	if binary == "" {
		t.Skip("set THREADIFY_E2E_CLI_BINARY to test the standalone CLI")
	}
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	dir := t.TempDir()
	cliConfig := filepath.Join(dir, "cli.yaml")
	environment := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "THREADIFY_CLI_CONFIG=" + cliConfig, "THREADIFY_API_KEY=" + key}
	run := func(input string, args ...string) (map[string]any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"--api-url", base}, args...)...)
		cmd.Env = environment
		cmd.Stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			return nil, fmt.Errorf("CLI failed: %s", stderr.String())
		}
		var result map[string]any
		err = json.Unmarshal(stdout.Bytes(), &result)
		return result, err
	}
	unique := strings.ReplaceAll(uuid.NewString(), "-", "")
	name := "cli_" + unique
	refKey := "cli_customer_" + unique
	typName := "CLI Customers " + unique
	contract := fmt.Sprintf(`Feature: %s
Version: 1
Description: CLI integration test

Rule: Receive work
  When step "received" is submitted
  Then owner must be "worker"
  And step type is "managed"
  And this step is an entry point
  And this step is terminal
`, name)
	preview, err := run(contract, "contracts", "preview", "--file", "-")
	require.NoError(t, err)
	require.Equal(t, true, preview["valid"])
	result, err := run(contract, "contracts", "create", "--file", "-")
	require.NoError(t, err)
	contractID := result["contract"].(map[string]any)["id"].(string)
	_, err = run("", "contracts", "get", "--id", contractID)
	require.NoError(t, err)
	_, err = run(contract, "contracts", "create", "--file", "-")
	require.Error(t, err, "duplicate create must fail")
	_, err = run(strings.ReplaceAll(strings.Replace(contract, "Version: 1", "Version: 2", 1), "received", "received_v2"), "contracts", "update", "--id", contractID, "--file", "-")
	require.NoError(t, err)
	versions, err := run("", "contracts", "versions", "--id", contractID)
	require.NoError(t, err)
	require.Len(t, versions["versions"], 2)
	_, err = run("", "contracts", "get", "--id", contractID, "--version", "1")
	require.NoError(t, err)
	_, err = run("", "contracts", "list", "--search", name, "--limit", "1", "--offset", "0")
	require.NoError(t, err)
	_, err = run("", "contracts", "list", "--key", "invalid-test-key")
	require.Error(t, err, "invalid keys must fail")
	profileYAML := fmt.Sprintf("name: %s\ntype: [%s]\n", typName, refKey)
	result, err = run(profileYAML, "profile-types", "create", "--file", "-")
	require.NoError(t, err)
	typeID := result["data"].(map[string]any)["id"].(string)
	_, err = run("", "profile-types", "list")
	require.NoError(t, err)
	result, err = run("", "profiles", "create", "--type-id", typeID, "--ref-value", "explicit", "--name", "CLI Customer")
	require.NoError(t, err)
	profileID := result["data"].(map[string]any)["id"]
	result, err = run("", "profiles", "create", "--type-id", typeID, "--ref-value", "explicit", "--name", "Renamed Customer")
	require.NoError(t, err)
	require.Equal(t, profileID, result["data"].(map[string]any)["id"])
	refs, _ := json.Marshal(map[string]string{refKey: "automatic"})
	result, err = run("", "threads", "start", "--label", "CLI workflow", "--contract", name, "--role", "worker", "--refs", string(refs))
	require.NoError(t, err)
	threadID := result["threadId"].(string)
	require.Eventually(t, func() bool {
		var count int
		err := pool.QueryRow(context.Background(), "SELECT count(*) FROM entity_profile WHERE company_id=$1 AND entity_profile_type_id=$2 AND ref_key='automatic'", company, typeID).Scan(&count)
		return err == nil && count == 1
	}, 15*time.Second, 100*time.Millisecond)
	result, err = run("", "profiles", "list", "--type", typName)
	require.NoError(t, err)
	require.EqualValues(t, 2, result["entityProfilesByType"].(map[string]any)["totalCount"])
	result, err = run("", "profiles", "get", "--type", typName, "--ref-value", "automatic")
	require.NoError(t, err)
	automaticID := result["entityProfile"].(map[string]any)["id"].(string)
	history, err := run("", "profiles", "history", "--id", automaticID)
	require.NoError(t, err)
	require.EqualValues(t, 1, history["entityProfileHistory"].(map[string]any)["totalCount"])
	_, err = run("", "profiles", "list", "--type", typName, "--search", "automatic", "--limit", "1")
	require.NoError(t, err)
	_, err = run("", "profiles", "get", "--id", profileID.(string))
	require.NoError(t, err)
	_, err = run("", "threads", "get", "--id", threadID)
	require.NoError(t, err)
	variables, _ := json.Marshal(map[string]string{"id": threadID})
	queried, err := run(`query($id:ID!){thread(id:$id){id}}`, "graphql", "--file", "-", "--variables", string(variables))
	require.NoError(t, err)
	require.Equal(t, threadID, queried["thread"].(map[string]any)["id"])
	_, err = run("", "threads", "list", "--status", "active", "--limit", "1")
	require.NoError(t, err)
	result, err = run("", "threads", "find", "--ref-key", refKey, "--ref-value", "automatic")
	require.NoError(t, err)
	require.EqualValues(t, 1, result["threadsByRef"].(map[string]any)["totalCount"])
	_, err = run("", "threads", "add-refs", "--id", threadID, "--refs", `{"source":"cli"}`)
	require.NoError(t, err)
	_, err = run("", "threads", "close", "--id", threadID, "--status", "cancelled")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var status string
		err := pool.QueryRow(context.Background(), "SELECT status FROM threads WHERE id=$1", threadID).Scan(&status)
		return err == nil && status == "cancelled"
	}, 15*time.Second, 100*time.Millisecond)
	_, err = run("", "threads", "verify", "--id", threadID)
	require.NoError(t, err)

	_, err = run("", "contracts", "delete", "--id", contractID)
	require.Error(t, err, "destructive commands require confirmation")
	_, err = run("", "contracts", "delete", "--id", contractID, "--yes")
	require.NoError(t, err)

	// Approve the real login transaction using a cookie-authenticated browser client.
	loginCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(loginCtx, binary, "login", "--api-url", base, "--no-browser")
	cmd.Env = environment
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var loginErrors bytes.Buffer
	cmd.Stderr = &loginErrors
	require.NoError(t, cmd.Start())
	links := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "http") {
				links <- scanner.Text()
			}
		}
		close(links)
	}()
	var link string
	select {
	case link = <-links:
	case <-loginCtx.Done():
		t.Fatal("login URL not produced")
	}
	verification, err := url.Parse(link)
	require.NoError(t, err)
	require.NotEmpty(t, verification.Fragment)
	fragment, err := url.ParseQuery(verification.Fragment)
	require.NoError(t, err)
	origin := verification.Scheme + "://" + verification.Host
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	browserPost := func(path string, body any, csrf string) int {
		data, _ := json.Marshal(body)
		req, err := http.NewRequest("POST", base+path, bytes.NewReader(data))
		require.NoError(t, err)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set("X-Threadify-CSRF", csrf)
		}
		resp, err := browser.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}
	require.Equal(t, 200, browserPost("/auth/api-key/exchange", map[string]string{"api_key": key}, ""))
	session, err := browser.Get(base + "/auth/session")
	require.NoError(t, err)
	session.Body.Close()
	engineURL, _ := url.Parse(base)
	csrf := ""
	for _, cookie := range jar.Cookies(engineURL) {
		if strings.Contains(cookie.Name, "csrf") {
			csrf = cookie.Value
		}
	}
	require.NotEmpty(t, csrf)
	approval := map[string]string{"transaction_id": fragment.Get("transaction_id"), "browser_token": fragment.Get("browser_token")}
	require.Equal(t, 403, browserPost("/auth/cli/approve", approval, ""), "CSRF must be required")
	require.Equal(t, 200, browserPost("/auth/cli/approve", approval, csrf))
	require.Equal(t, 403, browserPost("/auth/cli/approve", approval, csrf), "approval must be one-use")
	require.NoError(t, cmd.Wait(), loginErrors.String())
	// Clear the ambient key; commands must now use only the newly saved login.
	environment = environment[:len(environment)-1]
	result, err = run("", "whoami")
	require.NoError(t, err)
	require.Equal(t, company, result["user"].(map[string]any)["company_id"])
	require.Equal(t, "service_account", result["user"].(map[string]any)["principal_type"])
	require.Equal(t, "managed_cli_login", result["local_credential_source"])
	_, err = run("", "threads", "start", "--label", "Managed CLI login")
	require.NoError(t, err)
	_, err = run("", "contracts", "list")
	require.NoError(t, err)
	_, err = run("", "threads", "list")
	require.NoError(t, err)
	logoutCmd := exec.CommandContext(loginCtx, binary, "logout")
	logoutCmd.Env = environment
	out, err := logoutCmd.CombinedOutput()
	require.NoError(t, err, string(out))
	_, err = run("", "whoami")
	require.Error(t, err, "logout must remove the saved login")
}
