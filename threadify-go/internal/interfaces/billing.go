package interfaces

import (
	"context"
	"threadify-go/shared/billing"
	billingmodels "threadify-go/shared/models"
)

type InvoiceProvider interface {
	billing.InvoiceProvider
}

//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/billing_mocks.go -source=billing.go
type WebhookProvider interface {
	Name() string
	SignatureHeader() string
	VerifyAndParse(payload []byte, signature string) (*billingmodels.WebhookEvent, error)
}

type BillingWebhookService interface {
	LinkAndMarkSnapshotPaid(ctx context.Context, snapshotID string, externalInvoiceID string) error
	MarkSnapshotPaid(ctx context.Context, externalInvoiceID string) error
	MarkSnapshotFailed(ctx context.Context, externalInvoiceID string) error
	FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*billingmodels.BillingSnapshot, error)
	ApplyCreditTopup(ctx context.Context, snapshot *billingmodels.BillingSnapshot) error
	ClearCreditTopupPending(ctx context.Context, companyID string) error
	ProvisionSubscription(ctx context.Context, companyID string, externalCustomerID string, initialAmount int64) error
	UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthly int64) error
}
