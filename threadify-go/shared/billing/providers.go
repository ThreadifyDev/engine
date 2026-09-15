package billing

import (
	"errors"
	"fmt"

	"threadify-go/shared/config"
	"threadify-go/shared/domain"
)

type InvoiceProvider interface {
	Name() string
	SkipInvoicing() bool
	IssueTopupInvoice(snapshot *domain.BillingSnapshot) (*domain.InvoiceResult, error)
	CreateCheckoutSession(params domain.CheckoutSessionParams) (string, error)
}

type WebhookProvider interface {
	Name() string
	SignatureHeader() string
	VerifyAndParse(body []byte, signature string) (*domain.WebhookEvent, error)
}

type BillingProvider interface {
	InvoiceProvider
	WebhookProvider
}

var ErrCheckoutUnavailable = errors.New("billing: local checkout is disabled; external billing integration is not configured")

// Billing remains disabled locally until the Registry integration is available.
func InitializeProvider(cfg config.BillingConfig) (BillingProvider, error) {
	if cfg.Provider != "" && cfg.Provider != "noop" {
		return nil, fmt.Errorf("billing: unsupported provider %q; only noop is available", cfg.Provider)
	}
	return NewNoOpBillingProvider(), nil
}

type NoOpBillingProvider struct{}

func NewNoOpBillingProvider() BillingProvider          { return &NoOpBillingProvider{} }
func (p *NoOpBillingProvider) Name() string            { return "noop" }
func (p *NoOpBillingProvider) SkipInvoicing() bool     { return true }
func (p *NoOpBillingProvider) SignatureHeader() string { return "" }
func (p *NoOpBillingProvider) IssueTopupInvoice(_ *domain.BillingSnapshot) (*domain.InvoiceResult, error) {
	return &domain.InvoiceResult{ExternalInvoiceID: "", ProviderName: "noop"}, nil
}

// Ignore payment payloads while billing is disabled; do not grant credits from
// unsigned requests or fabricate a hosted checkout URL.
func (p *NoOpBillingProvider) VerifyAndParse(_ []byte, _ string) (*domain.WebhookEvent, error) {
	return nil, nil
}
func (p *NoOpBillingProvider) CreateCheckoutSession(_ domain.CheckoutSessionParams) (string, error) {
	return "", ErrCheckoutUnavailable
}
func (p *NoOpBillingProvider) CancelSubscription(_ string) error { return nil }
