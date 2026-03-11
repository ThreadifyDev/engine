package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/invoice"
	"github.com/stripe/stripe-go/v82/invoiceitem"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"go.uber.org/zap"
)

type InvoiceProviderBuilder func(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.InvoiceProvider, error)

const (
	providerNoop     = "noop"
	providerStripe   = "stripe"
	providerPaystack = "paystack"

	providerSecretKey = "secret_key"
)

const (
	stripeSignatureHeader   = "Stripe-Signature"
	paystackSignatureHeader = "X-Paystack-Signature"
)

var (
	errInvalidProviderName    = errors.New("provider name is required")
	errInvalidProviderBuilder = errors.New("provider builder is required")
)

var (
	invoiceProviderMu       sync.RWMutex
	invoiceProviderBuilders = map[string]InvoiceProviderBuilder{
		providerNoop: func(_ *config.BillingConfig, logger *zap.Logger) (interfaces.InvoiceProvider, error) {
			return NewNoOpInvoiceProvider(logger), nil
		},
		providerStripe: func(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.InvoiceProvider, error) {
			apiKey := getProviderSetting(cfg, providerStripe, providerSecretKey)
			if !isValidProviderSecret(apiKey) {
				return nil, fmt.Errorf("billing configuration for %s is missing %s", providerStripe, providerSecretKey)
			}
			return NewStripeInvoiceProvider(apiKey, logger), nil
		},
		providerPaystack: func(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.InvoiceProvider, error) {
			secretKey := getProviderSetting(cfg, providerPaystack, providerSecretKey)
			if !isValidProviderSecret(secretKey) {
				return nil, fmt.Errorf("billing configuration for %s is missing %s", providerPaystack, providerSecretKey)
			}
			return NewPaystackInvoiceProvider(secretKey, logger), nil
		},
	}
)

func NewInvoiceProvider(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.InvoiceProvider, error) {
	providerName := providerNoop
	if cfg != nil && strings.TrimSpace(cfg.Provider) != "" {
		providerName = strings.TrimSpace(strings.ToLower(cfg.Provider))
	}

	invoiceProviderMu.RLock()
	builder, ok := invoiceProviderBuilders[providerName]
	invoiceProviderMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported billing provider %q", providerName)
	}

	provider, err := builder(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("build invoice provider %q: %w", providerName, err)
	}
	return provider, nil
}

func getProviderSetting(cfg *config.BillingConfig, provider, key string) string {
	if cfg == nil || cfg.Providers == nil {
		return ""
	}

	normalizedProvider := strings.TrimSpace(strings.ToLower(provider))
	if normalizedProvider == "" {
		return ""
	}

	settings, ok := cfg.Providers[normalizedProvider]
	if !ok {
		return ""
	}

	return strings.TrimSpace(settings[key])
}

func isValidProviderSecret(secret string) bool {
	s := strings.TrimSpace(secret)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "${") || strings.HasPrefix(s, "$ENV") {
		return false
	}
	return true
}

func RegisterInvoiceProvider(name string, builder InvoiceProviderBuilder) error {
	normalized := strings.TrimSpace(strings.ToLower(name))
	if normalized == "" {
		return errInvalidProviderName
	}
	if builder == nil {
		return errInvalidProviderBuilder
	}

	invoiceProviderMu.Lock()
	defer invoiceProviderMu.Unlock()

	if _, exists := invoiceProviderBuilders[normalized]; exists {
		return fmt.Errorf("provider %q is already registered", normalized)
	}
	invoiceProviderBuilders[normalized] = builder
	return nil
}

type NoOpInvoiceProvider struct {
	logger *zap.Logger
}

func NewNoOpInvoiceProvider(logger *zap.Logger) *NoOpInvoiceProvider {
	return &NoOpInvoiceProvider{logger: logger}
}

func (p *NoOpInvoiceProvider) Name() string        { return providerNoop }
func (p *NoOpInvoiceProvider) SkipInvoicing() bool { return true }

func (p *NoOpInvoiceProvider) IssueOverage(_ context.Context, snapshot *models.BillingSnapshot) (*interfaces.InvoiceResult, error) {
	p.logger.Info("invoice generated (no-op)",
		zap.String("company_id", snapshot.CompanyID),
		zap.String("snapshot_id", snapshot.ID),
		zap.Int64("total_cents", snapshot.TotalCents),
		zap.Int("line_items", len(snapshot.LineItems)),
		zap.Int("consecutive_overage", snapshot.ConsecutiveOverageCount),
	)
	return &interfaces.InvoiceResult{
		ExternalInvoiceID: "",
		ProviderName:      providerNoop,
	}, nil
}

