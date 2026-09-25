package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/middleware"
	"go.uber.org/zap"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/rbac"
	"threadify-go/shared/registry"
)

type mcpOAuthRegistry struct {
	sharedauth.BrowserRegistry
	origin string
}

func (*mcpOAuthRegistry) Snapshot() (registry.Snapshot, error) { return registry.Snapshot{}, nil }
func (*mcpOAuthRegistry) CompanyID() string                    { return "company" }
func (*mcpOAuthRegistry) AccountID() string                    { return "account" }
func (*mcpOAuthRegistry) OwnerEmail() string                   { return "owner@example.test" }
func (*mcpOAuthRegistry) InstallationID() string               { return "installation" }
func (*mcpOAuthRegistry) BrowserKey() []byte                   { return []byte(strings.Repeat("k", 32)) }
func (r *mcpOAuthRegistry) BrowserOrigin() string              { return r.origin }
func (*mcpOAuthRegistry) IsLicenseKey(key string) bool         { return key == "test-license" }

type mcpOAuthAuth struct{ domain.AuthService }

func (mcpOAuthAuth) VerifyToken(ctx context.Context, token string) (*sharedauth.TokenClaims, error) {
	return sharedauth.VerifyOAuthAccess(ctx, token)
}

type mcpBearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t mcpBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(copy)
}

