package billing

import (
	"encoding/json"
	"errors"
	"fmt"

	"threadify-go/shared/config"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/invoice"
	"github.com/stripe/stripe-go/v82/invoiceitem"
)

type CheckoutSessionParams struct {
	CompanyID      string
	Tier           string
	BillingCycle   string
	SuccessURL     string
	CancelURL      string
	ProviderParams map[string]interface{}
}

type InvoiceProvider interface {
	Name() string
	SkipInvoicing() bool
	IssueOverage(snapshot *BillingSnapshot) (*InvoiceResult, error)
}

type CheckoutSessionProvider interface {
	CreateCheckoutSession(params *CheckoutSessionParams) (string, error)
}

type WebhookProvider interface {
	Name() string
	SignatureHeader() string
	VerifyAndParse(body []byte, signature string) (*WebhookEvent, error)
}

type BillingProvider interface {
	CheckoutSessionProvider
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

func (p *NoOpBillingProvider) CreateCheckoutSession(_ *CheckoutSessionParams) (string, error) {
	return "https://noop.checkout.url", nil
}

func (p *NoOpBillingProvider) IssueOverage(_ *BillingSnapshot) (*InvoiceResult, error) {
	return &InvoiceResult{ExternalInvoiceID: "", ProviderName: "noop"}, nil
}

func (p *NoOpBillingProvider) VerifyAndParse(_ []byte, _ string) (*WebhookEvent, error) {
	return nil, nil
}

const (
	stripeSignatureHeader           = "Stripe-Signature"
	stripeEventInvoicePaid          = "invoice.paid"
	stripeEventInvoicePaymentFailed = "invoice.payment_failed"
	stripeEventSubscriptionDeleted  = "customer.subscription.deleted"
	stripeEventCheckoutCompleted    = "checkout.session.completed"

	stripeMetaCompanyID    = "company_id"
	stripeMetaTier         = "tier"
	stripeMetaBillingCycle = "billing_cycle"
)

type StripeBillingProvider struct {
	apiKey        string
	webhookSecret string
	sessionClient session.Client
	invoiceClient invoice.Client
	itemClient    invoiceitem.Client
}

func NewStripeBillingProvider(apiKey, webhookSecret string, backend stripe.Backend) BillingProvider {
	return &StripeBillingProvider{
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		sessionClient: session.Client{B: backend, Key: apiKey},
		invoiceClient: invoice.Client{B: backend, Key: apiKey},
		itemClient:    invoiceitem.Client{B: backend, Key: apiKey},
	}
}

func (p *StripeBillingProvider) Name() string            { return "stripe" }
func (p *StripeBillingProvider) SkipInvoicing() bool     { return false }
func (p *StripeBillingProvider) SignatureHeader() string { return stripeSignatureHeader }

func (p *StripeBillingProvider) CreateCheckoutSession(params *CheckoutSessionParams) (string, error) {
	priceIDVal, ok := params.ProviderParams["price_id"]
	if !ok {
		return "", errors.New("stripe: missing stripe_price_id in ProviderParams")
	}

	priceID, ok := priceIDVal.(string)
	if !ok || priceID == "" {
		return "", errors.New("stripe: invalid stripe_price_id in ProviderParams")
	}

	stripeParams := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Metadata: map[string]string{
			stripeMetaCompanyID:    params.CompanyID,
			stripeMetaTier:         params.Tier,
			stripeMetaBillingCycle: params.BillingCycle,
		},
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
	}

	stripeSession, err := p.sessionClient.New(stripeParams)
	if err != nil {
		return "", fmt.Errorf("stripe: checkout session: %w", err)
	}

	return stripeSession.URL, nil
}

