package billing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v82"
	"threadify-go/shared/domain"
)

func TestScopedStripeClientsPreserveInvoiceAndCheckoutRequests(t *testing.T) {
	var paths []string
	var customers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/invoices":
			require.Equal(t, "cus_test", r.Form.Get("customer"))
			require.Equal(t, "company", r.Form.Get("metadata[company_id]"))
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "in_test"})
		case "/v1/invoiceitems":
			require.Equal(t, "in_test", r.Form.Get("invoice"))
			require.Equal(t, "125", r.Form.Get("amount"))
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ii_test"})
		case "/v1/invoices/in_test/finalize":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "in_test"})
		case "/v1/checkout/sessions":
			customers = append(customers, r.Form.Get("customer"))
			if len(customers) == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"resource_missing","message":"No such customer"}}`))
				return
			}
			require.Equal(t, "always", r.Form.Get("customer_creation"))
			require.Equal(t, "125", r.Form.Get("line_items[0][price_data][unit_amount]"))
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "cs_test", "url": "https://example.test/checkout"})
		default:
			t.Errorf("unexpected Stripe endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider := NewStripeBillingProvider("test-key", "whsec_test").(*StripeBillingProvider)
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(server.URL), HTTPClient: server.Client(), MaxNetworkRetries: stripe.Int64(0)})
	provider.api.Invoices.B = backend
	provider.api.InvoiceItems.B = backend
	provider.api.CheckoutSessions.B = backend
	result, err := provider.IssueTopupInvoice(&domain.BillingSnapshot{ID: "snapshot", CompanyID: "company", ExternalCustomerID: "cus_test", TotalCents: 125, PeriodStart: time.Now(), PeriodEnd: time.Now()})
	require.NoError(t, err)
	require.Equal(t, "in_test", result.ExternalInvoiceID)
	checkout, err := provider.CreateCheckoutSession(domain.CheckoutSessionParams{CompanyID: "company", ExternalCustomerID: "cus_missing", InitialAmountMillicents: 125000})
	require.NoError(t, err)
	require.Equal(t, "https://example.test/checkout", checkout)
	require.Equal(t, []string{"/v1/invoices", "/v1/invoiceitems", "/v1/invoices/in_test/finalize", "/v1/checkout/sessions", "/v1/checkout/sessions"}, paths)
	require.Equal(t, []string{"cus_missing", ""}, customers)
}
