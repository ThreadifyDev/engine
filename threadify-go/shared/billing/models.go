package billing

import (
	"context"
	"time"
)

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

type SubscriptionStatus string

const (
	StatusActive    SubscriptionStatus = "active"
	StatusCancelled SubscriptionStatus = "cancelled"
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
	Meter       string
	OverageQty  int64
	UnitLabel   string
	RateCents   int
	AmountCents int64
}

type BillingSnapshot struct {
	ID                      string
	CompanyID               string
	Tier                    PlanTier
	Reason                  SnapshotReason
	PeriodStart             time.Time
	PeriodEnd               time.Time
	IsCycleEnd              bool
	IngressBalanceFinal     int64
	EgressBalanceFinal      int64
	MaxIngress              int64
	MaxEgress               int64
	LineItems               []InvoiceLineItem
	TotalCents              int64
	ProviderName            string
	ExternalInvoiceID       string
	ExternalCustomerID      string
	ExternalSubscriptionID  string
	PaymentStatus           PaymentStatus
	ConsecutiveOverageCount int
	CreatedAt               time.Time
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

type CompanyPlan struct {
	ID                     string
	CompanyID              string
	SubscriptionTier       PlanTier
	BillingCycle           BillingCycle
	ExternalCustomerID     string
	ExternalSubscriptionID string
	Status                 SubscriptionStatus
	BillingStart           time.Time
	BillingEnd             time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type UsageMeter struct {
	ID                      string
	CompanyID               string
	SubscriptionTier        PlanTier
	BillingCycleStart       time.Time
	BillingEnd              time.Time
	BandwidthIngressBalance int64
	BandwidthEgressBalance  int64
	MaxBandwidthIngress     int64
	MaxBandwidthEgress      int64
	MaxTeamSeats            int
	MaxContractLimit        int
	MaxRateLimit            int
	MaxPayloadBytes         int64
	HotStorageDays          int
	ColdStorageDays         int
	Support                 string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type PlanRepository interface {
	GetCompanyPlan(ctx context.Context, companyID string) (*CompanyPlan, error)
	GetCurrentUsageMeter(ctx context.Context, companyID string) (*UsageMeter, error)
	MarkPlanCancelled(ctx context.Context, companyID string) error
}
