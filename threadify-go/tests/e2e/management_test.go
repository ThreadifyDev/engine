package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Exercise management through cookie-protected Engine routes, with no Web API.
func testEngineManagement(t *testing.T, base string, client *http.Client) {
	t.Helper()
	request := func(method, path string, payload any, key string, want int) map[string]any {
		t.Helper()
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", base)
		active := client
		if key != "" {
			req.Header.Set("X-API-Key", key)
			active = &http.Client{Timeout: 10 * time.Second}
		} else {
			for _, cookie := range client.Jar.Cookies(req.URL) {
				if strings.Contains(cookie.Name, "csrf") {
					req.Header.Set("X-Threadify-CSRF", cookie.Value)
				}
			}
		}
		resp, err := active.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, want, resp.StatusCode, "%s %s: %s", method, path, data)
		var out map[string]any
		require.NoError(t, json.Unmarshal(data, &out), string(data))
		return out
	}
	for _, path := range []string{"/v1/user/profile", "/v1/roles", "/v1/roles/api_level", "/v1/api-keys", "/v1/service-accounts", "/v1/entity-profile-types", "/v1/metrics-templates", "/v1/billing/plan", "/v1/code-samples"} {
		request("GET", path, nil, "", 200)
	}
	request("POST", "/v1/user/profile", map[string]any{"full_name": "Engine Tester", "job_role": "Engineer"}, "", 200)
	request("POST", "/v1/user/profile", map[string]any{"full_name": "Engine Tester", "job_role": "Engineer", "industry": "Technology", "company_size": "small", "use_case": "Support"}, "", 200)
	request("POST", "/v1/user/profile", map[string]any{"full_name": "Engine Tester", "job_role": "Engineer", "industry": "Technology", "company_size": "medium", "use_case": "Operations"}, "", 200)
	name := "management_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	profile := map[string]any{"name": name, "type": []string{name + "_id"}, "description": "Engine UI test", "metrics": []any{}}
	path := "/v1/entity-profile-types/" + name
	dry := request("PUT", path+"?dry_run=true", profile, "", 200)
	require.Equal(t, true, dry["dry_run"])
	created := request("PUT", path, profile, "", 200)
	require.Equal(t, "created", created["status"])
	repeat := request("PUT", path, profile, "", 200)
	require.Equal(t, "unchanged", repeat["status"])
	profile["description"] = "updated through Engine"
	request("PUT", path, profile, "", 200)
	account := request("POST", "/v1/service-accounts", map[string]any{"name": name, "description": "Test account", "role": "reader"}, "", 201)
	id := account["service_account"].(map[string]any)["id"].(string)
	credential := request("POST", "/v1/api-keys", map[string]any{"name": name, "service_account_id": id}, "", 201)
	key := credential["key"].(string)
	keyID := credential["api_key"].(map[string]any)["id"].(string)
	request("GET", "/v1/entity-profile-types", nil, key, 200)
	request("PUT", path, profile, key, 403)
	request("GET", "/v1/service-accounts", nil, key, 403)
	request("POST", "/v1/api-keys", map[string]any{"name": "forbidden"}, key, 403)
	request("PUT", "/v1/service-accounts/"+id, map[string]any{"name": name, "description": "Disabled", "is_active": false}, "", 200)
	request("GET", "/v1/entity-profile-types", nil, key, 401)
	request("PUT", "/v1/service-accounts/"+id, map[string]any{"name": name, "description": "Re-enabled", "is_active": true}, "", 200)
	request("GET", "/v1/entity-profile-types", nil, key, 200)
	request("DELETE", "/v1/api-keys/"+keyID, nil, "", 200)
	request("GET", "/v1/entity-profile-types", nil, key, 401)
	request("DELETE", "/v1/service-accounts/"+id, nil, "", 200)
	request("DELETE", path, nil, "", 200)
}
