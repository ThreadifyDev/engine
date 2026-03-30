package billing

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"threadify-go/shared/config"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/client"
)

type InvoiceProvider interface {
	Name() string
	SkipInvoicing() bool
	IssueTopupInvoice(snapshot *BillingSnapshot) (*InvoiceResult, error)
	CreateCheckoutSession(params CheckoutSessionParams) (string, error)
}

type WebhookProvider interface {
	Name() string
	SignatureHeader() string
	VerifyAndParse(body []byte, signature string) (*WebhookEvent, error)
}

// isStripeResourceMissingError checks if the error is a Stripe resource_missing error
func isStripeResourceMissingError(err error) bool {
	if err == nil {
		return false
	}
	var stripeErr *stripe.Error
	if errors.As(err, &stripeErr) {
		return stripeErr.Code == stripe.ErrorCodeResourceMissing
	}
	return false
}

type BillingProvider interface {
	InvoiceProvider
	WebhookProvider
}

type ProviderFactory interface {
	Name() string
	Build(cfg config.BillingConfig) (BillingProvider, error)
}

var defaultFactory ProviderFactory = &StripeProviderFactory{}

func InitializeProvider(cfg config.BillingConfig) (BillingProvider, error) {
	if cfg.Provider == "" || cfg.Provider == "noop" {
		return NewNoOpBillingProvider(), nil
	}

	if defaultFactory.Name() != cfg.Provider {
		return nil, fmt.Errorf("billing: unknown provider %q", cfg.Provider)
	}

	return defaultFactory.Build(cfg)
}

type NoOpBillingProvider struct{}

func NewNoOpBillingProvider() BillingProvider {
	return &NoOpBillingProvider{}
}

func (p *NoOpBillingProvider) Name() string            { return "noop" }
func (p *NoOpBillingProvider) SkipInvoicing() bool     { return true }
func (p *NoOpBillingProvider) SignatureHeader() string { return "" }

func (p *NoOpBillingProvider) IssueTopupInvoice(_ *BillingSnapshot) (*InvoiceResult, error) {
	return &InvoiceResult{ExternalInvoiceID: "", ProviderName: "noop"}, nil
}

func (p *NoOpBillingProvider) VerifyAndParse(_ []byte, _ string) (*WebhookEvent, error) {
	return nil, nil
}

func (p *NoOpBillingProvider) CreateCheckoutSession(_ CheckoutSessionParams) (string, error) {
	return "https://example.com/checkout", nil
}

func (p *NoOpBillingProvider) CancelSubscription(_ string) error { return nil }

const (
	stripeSignatureHeader           = "Stripe-Signature"
	stripeEventInvoicePaid          = "invoice.paid"
	stripeEventInvoicePaymentFailed = "invoice.payment_failed"
	stripeEventCheckoutCompleted    = "checkout.session.completed"
)

type StripeBillingProvider struct {
	apiKey        string
	webhookSecret string
	api           *client.API
}

func NewStripeBillingProvider(apiKey, webhookSecret string) BillingProvider {
	return &StripeBillingProvider{
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		api:           client.New(apiKey, nil),
	}
}

func (p *StripeBillingProvider) Name() string            { return "stripe" }
func (p *StripeBillingProvider) SkipInvoicing() bool     { return false }
func (p *StripeBillingProvider) SignatureHeader() string { return stripeSignatureHeader }

func (p *StripeBillingProvider) IssueTopupInvoice(snapshot *BillingSnapshot) (*InvoiceResult, error) {
	if snapshot.ExternalCustomerID == "" {
		return nil, errors.New("stripe: missing external_customer_id on billing snapshot")
	}

	if snapshot.TotalCents <= 0 {
		return &InvoiceResult{ExternalInvoiceID: "", ProviderName: "stripe"}, nil
	}

	invParams := &stripe.InvoiceParams{
		Customer:         stripe.String(snapshot.ExternalCustomerID),
		AutoAdvance:      stripe.Bool(true),
		CollectionMethod: stripe.String(string(stripe.InvoiceCollectionMethodChargeAutomatically)),
		Description: stripe.String(fmt.Sprintf("Threadify Credit Top-up — %s to %s",
			snapshot.PeriodStart.Format("Jan 2, 2006"),
			snapshot.PeriodEnd.Format("Jan 2, 2006"),
		)),
	}
	invParams.AddMetadata("snapshot_id", snapshot.ID)
	invParams.AddMetadata("company_id", snapshot.CompanyID)

	inv, err := p.api.Invoices.New(invParams)
	if err != nil {
		return nil, fmt.Errorf("create top-up invoice: %w", err)
	}

	itemParams := &stripe.InvoiceItemParams{
		Customer:    stripe.String(snapshot.ExternalCustomerID),
		Amount:      stripe.Int64(snapshot.TotalCents),
		Currency:    stripe.String("usd"),
		Description: stripe.String("Threadify Credit Top-up"),
		Invoice:     stripe.String(inv.ID),
	}
	itemParams.AddMetadata("snapshot_id", snapshot.ID)
	itemParams.AddMetadata("type", "credit_topup")

	if _, err := p.api.InvoiceItems.New(itemParams); err != nil {
		return nil, fmt.Errorf("stripe: create top-up invoice item: %w", err)
	}

	finalParams := &stripe.InvoiceFinalizeInvoiceParams{
		AutoAdvance: stripe.Bool(true),
	}
	if _, err := p.api.Invoices.FinalizeInvoice(inv.ID, finalParams); err != nil {
		return nil, fmt.Errorf("stripe: finalize top-up invoice: %w", err)
	}

	return &InvoiceResult{
		ExternalInvoiceID: inv.ID,
		ProviderName:      "stripe",
	}, nil
}

