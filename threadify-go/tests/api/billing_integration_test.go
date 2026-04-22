package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBilling_GetCurrentPlan_WithAccount(t *testing.T) {
	user := setupAuthenticatedUser(t)

	ensureCreditAccount(t, user.CompanyID, creditAccountFixture{
		AutoTopupMillicents:  10 * 100_000,
		MinBalanceMillicents: 5 * 100_000,
	})

	resp := doRawWithAuth(t, http.MethodGet, "/api/billing/plan", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	ca, ok := body["credit_account"].(map[string]any)
	require.True(t, ok, "response must contain a credit_account object")

	// Identity
	assert.Equal(t, user.CompanyID, ca["company_id"],
		"credit account must belong to the authenticated user's company")
	assert.NotEmpty(t, ca["id"],
		"credit account must have an id")

	// fields match what was seeded
	assert.Equal(t, float64(100_000), ca["balance_millicents"],
		"new signups must have their initial signup credits balance")
	assert.Equal(t, float64(10*100_000), ca["auto_topup_millicents"],
		"auto_topup_millicents must match seeded value")
	assert.Equal(t, float64(5*100_000), ca["min_balance_millicents"],
		"min_balance_millicents must match seeded value")

	// Timestamps must be present and parseable
	billingCycleStart, ok := ca["billing_cycle_start"].(string)
	require.True(t, ok, "billing_cycle_start must be a string")
	_, err := time.Parse(time.RFC3339, billingCycleStart)
	assert.NoError(t, err, "billing_cycle_start must be a valid RFC3339 timestamp")

	createdAt, ok := ca["created_at"].(string)
	require.True(t, ok, "created_at must be a string")
	_, err = time.Parse(time.RFC3339, createdAt)
	assert.NoError(t, err, "created_at must be a valid RFC3339 timestamp")
}

func TestBilling_GetCurrentPlan_IsolatedByCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	ensureCreditAccount(t, user1.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 10 * 100_000,
	})
	ensureCreditAccount(t, user2.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 20 * 100_000,
	})

	resp1 := doRawWithAuth(t, http.MethodGet, "/api/billing/plan", nil, "", user1.AccessToken)
	require.Equal(t, http.StatusOK, resp1.StatusCode)
	body1 := decodeJSONBody(t, resp1)
	ca1 := body1["credit_account"].(map[string]any)

	resp2 := doRawWithAuth(t, http.MethodGet, "/api/billing/plan", nil, "", user2.AccessToken)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	body2 := decodeJSONBody(t, resp2)
	ca2 := body2["credit_account"].(map[string]any)

	assert.Equal(t, user1.CompanyID, ca1["company_id"])
	assert.Equal(t, user2.CompanyID, ca2["company_id"])
	assert.NotEqual(t, ca1["id"], ca2["id"],
		"each company must have its own credit account")
	assert.Equal(t, float64(10*100_000), ca1["auto_topup_millicents"])
	assert.Equal(t, float64(20*100_000), ca2["auto_topup_millicents"],
		"user2 must see their own auto_topup, not user1's")
}

func TestBilling_GetCurrentPlan_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/billing/plan", nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["credit_account"],
		"unauthorized response must not leak credit account data")
}

func TestBilling_UpdateSpendingLimit_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)
	account := ensureCreditAccount(t, user.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 10 * 100_000,
	})

	newLimit := account.CreditAutoTopupMillicents + 500_000

	updateResp := doRawWithAuth(t, http.MethodPut, "/api/billing/spending-limit",
		marshalJSON(t, map[string]any{"max_monthly_millicents": newLimit}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusOK, updateResp.StatusCode, string(updateResp.Body))

	updateBody := decodeJSONBody(t, updateResp)
	assert.Equal(t, "success", updateBody["status"],
		"successful update must return status=success")

	// Verify the update actually persisted
	planResp := doRawWithAuth(t, http.MethodGet, "/api/billing/plan", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, planResp.StatusCode)
	planBody := decodeJSONBody(t, planResp)
	ca := planBody["credit_account"].(map[string]any)
	assert.Equal(t, float64(newLimit), ca["max_monthly_charge_millicents"],
		"updated spending limit must be reflected in GET /billing/plan")
}

func TestBilling_UpdateSpendingLimit_ZeroDisablesLimit(t *testing.T) {
	user := setupAuthenticatedUser(t)
	ensureCreditAccount(t, user.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 10 * 100_000,
	})

	// set a non-zero limit
	doRawWithAuth(t, http.MethodPut, "/api/billing/spending-limit",
		marshalJSON(t, map[string]any{"max_monthly_millicents": 20 * 100_000}),
		"application/json", user.AccessToken)

	// set to zero — should disable the limit
	updateResp := doRawWithAuth(t, http.MethodPut, "/api/billing/spending-limit",
		marshalJSON(t, map[string]any{"max_monthly_millicents": 0}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusOK, updateResp.StatusCode, string(updateResp.Body))

	planResp := doRawWithAuth(t, http.MethodGet, "/api/billing/plan", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, planResp.StatusCode)
	planBody := decodeJSONBody(t, planResp)
	ca := planBody["credit_account"].(map[string]any)
	assert.Equal(t, float64(0), ca["max_monthly_charge_millicents"],
		"zero must be stored and returned, disabling the spending limit")
}

