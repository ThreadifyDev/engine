package interfaces

import (
	"context"

	"github.com/threadify/engine/internal/models"
)

type InvoiceResult struct {
	ExternalInvoiceID string
	ProviderName      string
}

type InvoiceProvider interface {
	Name() string
	SkipInvoicing() bool
	IssueOverage(ctx context.Context, snapshot *models.BillingSnapshot) (*InvoiceResult, error)
}

type WebhookEvent struct {
	Type                   string
	ExternalInvoiceID      string
	ExternalCustomerID     string
	ExternalSubscriptionID string
	AttemptCount           int64
	CompanyID              string
	Tier                   string
	BillingCycle           string
}

type WebhookProvider interface {
	Name() string
	SignatureHeader() string
	VerifyAndParse(body []byte, signature string) (*WebhookEvent, error)
}