func TestMCPConnectWithOAuthAuthorizationCode(t *testing.T) {
	dsn := os.Getenv("THREADIFY_AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_AUTH_TEST_DATABASE_URL for PostgreSQL OAuth MCP integration test")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	schema := "mcp_oauth_" + strings.ToLower(strconv.FormatInt(int64(os.Getpid()), 16)) + "_" + strings.ToLower(strconv.FormatInt(time.Now().UnixNano(), 16))
	// A per-test schema keeps this test independent of a running Engine.
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	_, err = pool.Exec(ctx, `CREATE TABLE users(id varchar(255) PRIMARY KEY, company_id varchar(255), email varchar(255) UNIQUE NOT NULL, full_name varchar(255), email_verified bool DEFAULT false, onboarding_completed bool DEFAULT false, first_instrumentation_done bool DEFAULT false, last_login_at timestamp);
CREATE TABLE service_accounts(id varchar(255) PRIMARY KEY, company_id varchar(255), name text, is_active bool DEFAULT true);
CREATE TABLE user_roles(principal_id varchar(255), principal_type varchar(50), role_name varchar(100), assigned_by varchar(255), PRIMARY KEY(principal_id,role_name));
CREATE TABLE api_keys(id varchar(255) PRIMARY KEY, company_id varchar(255), user_id varchar(255), service_account_id varchar(255), key_hash varchar(255) UNIQUE, is_active bool DEFAULT true, revoked_at timestamp, expires_at timestamp);`)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httptest.NewUnstartedServer(nil)
	serverURL := "http://" + server.Listener.Addr().String()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	runtime := &mcpOAuthRegistry{origin: serverURL}
	browser, err := sharedauth.NewBrowserService(ctx, pool, runtime)
	require.NoError(t, err)
	sharedauth.SetBrowserService(browser)
	t.Cleanup(func() { sharedauth.SetBrowserService(nil) })
	loader, err := rbac.NewEmbeddedLoader()
	require.NoError(t, err)
	settings := &config.Config{}
	settings.Server.Port = port
	settings.Server.PublicURL = serverURL
	authService := mcpOAuthAuth{}
	router.POST("/graphql", middleware.AuthMiddleware(authService, middleware.AuthJWT), func(c *gin.Context) {
		c.JSON(200, gin.H{"data": gin.H{"threads": gin.H{"threads": []any{}, "totalCount": 0}}})
	})
	sse := router.Group("/sse")
	sse.Use(func(c *gin.Context) {
		c.Set("oauth_resource_metadata", serverURL+"/.well-known/oauth-protected-resource/sse")
		c.Next()
	})
	sse.Use(middleware.AuthMiddleware(authService, middleware.AuthDual))
	mountMCPServer(sse, settings, nil, zap.NewNop())
	wrapped, err := browser.OAuthServer(ctx, loader, serverURL, browser.Wrap(router))
	require.NoError(t, err)
	server.Config.Handler = wrapped
	server.Start()
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	request := func(method, path, body, contentType string, cookies []*http.Cookie, csrf string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, serverURL+path, strings.NewReader(body))
		require.NoError(t, err)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if cookies != nil {
			for _, cookie := range cookies {
				req.AddCookie(cookie)
			}
		}
		if csrf != "" {
			req.Header.Set(sharedauth.BrowserCSRFHeader, csrf)
		}
		if method == http.MethodPost {
			req.Header.Set("Origin", serverURL)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		return resp
	}
	login := request("POST", "/auth/api-key/exchange", `{"api_key":"test-license"}`, "application/json", nil, "")
	require.Equal(t, 200, login.StatusCode)
	cookies := login.Cookies()
	login.Body.Close()
	var csrf string
	for _, cookie := range cookies {
		if strings.Contains(cookie.Name, "csrf") {
			csrf = cookie.Value
		}
	}
	require.NotEmpty(t, csrf)
	registration := `{"name":"Threadify MCP test","client_type":"confidential","redirect_uris":["https://client.example/callback"],"allowed_scopes":["query.execution.read"]}`
	resp := request("POST", "/v1/oauth/clients", registration, "application/json", cookies, csrf)
	require.Equal(t, 201, resp.StatusCode)
	var created struct {
		Client struct {
			ID string `json:"ClientID"`
		} `json:"client"`
		Secret string `json:"client_secret"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()
	require.NotEmpty(t, created.Client.ID)
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	values := url.Values{"client_id": {created.Client.ID}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "scope": {"query.execution.read"}, "state": {"mcp-state"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	resp = request("GET", "/oauth/authorize?"+values.Encode(), "", "", cookies, "")
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	values.Set("decision", "allow")
	resp = request("POST", "/oauth/authorize/consent", values.Encode(), "application/x-www-form-urlencoded", cookies, csrf)
	require.Equal(t, 302, resp.StatusCode)
	redirect, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	code := redirect.Query().Get("code")
	require.NotEmpty(t, code)
	resp.Body.Close()
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"https://client.example/callback"}, "code_verifier": {verifier}}
	tokenReq, err := http.NewRequest("POST", serverURL+"/oauth/token", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.SetBasicAuth(created.Client.ID, created.Secret)
	resp, err = client.Do(tokenReq)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	var tokens struct {
		Access string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tokens))
	resp.Body.Close()
	require.NotEmpty(t, tokens.Access)

	resp = request("GET", "/.well-known/oauth-protected-resource/sse", "", "", nil, "")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, string(body), serverURL+"/sse")
	require.Contains(t, string(body), "query.execution.read")
	resp = request("POST", "/sse", `{ "jsonrpc":"2.0", "id":1, "method":"initialize", "params":{} }`, "application/json", nil, "")
	require.Equal(t, 401, resp.StatusCode)
	require.Contains(t, resp.Header.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource/sse")
	resp.Body.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "oauth-test", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: serverURL + "/sse", HTTPClient: &http.Client{Transport: mcpBearerTransport{base: http.DefaultTransport, token: tokens.Access}}, DisableStandaloneSSE: true}
	session, err := mcpClient.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	var found bool
	for _, tool := range listed.Tools {
		if tool.Name == "search_threads" {
			found = true
		}
	}
	require.True(t, found)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_threads", Arguments: map[string]any{"limit": 1}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "totalCount")

	mutationReq, err := http.NewRequest("POST", serverURL+"/graphql", strings.NewReader(`{"query":"mutation { deleteAllContracts }"}`))
	require.NoError(t, err)
	mutationReq.Header.Set("Authorization", "Bearer "+tokens.Access)
	resp, err = client.Do(mutationReq)
	require.NoError(t, err)
	require.Equal(t, 401, resp.StatusCode)
	resp.Body.Close()
	_, err = pool.Exec(ctx, `DELETE FROM user_roles WHERE role_name='admin'`)
	require.NoError(t, err)
	_, err = session.ListTools(ctx, nil)
	require.Error(t, err)
}