func TestBilling_UpdateSpendingLimit_BelowAutoTopup_Rejected(t *testing.T) {
	user := setupAuthenticatedUser(t)
	account := ensureCreditAccount(t, user.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 10 * 100_000,
	})

	invalidLimit := account.CreditAutoTopupMillicents - 1

	resp := doRawWithAuth(t, http.MethodPut, "/api/billing/spending-limit",
		marshalJSON(t, map[string]any{"max_monthly_millicents": invalidLimit}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "INVALID_LIMIT", body["error"])
	assert.Contains(t, body["message"], "spending limit",
		"error message must describe the spending limit constraint")
	assert.Contains(t, body["message"], "auto-topup",
		"error message must mention the auto-topup amount for clarity")

	// Verify the limit was NOT updated
	planResp := doRawWithAuth(t, http.MethodGet, "/api/billing/plan", nil, "", user.AccessToken)
	planBody := decodeJSONBody(t, planResp)
	ca := planBody["credit_account"].(map[string]any)
	assert.NotEqual(t, float64(invalidLimit), ca["max_monthly_charge_millicents"],
		"rejected limit must not be persisted")
}

func TestBilling_UpdateSpendingLimit_NegativeRejectedByBinding(t *testing.T) {
	user := setupAuthenticatedUser(t)
	ensureCreditAccount(t, user.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 10 * 100_000,
	})

	resp := doRawWithAuth(t, http.MethodPut, "/api/billing/spending-limit",
		marshalJSON(t, map[string]any{"max_monthly_millicents": -1}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid request body", body["error"],
		"negative amount must be rejected by binding before reaching service layer")
}

func TestBilling_UpdateSpendingLimit_MissingBody(t *testing.T) {
	user := setupAuthenticatedUser(t)
	ensureCreditAccount(t, user.CompanyID, creditAccountFixture{
		AutoTopupMillicents: 10 * 100_000,
	})

	resp := doRawWithAuth(t, http.MethodPut, "/api/billing/spending-limit",
		nil, "application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid request body", body["error"])
}

func TestBilling_UpdateSpendingLimit_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodPut, "/api/billing/spending-limit",
		marshalJSON(t, map[string]any{"max_monthly_millicents": 1_000_000}),
		"application/json")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
}

func TestBilling_CreateCheckoutSession_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	amountMillicents := int64(2_000_000) // $20.00

	resp := doRawWithAuth(t, http.MethodPost, "/api/billing/checkout",
		marshalJSON(t, map[string]any{"amount_millicents": amountMillicents}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)

	checkoutURL, ok := body["url"].(string)
	require.True(t, ok, "response must contain a url string")
	assert.NotEmpty(t, checkoutURL, "checkout url must not be empty")
	assert.Contains(t, checkoutURL, "example.com/checkout",
		"checkout url must point to the configured checkout endpoint")

	// Must not leak internal data
	assert.Nil(t, body["credit_account"],
		"checkout response must not include credit account data")
	assert.Nil(t, body["error"],
		"successful checkout must not include an error field")
}

func TestBilling_CreateCheckoutSession_MissingBody(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/billing/checkout",
		nil, "application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid request body", body["error"])
	assert.Nil(t, body["url"],
		"error response must not include a checkout url")
}

func TestBilling_CreateCheckoutSession_MissingAmount(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/billing/checkout",
		marshalJSON(t, map[string]any{}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid request body", body["error"],
		"missing required amount_millicents must be caught by binding")
	assert.Nil(t, body["url"])
}

func TestBilling_CreateCheckoutSession_WrongType(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/billing/checkout",
		[]byte(`{"amount_millicents":"not-a-number"}`),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid request body", body["error"])
	assert.Nil(t, body["url"])
}

func TestBilling_CreateCheckoutSession_ZeroAmount(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/billing/checkout",
		marshalJSON(t, map[string]any{"amount_millicents": 0}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "amount_millicents must be greater than zero", body["error"])
	assert.Nil(t, body["url"],
		"zero amount must be rejected before creating a checkout session")
}

func TestBilling_CreateCheckoutSession_NegativeAmount(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/billing/checkout",
		marshalJSON(t, map[string]any{"amount_millicents": -1}),
		"application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "amount_millicents must be greater than zero", body["error"])
	assert.Nil(t, body["url"])
}

func TestBilling_CreateCheckoutSession_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodPost, "/api/billing/checkout",
		marshalJSON(t, map[string]any{"amount_millicents": 1_000_000}),
		"application/json")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["url"],
		"unauthorized response must not include a checkout url")
}
