package billing

import (
	"github.com/stretchr/testify/require"
	"testing"
	"threadify-go/shared/config"
	"threadify-go/shared/domain"
)

func TestDisabledBillingDoesNotProcessPayments(t *testing.T) {
	for _, name := range []string{"", "noop"} {
		provider, err := InitializeProvider(config.BillingConfig{Provider: name})
		require.NoError(t, err)
		require.True(t, provider.SkipInvoicing())
		url, err := provider.CreateCheckoutSession(domain.CheckoutSessionParams{InitialAmountMillicents: 100000})
		require.ErrorIs(t, err, ErrCheckoutUnavailable)
		require.Empty(t, url)
		event, err := provider.VerifyAndParse([]byte(`{"type":"invoice.paid","amount_millicents":100000}`), "")
		require.NoError(t, err)
		require.Nil(t, event, "disabled billing must not accept unsigned credit grants")
	}
}

func TestUnsupportedBillingProviderFailsClearly(t *testing.T) {
	_, err := InitializeProvider(config.BillingConfig{Provider: "unavailable-provider"})
	require.ErrorContains(t, err, "only noop is available")
}
