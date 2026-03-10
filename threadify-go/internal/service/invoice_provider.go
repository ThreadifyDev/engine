package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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
	p.logger.Warn("stripe invoice provider not yet implemented",
		zap.String("company_id", snapshot.CompanyID),
		zap.Int64("total_cents", snapshot.TotalCents),
	)
	return nil, errors.New("stripe invoice provider not implemented")
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