type StripeInvoiceProvider struct {
	apiKey string
	logger *zap.Logger
}

func NewStripeInvoiceProvider(apiKey string, logger *zap.Logger) *StripeInvoiceProvider {
	return &StripeInvoiceProvider{apiKey: apiKey, logger: logger}
}

func (p *StripeInvoiceProvider) Name() string        { return providerStripe }
func (p *StripeInvoiceProvider) SkipInvoicing() bool { return false }

func (p *StripeInvoiceProvider) IssueOverage(_ context.Context, snapshot *models.BillingSnapshot) (*interfaces.InvoiceResult, error) {
	if snapshot.ExternalCustomerID == "" {
		return nil, errors.New("stripe: missing external_customer_id on billing snapshot")
	}

	stripe.Key = p.apiKey

	iiClient := invoiceitem.Client{B: stripe.GetBackend(stripe.APIBackend), Key: p.apiKey}
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

		if _, err := iiClient.New(params); err != nil {
			p.logger.Error("stripe: failed to create invoice item",
				zap.String("meter", item.Meter),
				zap.String("company_id", snapshot.CompanyID),
				zap.Error(err),
			)
			return nil, fmt.Errorf("stripe: create invoice item for %s: %w", item.Meter, err)
		}
	}

	if snapshot.Reason == models.SnapshotReasonOverageOnly {
		invClient := invoice.Client{B: stripe.GetBackend(stripe.APIBackend), Key: p.apiKey}
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

		inv, err := invClient.New(invParams)
		if err != nil {
			return nil, fmt.Errorf("stripe: create overage invoice: %w", err)
		}

		p.logger.Info("stripe: immediate overage invoice created",
			zap.String("invoice_id", inv.ID),
			zap.String("company_id", snapshot.CompanyID),
			zap.Int64("total_cents", snapshot.TotalCents),
		)

		return &interfaces.InvoiceResult{
			ExternalInvoiceID: inv.ID,
			ProviderName:      providerStripe,
		}, nil
	}

	p.logger.Info("stripe: overage items attached — Stripe will collect on next subscription invoice",
		zap.String("company_id", snapshot.CompanyID),
		zap.String("subscription_id", snapshot.ExternalSubscriptionID),
		zap.Int64("total_cents", snapshot.TotalCents),
		zap.Int("line_items", len(snapshot.LineItems)),
	)

	return &interfaces.InvoiceResult{
		ExternalInvoiceID: "",
		ProviderName:      providerStripe,
	}, nil
}

type PaystackInvoiceProvider struct {
	secretKey string
	logger    *zap.Logger
}

func NewPaystackInvoiceProvider(secretKey string, logger *zap.Logger) *PaystackInvoiceProvider {
	return &PaystackInvoiceProvider{secretKey: secretKey, logger: logger}
}

func (p *PaystackInvoiceProvider) Name() string        { return providerPaystack }
func (p *PaystackInvoiceProvider) SkipInvoicing() bool { return false }

func (p *PaystackInvoiceProvider) IssueOverage(_ context.Context, snapshot *models.BillingSnapshot) (*interfaces.InvoiceResult, error) {
	p.logger.Warn("paystack invoice provider not yet implemented",
		zap.String("company_id", snapshot.CompanyID),
		zap.Int64("total_cents", snapshot.TotalCents),
	)
	return nil, errors.New("paystack invoice provider not implemented")
}

type WebhookProviderBuilder func(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.WebhookProvider, error)

var (
	webhookProviderMu       sync.RWMutex
	webhookProviderBuilders = map[string]WebhookProviderBuilder{
		providerNoop: func(_ *config.BillingConfig, logger *zap.Logger) (interfaces.WebhookProvider, error) {
			return &NoOpWebhookProvider{logger: logger}, nil
		},
		providerStripe: func(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.WebhookProvider, error) {
			if cfg.WebhookSecret == "" {
				return nil, fmt.Errorf("billing configuration for %s is missing webhook_secret", providerStripe)
			}
			return NewStripeWebhookProvider(cfg.WebhookSecret, logger), nil
		},
		providerPaystack: func(_ *config.BillingConfig, logger *zap.Logger) (interfaces.WebhookProvider, error) {
			return &NoOpWebhookProvider{logger: logger}, nil
		},
	}
)

