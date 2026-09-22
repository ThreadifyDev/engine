package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"threadify-go/shared/testutil/registryfixture"
)

// The release binary owns both browser assets and the authenticated API origin.
func TestEmbeddedDashboard(t *testing.T) {
	dir := standaloneDirectory(t)
	v := viper.New()
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, v.ReadInConfig())
	base := fmt.Sprintf("http://127.0.0.1:%d", v.GetInt("server.port"))
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	request := func(method, path string, body []byte, csrf bool) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Origin", base)
		req.Header.Set("Content-Type", "application/json")
		if csrf {
			for _, cookie := range jar.Cookies(req.URL) {
				if strings.Contains(cookie.Name, "csrf") {
					req.Header.Set("X-Threadify-CSRF", cookie.Value)
				}
			}
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp, string(data)
	}
	var shell string
	for _, path := range []string{"/", "/login", "/cli-login", "/u/threads/order-123?tab=history", "/u/contracts/order-flow/versions/2", "/u/profiles/Customers/customer-123", "/auth/reset-password"} {
		resp, body := request("GET", path, nil, false)
		require.Equal(t, 200, resp.StatusCode, body)
		require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
		require.Contains(t, body, `id="root"`)
		require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
		if shell != "" {
			require.Equal(t, shell, body)
		}
		shell = body
	}
	assets := regexp.MustCompile(`(?:src|href)="(/assets/[^"?#]+)`).FindAllStringSubmatch(shell, -1)
	require.NotEmpty(t, assets)
	for _, asset := range assets {
		resp, body := request("GET", asset[1], nil, false)
		require.Equal(t, 200, resp.StatusCode, asset[1])
		require.NotEmpty(t, body)
		require.NotContains(t, resp.Header.Get("Content-Type"), "text/html")
		require.Contains(t, resp.Header.Get("Cache-Control"), "immutable")
		require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	}
	resp, _ := request("GET", "/assets/not-present.js", nil, false)
	require.Equal(t, 404, resp.StatusCode)
	resp, body := request("GET", "/auth/session", nil, false)
	require.Equal(t, 200, resp.StatusCode, body)
	require.Contains(t, body, `"authenticated":false`)
	resp, body = request("POST", "/graphql", []byte(`{"query":"{ __typename }"}`), false)
	require.Equal(t, 401, resp.StatusCode, body)
	require.NotContains(t, resp.Header.Get("Content-Type"), "text/html")
	// Only disposable fixtures may bootstrap with their integration license.
	if v.GetString("registry.license_key") != registryfixture.License {
		return
	}
	keyBody, err := json.Marshal(map[string]string{"api_key": registryfixture.License})
	require.NoError(t, err)
	resp, body = request("POST", "/auth/api-key/exchange", keyBody, false)
	require.Equal(t, 200, resp.StatusCode, body)
	resp, body = request("GET", "/auth/session", nil, false)
	require.Equal(t, 200, resp.StatusCode, body)
	require.Contains(t, body, `"authenticated":true`)
	resp, body = request("GET", "/v1/billing/plan", nil, false)
	require.Equal(t, 200, resp.StatusCode, body)
	require.Contains(t, body, `"billing_source":"registry"`)
	require.Contains(t, body, `"input_requests_per_second"`)
	resp, body = request("POST", "/webhook", []byte(`{}`), true)
	require.Equal(t, 404, resp.StatusCode, body)
	resp, body = request("POST", "/graphql", []byte(`{"query":"{ __typename }"}`), true)
	require.Equal(t, 200, resp.StatusCode, body)
	require.Contains(t, body, `"__typename":"Query"`)
	resp, body = request("GET", "/v1/engine/settings", nil, false)
	require.Equal(t, 200, resp.StatusCode, body)
	require.Contains(t, body, `"public_url"`)
	t.Run("engine_management", func(t *testing.T) { testEngineManagement(t, base, client) })
	resp, body = request("POST", "/auth/logout", []byte(`{}`), false)
	require.Equal(t, 403, resp.StatusCode, body)
	resp, body = request("POST", "/auth/logout", []byte(`{}`), true)
	require.Equal(t, 200, resp.StatusCode, body)
}

// Contracts created by SDK/service identities remain visible to their company's UI users.
func testCompanyContractBrowserAccess(t *testing.T, base, license, contractID string) {
	t.Helper()
	if license != registryfixture.License {
		return
	}
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	body, err := json.Marshal(map[string]string{"api_key": license})
	require.NoError(t, err)
	req, err := http.NewRequest("POST", base+"/auth/api-key/exchange", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	for _, path := range []string{"/v1/contracts", "/v1/contracts/" + contractID, "/v1/contracts/" + contractID + "/versions", "/v1/contracts/" + contractID + "/versions/1"} {
		resp, err := client.Get(base + path)
		require.NoError(t, err)
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode, "%s: %s", path, data)
		require.Contains(t, string(data), contractID)
	}
}
