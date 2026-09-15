package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// TestBrowserAuthentication exercises compiled Engine and Web API services with real cookie sessions.
func TestBrowserAuthentication(t *testing.T) {
	apiURL := os.Getenv("THREADIFY_E2E_WEB_API_URL")
	if apiURL == "" {
		t.Skip("set THREADIFY_E2E_WEB_API_URL and THREADIFY_LIVE_DIR for browser session E2E")
	}
	apiParsed, err := url.Parse(apiURL)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", apiParsed.Hostname())
	dir := standaloneDirectory(t)
	v := viper.New()
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, v.ReadInConfig())
	require.Equal(t, "127.0.0.1", v.GetString("server.host"))
	base := fmt.Sprintf("http://127.0.0.1:%d", v.GetInt("server.port"))
	origin := v.GetString("registry.browser_origin")
	require.NotEmpty(t, origin)
	pool, err := pgxpool.New(context.Background(), v.GetString("postgres.url"))
	require.NoError(t, err)
	defer pool.Close()
	company := v.GetString("registry.company_id")
	require.NotEmpty(t, company)
	user, keyID, key := uuid.NewString(), uuid.NewString(), "td_"+uuid.NewString()+uuid.NewString()
	hash := sha256.Sum256([]byte(key))
	_, err = pool.Exec(context.Background(), `INSERT INTO users(id,company_id,email,full_name,email_verified,onboarding_completed,first_instrumentation_done) VALUES($1,$2,$3,'Browser E2E',true,true,true)`, user, company, user+"@e2e.invalid")
	require.NoError(t, err)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM threadify_browser_sessions WHERE principal_id=$1;`, user)
		_, _ = pool.Exec(context.Background(), `DELETE FROM api_keys WHERE id=$1`, keyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_roles WHERE principal_id=$1`, user)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, user)
	}()
	_, err = pool.Exec(context.Background(), `INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'user','admin',$1)`, user)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `INSERT INTO api_keys(id,user_id,company_id,key_hash,key_prefix,name,expires_at) VALUES($1,$2,$3,$4,'td_e2e','Browser E2E',NOW()+interval '15 minutes')`, keyID, user, company, hex.EncodeToString(hash[:]))
	require.NoError(t, err)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	request := func(method, target string, body any, csrf bool) (int, []byte) {
		t.Helper()
		data, _ := json.Marshal(body)
		req, err := http.NewRequest(method, target, bytes.NewReader(data))
		require.NoError(t, err)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if csrf {
			for _, c := range jar.Cookies(req.URL) {
				if strings.Contains(c.Name, "csrf") {
					req.Header.Set("X-Threadify-CSRF", c.Value)
				}
			}
		}
		response, err := client.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		out, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, out
	}
	code, body := request("POST", base+"/auth/api-key/exchange", map[string]string{"api_key": key}, false)
	require.Equal(t, 200, code, string(body))
	require.NotContains(t, string(body), "tfs_")
	code, body = request("GET", base+"/auth/session", nil, false)
	require.Equal(t, 200, code, string(body))
	require.Contains(t, string(body), user)
	code, body = request("GET", apiURL+"/api/user/profile", nil, false)
	require.Equal(t, 200, code, string(body))
	require.Contains(t, string(body), user)
	query := map[string]string{"query": "{ __typename }"}
	code, _ = request("POST", base+"/graphql", query, false)
	require.Equal(t, 403, code)
	code, body = request("POST", base+"/graphql", query, true)
	require.Equal(t, 200, code, string(body))
	require.Contains(t, string(body), "Query")
	code, body = request("GET", base+"/v1/contracts", nil, false)
	require.Equal(t, 200, code, string(body))
	code, body = request("GET", apiURL+"/api/entity-profile-types", nil, false)
	require.Equal(t, 200, code, string(body))
	t.Run("EnginePublicURL", func(t *testing.T) {
		code, body := request("GET", base+"/v1/engine/settings", nil, false)
		require.Equal(t, 200, code, string(body))
		var original struct {
			PublicURL string `json:"public_url"`
			Source    string `json:"source"`
		}
		require.NoError(t, json.Unmarshal(body, &original))
		defer func() {
			if original.Source == "ui" {
				code, body = request("PUT", base+"/v1/engine/settings", map[string]string{"public_url": original.PublicURL}, true)
			} else {
				code, body = request("DELETE", base+"/v1/engine/settings", nil, true)
			}
			require.Equal(t, 200, code, string(body))
		}()
		value := map[string]string{"public_url": "https://engine.e2e.invalid/threadify/"}
		code, body = request("PUT", base+"/v1/engine/settings", value, false)
		require.Equal(t, 403, code, string(body))
		code, body = request("PUT", base+"/v1/engine/settings", value, true)
		require.Equal(t, 200, code, string(body))
		code, body = request("GET", base+"/v1/engine/settings", nil, false)
		require.Equal(t, 200, code, string(body))
		require.Contains(t, string(body), `"source":"ui"`)
		require.Contains(t, string(body), "wss://engine.e2e.invalid/threadify/threads")
		require.Contains(t, string(body), "https://engine.e2e.invalid/threadify/v1/traces")
		code, body = request("PUT", base+"/v1/engine/settings", map[string]string{"public_url": "https://engine.e2e.invalid?token=invalid"}, true)
		require.Equal(t, 400, code, string(body))
	})
	t.Run("EngineUserLifecycle", func(t *testing.T) {
		ctx := context.Background()
		email := uuid.NewString() + "@e2e.invalid"
		invite := map[string]string{"email": email, "full_name": "Invited E2E", "role": "member"}
		code, body := request("POST", base+"/v1/users", invite, false)
		require.Equal(t, 403, code, string(body))
		code, body = request("POST", base+"/v1/users", invite, true)
		require.Equal(t, 201, code, string(body))
		var result struct {
			User struct {
				ID, Status string
				Roles      []string
			} `json:"user"`
			LoginURL string `json:"login_url"`
		}
		require.NoError(t, json.Unmarshal(body, &result))
		require.Equal(t, "invited", result.User.Status)
		require.Equal(t, origin+"/login", result.LoginURL)
		invited := result.User.ID
		require.NotEmpty(t, invited)
		defer func() {
			_, _ = pool.Exec(ctx, `DELETE FROM threadify_user_audit WHERE user_id=$1`, invited)
			_, _ = pool.Exec(ctx, `DELETE FROM user_roles WHERE principal_id=$1`, invited)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, invited)
		}()
		code, body = request("POST", base+"/v1/users", invite, true)
		require.Equal(t, 200, code, string(body))
		require.Contains(t, string(body), invited)
		code, body = request("GET", base+"/v1/users", nil, false)
		require.Equal(t, 200, code, string(body))
		require.Contains(t, string(body), email)
		code, body = request("PATCH", base+"/v1/users/"+invited, map[string]string{"status": "active"}, true)
		require.Equal(t, 409, code, string(body))
		code, body = request("PATCH", base+"/v1/users/"+invited, map[string]string{"role": "viewer"}, true)
		require.Equal(t, 200, code, string(body))
		code, body = request("PATCH", base+"/v1/users/"+invited, map[string]string{"status": "archived"}, true)
		require.Equal(t, 200, code, string(body))
		code, body = request("POST", base+"/v1/users", invite, true)
		require.Equal(t, 409, code, string(body))
		var state string
		require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM users WHERE id=$1`, invited).Scan(&state))
		require.Equal(t, "archived", state)
		code, body = request("GET", apiURL+"/api/team", nil, false)
		require.Equal(t, 410, code, string(body))

		// Seed an already active member separately: Registry first activation is covered by shared/auth.
		member, memberKeyID, memberKey := uuid.NewString(), uuid.NewString(), "td_"+uuid.NewString()+uuid.NewString()
		memberHash := sha256.Sum256([]byte(memberKey))
		_, err = pool.Exec(ctx, `INSERT INTO users(id,company_id,email,full_name,status,email_verified,onboarding_completed,first_instrumentation_done) VALUES($1,$2,$3,'Active E2E','active',true,true,true)`, member, company, member+"@e2e.invalid")
		require.NoError(t, err)
		defer func() {
			_, _ = pool.Exec(ctx, `DELETE FROM threadify_user_audit WHERE user_id=$1`, member)
			_, _ = pool.Exec(ctx, `DELETE FROM api_keys WHERE id=$1`, memberKeyID)
			_, _ = pool.Exec(ctx, `DELETE FROM user_roles WHERE principal_id=$1`, member)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, member)
		}()
		_, err = pool.Exec(ctx, `INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'user','member',$2)`, member, user)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO api_keys(id,user_id,company_id,key_hash,key_prefix,name,expires_at) VALUES($1,$2,$3,$4,'td_e2e','Lifecycle E2E',NOW()+interval '15 minutes')`, memberKeyID, member, company, hex.EncodeToString(memberHash[:]))
		require.NoError(t, err)
		checkKey := func(want int) {
			t.Helper()
			for _, target := range []string{base + "/v1/users", apiURL + "/api/user/profile"} {
				req, err := http.NewRequest("GET", target, nil)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+memberKey)
				response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
				require.NoError(t, err)
				response.Body.Close()
				require.Equal(t, want, response.StatusCode, target)
			}
		}
		checkKey(200)
		for _, state := range []string{"suspended", "active", "archived"} {
			code, body = request("PATCH", base+"/v1/users/"+member, map[string]string{"status": state}, true)
			require.Equal(t, 200, code, string(body))
			if state == "active" {
				checkKey(200)
			} else {
				checkKey(401)
			}
		}
	})
	code, _ = request("POST", apiURL+"/api/auth/login", map[string]string{"email": "retired@example.test", "password": "unused"}, false)
	require.Equal(t, 410, code)
	code, body = request("POST", base+"/auth/logout", map[string]string{}, true)
	require.Equal(t, 200, code, string(body))
	code, _ = request("GET", apiURL+"/api/user/profile", nil, false)
	require.Equal(t, 401, code)
	code, body = request("POST", base+"/auth/api-key/exchange", map[string]string{"api_key": key}, false)
	require.Equal(t, 200, code, string(body))
	_, err = pool.Exec(context.Background(), `UPDATE api_keys SET revoked_at=NOW(),is_active=false WHERE id=$1`, keyID)
	require.NoError(t, err)
	code, _ = request("GET", apiURL+"/api/user/profile", nil, false)
	require.Equal(t, 401, code)
}
