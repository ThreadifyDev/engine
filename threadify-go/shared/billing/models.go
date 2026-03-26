package billing

import (
	"context"
	"time"
)

const (
	StatusActive              = "active"
	StatusCancelled           = "cancelled"
	StatusPendingCancellation = "pending_cancellation"

	CreditDisabled int64 = 0

	MeterLLMTokenUsage     = "llm_tokens"
	MeterContractExecution = "contract_execution"
	MeterContractVersion   = "contract_version"
	MeterIngress           = "ingress"
	MeterEgress            = "egress"
	MeterCreditSpend       = "credit_spend"
	MeterCreditTopup       = "credit_topup"
	MeterCreditTopupRequest = "credit_topup_request"
)

type SnapshotReason string

const (
	SnapshotReasonCreditTopup SnapshotReason = "credit_topup"
)

type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusPaid    PaymentStatus = "paid"
	PaymentStatusFailed  PaymentStatus = "failed"
)

type BillingSnapshot struct {
	ID                 string
	CompanyID          string
	Reason             SnapshotReason
	PeriodStart        time.Time
	PeriodEnd          time.Time
	TotalCents         int64
	ProviderName       string
	ExternalInvoiceID  string
	ExternalCustomerID string
	PaymentStatus      PaymentStatus
	CreatedAt          time.Time
}

type InvoiceResult struct {
	ExternalInvoiceID string
	ProviderName      string
}

type WebhookEvent struct {
	Type                 string
	ExternalInvoiceID    string
	ExternalCustomerID   string
	AmountMillicents     int64
	MaxMonthlyMillicents int64
	AttemptCount         int64
	PeriodStart          time.Time
	Metadata             map[string]string
}

type CreditAccount struct {
	ID                               string
	CompanyID                        string
	ExternalCustomerID               string
	BillingCycleStart                time.Time
	CreditBalanceMillicents          int64
	CreditMinBalanceMillicents       int64
	CreditMaxMonthlyChargeMillicents int64
	CreditAutoTopupMillicents        int64
	CreditMonthlyChargedMillicents   int64
	RateLimitTPS                     int64
	PayloadLimitBytes                int64
	CreatedAt                        time.Time
	UpdatedAt                        time.Time
}

func (a *CreditAccount) IsTopupEnabled() bool {
	return a.CreditAutoTopupMillicents != CreditDisabled && a.CreditMaxMonthlyChargeMillicents != CreditDisabled
}

type PlanRepository interface {
	GetCreditAccount(ctx context.Context, companyID string) (*CreditAccount, error)
	CreateCreditAccount(ctx context.Context, account *CreditAccount) error
	GetExternalCustomerID(ctx context.Context, companyID string) (string, error)
	SetExternalCustomerID(ctx context.Context, companyID, externalCustomerID string) error
	FindCompanyByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error)
	DisableAutoTopup(ctx context.Context, companyID string) error
	UpdateMonthlyCharged(ctx context.Context, id string, amount int64) error
}

type PlanService interface {
	ChargeContract(ctx context.Context, companyID string) error
	ChargeContractVersion(ctx context.Context, companyID string) error
	DecrementEgress(ctx context.Context, companyID string, bytes int64) error
	DecrementIngress(ctx context.Context, companyID string, count int64) error
	DecrementLLMUsage(ctx context.Context, companyID string, tokens int64) error
	GetCurrentLimits(ctx context.Context, companyID string) (*CreditAccount, error)
	GetExternalCustomerID(ctx context.Context, companyID string) (string, error)
	ProvisionSubscription(ctx context.Context, companyID, externalCustomerID string, initialAmount, maxMonthly int64) error
	InvalidatePlanCache(ctx context.Context, companyID string)
	ProcessRollovers(ctx context.Context) error
	CheckPayloadSize(ctx context.Context, account *CreditAccount, payloadBytes int64) error
	CheckRateLimit(ctx context.Context, account *CreditAccount) (bool, error)
	CheckCreditAvailable(ctx context.Context, companyID, meter string, amount int64) error
}
