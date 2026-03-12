package billing

import "time"

type PlanTier string

const (
	PlanTierStarter PlanTier = "starter"
	PlanTierGrowth  PlanTier = "growth"
)

type BillingCycle string

const (
	BillingCycleMonthly BillingCycle = "monthly"
	BillingCycleYearly  BillingCycle = "yearly"
)

type SnapshotReason string

const (
	SnapshotReasonOverageOnly    SnapshotReason = "overage_only"
	SnapshotReasonMonthlyRenewal SnapshotReason = "monthly_renewal"
	SnapshotReasonYearlyRenewal  SnapshotReason = "yearly_renewal"
)

type PaymentStatus string

const (
	PaymentStatusNoCharge PaymentStatus = "no_charge"
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusPaid     PaymentStatus = "paid"
	PaymentStatusFailed   PaymentStatus = "failed"
)

type InvoiceLineItem struct {
	Meter       string `json:"meter"`
	OverageQty  int64  `json:"overageQty"`
	UnitLabel   string `json:"unitLabel"`
	RateCents   int    `json:"rateCents"`
	AmountCents int64  `json:"amountCents"`
}

type BillingSnapshot struct {
	ID                      string            `json:"id"`
	CompanyID               string            `json:"companyId"`
	Tier                    PlanTier          `json:"tier"`
	Reason                  SnapshotReason    `json:"reason"`
	PeriodStart             time.Time         `json:"periodStart"`
	PeriodEnd               time.Time         `json:"periodEnd"`
	IsCycleEnd              bool              `json:"isCycleEnd"`
	IngressBalanceFinal     int64             `json:"ingressBalanceFinal"`
	EgressBalanceFinal      int64             `json:"egressBalanceFinal"`
	MaxIngress              int64             `json:"maxIngress"`
	MaxEgress               int64             `json:"maxEgress"`
	LineItems               []InvoiceLineItem `json:"lineItems"`
	TotalCents              int64             `json:"totalCents"`
	ProviderName            string            `json:"providerName"`
	ExternalInvoiceID       string            `json:"externalInvoiceId"`
	ExternalCustomerID      string            `json:"externalCustomerId"`
	ExternalSubscriptionID  string            `json:"externalSubscriptionId"`
	PaymentStatus           PaymentStatus     `json:"paymentStatus"`
	ConsecutiveOverageCount int               `json:"consecutiveOverageCount"`
	CreatedAt               time.Time         `json:"createdAt"`
}

func (s *BillingSnapshot) HasOverage() bool {
	return s.IngressBalanceFinal < 0 || s.EgressBalanceFinal < 0
}

type InvoiceResult struct {
	ExternalInvoiceID string
	ProviderName      string
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