func (p *StripeBillingProvider) IssueOverage(snapshot *BillingSnapshot) (*InvoiceResult, error) {
	if snapshot.ExternalCustomerID == "" {
		return nil, errors.New("stripe: missing external_customer_id on billing snapshot")
	}

	for _, item := range snapshot.LineItems {
		desc := fmt.Sprintf("Threadify overage: %s (%s)", item.Meter, item.UnitLabel)
		params := &stripe.InvoiceItemParams{
			Customer:    stripe.String(snapshot.ExternalCustomerID),
			Amount:      stripe.Int64(item.AmountCents),
			Currency:    stripe.String("usd"),
			Description: stripe.String(desc),
		}
		params.AddMetadata("snapshot_id", snapshot.ID)
		params.AddMetadata("meter", item.Meter)
		params.AddMetadata("overage_qty", fmt.Sprintf("%d", item.OverageQty))

		if snapshot.ExternalSubscriptionID != "" {
			params.Subscription = stripe.String(snapshot.ExternalSubscriptionID)
		}

		if _, err := p.itemClient.New(params); err != nil {
			return nil, fmt.Errorf("stripe: create invoice item for %s: %w", item.Meter, err)
		}
	}

	if snapshot.Reason == SnapshotReasonOverageOnly {
		invParams := &stripe.InvoiceParams{
			Customer:         stripe.String(snapshot.ExternalCustomerID),
			AutoAdvance:      stripe.Bool(true),
			CollectionMethod: stripe.String(string(stripe.InvoiceCollectionMethodChargeAutomatically)),
			Description: stripe.String(fmt.Sprintf("Threadify usage overage — %s to %s",
				snapshot.PeriodStart.Format("Jan 2, 2006"),
				snapshot.PeriodEnd.Format("Jan 2, 2006"),
			)),
		}
		invParams.AddMetadata("snapshot_id", snapshot.ID)
		invParams.AddMetadata("company_id", snapshot.CompanyID)

		inv, err := p.invoiceClient.New(invParams)
		if err != nil {
			return nil, fmt.Errorf("stripe: create overage invoice: %w", err)
		}

		return &InvoiceResult{
			ExternalInvoiceID: inv.ID,
			ProviderName:      "stripe",
		}, nil
	}

	return &InvoiceResult{ExternalInvoiceID: "", ProviderName: "stripe"}, nil
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

		customerID := ""
		if inv.Customer != nil {
			customerID = inv.Customer.ID
		}

		return &WebhookEvent{
			Type:               string(event.Type),
			ExternalInvoiceID:  inv.ID,
			ExternalCustomerID: customerID,
			AttemptCount:       inv.AttemptCount,
		}, nil

	case stripeEventSubscriptionDeleted:
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal %s: %w", event.Type, err)
		}

		customerID := ""
		if sub.Customer != nil {
			customerID = sub.Customer.ID
		}

		return &WebhookEvent{
			Type:                   stripeEventSubscriptionDeleted,
			ExternalSubscriptionID: sub.ID,
			ExternalCustomerID:     customerID,
		}, nil

	case stripeEventCheckoutCompleted:
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &cs); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal %s: %w", event.Type, err)
		}

		customerID, subID := "", ""
		if cs.Customer != nil {
			customerID = cs.Customer.ID
		}
		if cs.Subscription != nil {
			subID = cs.Subscription.ID
		}

		return &WebhookEvent{
			Type:                   stripeEventCheckoutCompleted,
			ExternalCustomerID:     customerID,
			ExternalSubscriptionID: subID,
			CompanyID:              cs.Metadata[stripeMetaCompanyID],
			Tier:                   cs.Metadata[stripeMetaTier],
			BillingCycle:           cs.Metadata[stripeMetaBillingCycle],
		}, nil

	default:
		return nil, nil
	}
}

type StripeProviderFactory struct{}

func (f *StripeProviderFactory) Name() string { return "stripe" }

func (f *StripeProviderFactory) Build(cfg config.BillingConfig) (BillingProvider, error) {
	if cfg.WebhookSecret == "" {
		return nil, errors.New("stripe: missing webhook_secret in billing config")
	}

	secretKey := cfg.SecretKey
	if secretKey == "" {
		return nil, errors.New("stripe: missing secret_key in providers.stripe")
	}

	backend := stripe.GetBackend(stripe.APIBackend)
	return NewStripeBillingProvider(secretKey, cfg.WebhookSecret, backend), nil
}
