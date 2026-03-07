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

	CreateInvoice(ctx context.Context, snapshot *models.BillingSnapshot) (*InvoiceResult, error)
}
