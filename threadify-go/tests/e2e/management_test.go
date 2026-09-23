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
	viewPath := "/v1/entity-profile-views/" + created["data"].(map[string]any)["id"].(string)
	emptyView := request("GET", viewPath, nil, "", 200)
	require.Nil(t, emptyView["data"].(map[string]any)["definition"])
	require.Equal(t, true, emptyView["can_manage"])
	viewDefinition := map[string]any{"schemaVersion": 1, "title": "Customer health", "description": "Live delivery health", "range": "30d", "columns": 2, "blocks": []any{map[string]any{"id": "health", "source": "deliveryHealth", "title": "Health"}}}
	viewPayload := map[string]any{"expected_revision": 0, "definition": viewDefinition}
	savedView := request("PUT", viewPath, viewPayload, "", 200)
	require.EqualValues(t, 1, savedView["data"].(map[string]any)["revision"])
	persistedView := request("GET", viewPath, nil, "", 200)
	require.Equal(t, savedView["data"], persistedView["data"])
	require.Equal(t, "PROFILE_VIEW_CONFLICT", request("PUT", viewPath, viewPayload, "", 409)["code"])
	forbiddenPlan := map[string]any{"expected_revision": 1, "definition": viewDefinition, "requests": []any{map[string]any{"operation": "arbitraryQuery"}}}
	request("PUT", viewPath, forbiddenPlan, "", 400)
	historyDefinition := map[string]any{"schemaVersion": 1, "title": "History", "description": "", "range": "30d", "columns": 1, "blocks": []any{map[string]any{"id": "history", "source": "history", "title": "History"}}}
	request("PUT", viewPath, map[string]any{"expected_revision": 1, "definition": historyDefinition}, "", 400)
	// A cookie-authenticated write must also carry the session CSRF value.
	csrfBody, err := json.Marshal(map[string]any{"expected_revision": 1, "definition": viewDefinition})
	require.NoError(t, err)
	csrfRequest, err := http.NewRequest("PUT", base+viewPath, bytes.NewReader(csrfBody))
	require.NoError(t, err)
	csrfRequest.Header.Set("Content-Type", "application/json")
	csrfRequest.Header.Set("Origin", base)
	csrfResponse, err := client.Do(csrfRequest)
	require.NoError(t, err)
	require.Equal(t, 403, csrfResponse.StatusCode)
	csrfResponse.Body.Close()

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
	readerView := request("GET", viewPath, nil, key, 200)
	require.Equal(t, false, readerView["can_manage"])
	require.Equal(t, persistedView["data"], readerView["data"])
	request("PUT", viewPath, map[string]any{"expected_revision": 1, "definition": viewDefinition}, key, 403)
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
	request("GET", viewPath, nil, "", 404)
	request("PUT", viewPath, map[string]any{"expected_revision": 1, "definition": viewDefinition}, "", 404)
}
