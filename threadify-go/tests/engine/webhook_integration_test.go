package engine

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestWebhook_CheckoutCompleted_CreditTopup(t *testing.T) {
	user := setupTestUser(t)
	db := dbhelpers.New(env.Postgres.Pool)
	vk := newValkeyHelpers(engineApp.Valkey())

	externalCustomerID := db.GetExternalCustomerID(t, user.CompanyID)
	topupAmount := int64(5_000_000)
	balanceBefore := db.GetCreditBalance(t, user.CompanyID)

	resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
		"Type":               "checkout.session.completed",
		"ExternalCustomerID": externalCustomerID,
		"AmountMillicents":   topupAmount,
		"Metadata": map[string]string{
			"company_id": user.CompanyID,
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"webhook body: %s", resp.Body)
	assert.Equal(t, balanceBefore+topupAmount, vk.GetCreditBalance(t, user.CompanyID),
		"credit balance in valkey should increase by exactly the topup amount")
}

func TestWebhook_CheckoutCompleted_UnknownCustomer(t *testing.T) {
	user := setupTestUser(t)
	db := dbhelpers.New(env.Postgres.Pool)

	balanceBefore := db.GetCreditBalance(t, user.CompanyID)

	resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
		"Type":               "checkout.session.completed",
		"ExternalCustomerID": "cus_doesnotexist_" + uuid.NewString()[:8],
		"AmountMillicents":   int64(5_000_000),
		// No Metadata/company_id — handler returns early, no balance change expected
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, balanceBefore, db.GetCreditBalance(t, user.CompanyID),
		"unknown customer webhook must not affect any credit balance")
}

func TestWebhook_InvoicePaid_CreditsPendingTopup(t *testing.T) {
	user := setupTestUser(t)
	db := dbhelpers.New(env.Postgres.Pool)
	vk := newValkeyHelpers(engineApp.Valkey())

	invoiceID := "inv_" + uuid.NewString()[:16]
	invoiceAmountCents := int64(2_000)
	invoiceAmountMillicents := invoiceAmountCents * 1_000

	db.CreateTestInvoice(t, user.CompanyID, invoiceID, invoiceAmountCents)

	balanceBefore := vk.GetCreditBalance(t, user.CompanyID)

	resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
		"Type":              "invoice.paid",
		"ExternalInvoiceID": invoiceID,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"webhook body: %s", resp.Body)

	assert.Equal(t, balanceBefore+invoiceAmountMillicents, vk.GetCreditBalance(t, user.CompanyID),
		"invoice.paid should restore credit balance by the invoice amount")
	assert.Equal(t, "paid", db.GetInvoiceStatus(t, invoiceID),
		"invoice status must be updated to 'paid'")
}

func TestWebhook_InvoicePaid_Unmatched(t *testing.T) {
	user := setupTestUser(t)
	db := dbhelpers.New(env.Postgres.Pool)

	balanceBefore := db.GetCreditBalance(t, user.CompanyID)

	resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
		"Type":              "invoice.paid",
		"ExternalInvoiceID": "inv_" + uuid.NewString(),
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, balanceBefore, db.GetCreditBalance(t, user.CompanyID),
		"unmatched invoice.paid must not affect any credit balance")
}

func TestWebhook_InvoicePaymentFailed_MarksInvoice(t *testing.T) {
	user := setupTestUser(t)
	db := dbhelpers.New(env.Postgres.Pool)

	invoiceID := "inv_" + uuid.NewString()[:16]
	db.CreateTestInvoice(t, user.CompanyID, invoiceID, int64(1_000_000))

	resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
		"Type":              "invoice.payment_failed",
		"ExternalInvoiceID": invoiceID,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"webhook body: %s", resp.Body)

	assert.Equal(t, "failed", db.GetInvoiceStatus(t, invoiceID),
		"invoice status must be updated to 'failed'")
}

func TestWebhook_EdgeCases(t *testing.T) {
	t.Run("malformed_json", func(t *testing.T) {
		resp := httpc.DoRaw(t, http.MethodPost, "/webhook",
			[]byte(`{"Type": "checkout.session.completed", "AmountMillicents": "not-a-number"`),
			"application/json")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := enginetest.DecodeJSONBody(t, resp)
		_, hasError := body["error"]
		assert.True(t, hasError,
			"malformed JSON must return an error key; body: %s", resp.Body)
	})

	t.Run("unsupported_event_type", func(t *testing.T) {
		resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
			"Type": "unknown.event.type",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("checkout_missing_company_id", func(t *testing.T) {
		// No Metadata key at all — handler returns early with 200
		resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
			"Type":               "checkout.session.completed",
			"ExternalCustomerID": "cus_" + uuid.NewString()[:8],
			"AmountMillicents":   int64(5_000_000),
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("checkout_missing_customer_id", func(t *testing.T) {
		resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
			"Type":               "checkout.session.completed",
			"ExternalCustomerID": "",
			"AmountMillicents":   int64(5_000_000),
			"Metadata": map[string]string{
				"company_id": "co_" + uuid.NewString()[:8],
			},
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("checkout_zero_amount", func(t *testing.T) {
		resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
			"Type":               "checkout.session.completed",
			"ExternalCustomerID": "cus_" + uuid.NewString()[:8],
			"AmountMillicents":   int64(0),
			"Metadata": map[string]string{
				"company_id": "co_" + uuid.NewString()[:8],
			},
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invoice_paid_missing_id", func(t *testing.T) {
		resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
			"Type":              "invoice.paid",
			"ExternalInvoiceID": "",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invoice_failed_missing_id", func(t *testing.T) {
		resp := httpc.DoJSON(t, http.MethodPost, "/webhook", map[string]interface{}{
			"Type":              "invoice.payment_failed",
			"ExternalInvoiceID": "",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