func (p *StripeBillingProvider) VerifyAndParse(body []byte, signature string) (*WebhookEvent, error) {
	event, err := stripe.ConstructEvent(body, signature, p.webhookSecret, stripe.WithIgnoreAPIVersionMismatch())
	if err != nil {
		return nil, fmt.Errorf("stripe: signature verification failed: %w", err)
	}

	switch event.Type {
	case stripeEventInvoicePaid, stripeEventInvoicePaymentFailed:
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal %s: %w", event.Type, err)
		}

		var customerID string
		if inv.Customer != nil {
			customerID = inv.Customer.ID
		}

		return &WebhookEvent{
			Type:               string(event.Type),
			ExternalInvoiceID:  inv.ID,
			ExternalCustomerID: customerID,
			AttemptCount:       inv.AttemptCount,
		}, nil

	case stripeEventCheckoutCompleted:
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal %s: %w", event.Type, err)
		}

		var customerID string
		if session.Customer != nil {
			customerID = session.Customer.ID
		}

		var initialAmount int64
		if amt, ok := session.Metadata["initial_amount"]; ok {
			initialAmount, _ = strconv.ParseInt(amt, 10, 64)
		} else {
			initialAmount = session.AmountTotal * 1000
		}

		var maxMonthly int64
		if max, ok := session.Metadata["max_monthly"]; ok {
			maxMonthly, _ = strconv.ParseInt(max, 10, 64)
		}

		return &WebhookEvent{
			Type:                 string(event.Type),
			ExternalCustomerID:   customerID,
			AmountMillicents:     initialAmount,
			MaxMonthlyMillicents: maxMonthly,
			PeriodStart:          time.Unix(session.Created, 0).UTC(),
			Metadata:             session.Metadata,
		}, nil

	default:
		return nil, nil
	}
}

func (p *StripeBillingProvider) CreateCheckoutSession(
	checkoutParams CheckoutSessionParams,
) (string, error) {
	amountCents := checkoutParams.InitialAmountMillicents / 1000
	if amountCents <= 0 {
		return "", errors.New("stripe: initial amount must be greater than zero")
	}

	params := &stripe.CheckoutSessionParams{
		SuccessURL: stripe.String(checkoutParams.SuccessURL),
		CancelURL:  stripe.String(checkoutParams.CancelURL),
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			SetupFutureUsage: stripe.String(string(stripe.PaymentIntentSetupFutureUsageOffSession)),
		},
	}

	if checkoutParams.ExternalCustomerID != "" {
		params.Customer = stripe.String(checkoutParams.ExternalCustomerID)
	} else {
		params.CustomerCreation = stripe.String("always")
	}

	params.LineItems = []*stripe.CheckoutSessionLineItemParams{
		{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("usd"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String("Threadify Credits"),
					Description: stripe.String("Credit top-up"),
				},
				UnitAmount: stripe.Int64(amountCents),
			},
		},
	}

	params.AddMetadata("company_id", checkoutParams.CompanyID)
	params.AddMetadata("initial_amount", strconv.FormatInt(checkoutParams.InitialAmountMillicents, 10))

	session, err := p.api.CheckoutSessions.New(params)
	if err != nil {
		// If customer doesn't exist, retry with customer creation
		if checkoutParams.ExternalCustomerID != "" && isStripeResourceMissingError(err) {
			params.Customer = nil
			params.CustomerCreation = stripe.String("always")
			session, err = p.api.CheckoutSessions.New(params)
			if err != nil {
				return "", fmt.Errorf("stripe: create checkout session (retry): %w", err)
			}
			return session.URL, nil
		}
		return "", fmt.Errorf("stripe: create checkout session: %w", err)
	}
	return session.URL, nil
}

type StripeProviderFactory struct{}

func (f *StripeProviderFactory) Name() string { return "stripe" }

func (f *StripeProviderFactory) Build(cfg config.BillingConfig) (BillingProvider, error) {
	if cfg.WebhookSecret == "" {
		return nil, errors.New("stripe: missing webhook_secret in billing config")
	}
	if cfg.SecretKey == "" {
		return nil, errors.New("stripe: missing secret_key in billing config")
	}

	return NewStripeBillingProvider(cfg.SecretKey, cfg.WebhookSecret), nil
}