func NewWebhookProvider(cfg *config.BillingConfig, logger *zap.Logger) (interfaces.WebhookProvider, error) {
	providerName := providerNoop
	if cfg != nil && strings.TrimSpace(cfg.Provider) != "" {
		providerName = strings.TrimSpace(strings.ToLower(cfg.Provider))
	}

	webhookProviderMu.RLock()
	builder, ok := webhookProviderBuilders[providerName]
	webhookProviderMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported webhook provider %q", providerName)
	}

	provider, err := builder(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("build webhook provider %q: %w", providerName, err)
	}
	return provider, nil
}

type NoOpWebhookProvider struct {
	logger *zap.Logger
}

func (p *NoOpWebhookProvider) Name() string            { return providerNoop }
func (p *NoOpWebhookProvider) SignatureHeader() string { return "" }
func (p *NoOpWebhookProvider) VerifyAndParse(body []byte, signature string) (*interfaces.WebhookEvent, error) {
	p.logger.Debug("webhook received (no-op)", zap.Int("body_len", len(body)))
	return nil, nil
}

const (
	stripeEventInvoicePaid          = "invoice.paid"
	stripeEventInvoicePaymentFailed = "invoice.payment_failed"
	stripeEventSubscriptionDeleted  = "customer.subscription.deleted"
	stripeEventCheckoutCompleted    = "checkout.session.completed"
	stripeMetaCompanyID             = "company_id"
	stripeMetaTier                  = "tier"
	stripeMetaBillingCycle          = "billing_cycle"
)

type StripeWebhookProvider struct {
	webhookSecret string
	logger        *zap.Logger
}

func NewStripeWebhookProvider(webhookSecret string, logger *zap.Logger) *StripeWebhookProvider {
	return &StripeWebhookProvider{webhookSecret: webhookSecret, logger: logger}
}

func (p *StripeWebhookProvider) Name() string            { return providerStripe }
func (p *StripeWebhookProvider) SignatureHeader() string { return stripeSignatureHeader }

func (p *StripeWebhookProvider) VerifyAndParse(body []byte, signature string) (*interfaces.WebhookEvent, error) {
	event, err := stripe.ConstructEvent(body, signature, p.webhookSecret, stripe.WithIgnoreAPIVersionMismatch())
	if err != nil {
		return nil, fmt.Errorf("stripe: signature verification failed: %w", err)
	}

	switch event.Type {
	case stripeEventInvoicePaid, stripeEventInvoicePaymentFailed:
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return nil, fmt.Errorf("stripe: failed to unmarshal %s payload: %w", event.Type, err)
		}

		var customerID string
		if inv.Customer != nil {
			customerID = inv.Customer.ID
		}

		return &interfaces.WebhookEvent{
			Type:               string(event.Type),
			ExternalInvoiceID:  inv.ID,
			ExternalCustomerID: customerID,
			AttemptCount:       inv.AttemptCount,
		}, nil

	case stripeEventSubscriptionDeleted:
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return nil, fmt.Errorf("stripe: failed to unmarshal %s payload: %w", event.Type, err)
		}

		var customerID string
		if sub.Customer != nil {
			customerID = sub.Customer.ID
		}

		return &interfaces.WebhookEvent{
			Type:                   stripeEventSubscriptionDeleted,
			ExternalSubscriptionID: sub.ID,
			ExternalCustomerID:     customerID,
		}, nil

	case stripeEventCheckoutCompleted:
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return nil, fmt.Errorf("stripe: failed to unmarshal %s payload: %w", event.Type, err)
		}

		var customerID, subID string
		if session.Customer != nil {
			customerID = session.Customer.ID
		}
		if session.Subscription != nil {
			subID = session.Subscription.ID
		}

		return &interfaces.WebhookEvent{
			Type:                   stripeEventCheckoutCompleted,
			ExternalCustomerID:     customerID,
			ExternalSubscriptionID: subID,
			CompanyID:              session.Metadata[stripeMetaCompanyID],
			Tier:                   session.Metadata[stripeMetaTier],
			BillingCycle:           session.Metadata[stripeMetaBillingCycle],
		}, nil

	default:
		p.logger.Debug("stripe: unhandled event type", zap.String("type", string(event.Type)))
		return nil, nil
	}
}
