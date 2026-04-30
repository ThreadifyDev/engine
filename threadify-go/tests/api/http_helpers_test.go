package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"threadify-go/shared/domain"
	"threadify-go/shared/repository"
)

func uniqueEmail() string {
	return "test-" + uuid.NewString() + "@example.com"
}

type httpResponse struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

func doJSON(t *testing.T, method, path string, payload any) httpResponse {
	t.Helper()

	var body *bytes.Buffer
	if payload == nil {
		body = bytes.NewBuffer(nil)
	} else {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewBuffer(b)
	}

	req, err := http.NewRequest(method, apiApp.BaseURL+path, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := apiApp.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResponse{StatusCode: resp.StatusCode, Body: respBody, Header: resp.Header}
}

func doJSONWithAuth(t *testing.T, method, path string, payload any, token string) httpResponse {
	t.Helper()

	var body *bytes.Buffer
	if payload == nil {
		body = bytes.NewBuffer(nil)
	} else {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewBuffer(b)
	}

	req, err := http.NewRequest(method, apiApp.BaseURL+path, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := apiApp.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResponse{StatusCode: resp.StatusCode, Body: respBody, Header: resp.Header}
}

func doRaw(t *testing.T, method, path string, body []byte, contentType string) httpResponse {
	t.Helper()

	req, err := http.NewRequest(method, apiApp.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := apiApp.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResponse{StatusCode: resp.StatusCode, Body: respBody, Header: resp.Header}
}

func doRawWithAuth(t *testing.T, method, path string, body []byte, contentType, token string) httpResponse {
	t.Helper()

	req, err := http.NewRequest(method, apiApp.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := apiApp.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResponse{StatusCode: resp.StatusCode, Body: respBody, Header: resp.Header}
}

func decodeJSONBody(t *testing.T, resp httpResponse) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	return out
}

func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func signupUser(t *testing.T, email, password string) {
	t.Helper()

	resp := doJSON(t, http.MethodPost, "/api/auth/signup", map[string]any{
		"email":        email,
		"password":     password,
		"full_name":    "Test User",
		"company_name": "Test Company",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"signupUser: unexpected status: %s", string(resp.Body))

	waitForAuthUser(t, email)
}

func loginUser(t *testing.T, email, password string) map[string]any {
	t.Helper()

	resp := doJSON(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"loginUser: unexpected status: %s", string(resp.Body))

	otp := supabase.GetOTP(email)
	require.NotEmpty(t, otp,
		"loginUser: expected FakeSupabase to have an OTP for the user")

	verifyResp := doJSON(t, http.MethodPost, "/api/auth/verify-otp", map[string]any{
		"email": email,
		"token": otp,
	})
	require.Equal(t, http.StatusOK, verifyResp.StatusCode,
		"loginUser: unexpected status from verify: %s", string(verifyResp.Body))

	return decodeJSONBody(t, verifyResp)
}

type AuthUser struct {
	ID          string
	CompanyID   string
	Email       string
	Password    string
	AccessToken string
}

func setupAuthenticatedUser(t *testing.T) AuthUser {
	t.Helper()

	email := uniqueEmail()
	password := "Password123!@#"

	signupUser(t, email, password)
	authData := loginUser(t, email, password)

	userMap := authData["user"].(map[string]any)

	return AuthUser{
		ID:          userMap["id"].(string),
		CompanyID:   userMap["company_id"].(string),
		Email:       email,
		Password:    password,
		AccessToken: authData["token"].(string),
	}
}

func waitForAuthUser(t *testing.T, email string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if supabase.HasUser(email) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for user %q to be registered in the auth provider", email)
}

type creditAccountFixture struct {
	AutoTopupMillicents  int64
	MinBalanceMillicents int64
}

func ensureCreditAccount(t *testing.T, companyID string, fixture creditAccountFixture) *domain.CreditAccount {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, env.Postgres.ConnectionString)
	require.NoError(t, err)
	defer pool.Close()

	repo := repository.NewPlanRepo(pool)

	existing, err := repo.GetCreditAccount(ctx, companyID)
	require.NoError(t, err)
	if existing != nil {
		require.NoError(t, repo.UpdateTopupSettings(ctx, companyID, fixture.AutoTopupMillicents, fixture.MinBalanceMillicents))
		updated, err := repo.GetCreditAccount(ctx, companyID)
		require.NoError(t, err)
		require.NotNil(t, updated)
		return updated
	}

	account := &domain.CreditAccount{
		ID:                               "ca_" + uuid.NewString(),
		CompanyID:                        companyID,
		BillingCycleStart:                time.Now().UTC(),
		CreditBalanceMillicents:          0,
		CreditMinBalanceMillicents:       fixture.MinBalanceMillicents,
		CreditMaxMonthlyChargeMillicents: 0,
		CreditAutoTopupMillicents:        fixture.AutoTopupMillicents,
		CreditMonthlyChargedMillicents:   0,
		RateLimitTPS:                     0,
		PayloadLimitBytes:                0,
	}

	require.NoError(t, repo.CreateCreditAccount(ctx, account))

	return account
}

func assertEmptyOrNilList(t *testing.T, body map[string]any, key string) {
	t.Helper()
	raw := body[key]
	if raw == nil {
		return
	}
	items, ok := raw.([]any)
	require.True(t, ok, "expected %q to be an array, got %T", key, raw)
	assert.Empty(t, items)
}
