package models

import (
	"time"
)

const (
	StatusActive              = "active"
	StatusCancelled           = "cancelled"
	StatusPendingCancellation = "pending_cancellation"

	CreditDisabled int64 = 0

	MeterLLMTokenUsage      = "llm_tokens"
	MeterContractExecution  = "contract_execution"
	MeterContractVersion    = "contract_version"
	MeterIngress            = "ingress"
	MeterEgress             = "egress"
	MeterSeatCreate         = "seat_create"
	MeterCreditSpend        = "credit_spend"
	MeterCreditTopup        = "credit_topup"
	MeterCreditTopupRequest = "credit_topup_request"

	FieldEventID           = "event_id"
	FieldCompanyID         = "company_id"
	FieldMeter             = "meter"
	FieldAmount            = "amount"
	FieldBillingCycleStart = "billing_cycle_start"
	FieldTimestamp         = "timestamp"
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

type CheckoutSessionParams struct {
	CompanyID               string
	Tier                    string
	InitialAmountMillicents int64
	SuccessURL              string
	CancelURL               string
	ExternalCustomerID      string
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
	return a.CreditMaxMonthlyChargeMillicents > CreditDisabled
}
