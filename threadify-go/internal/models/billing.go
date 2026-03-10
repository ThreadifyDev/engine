package models

import "time"

type SnapshotReason string

const (
	SnapshotReasonOverageOnly    SnapshotReason = "overage_only"    // yearly mid-cycle
	SnapshotReasonMonthlyRenewal SnapshotReason = "monthly_renewal" // monthly plan: sub + overage
	SnapshotReasonYearlyRenewal  SnapshotReason = "yearly_renewal"  // end of 12-month period
)

type PaymentStatus string

const (
	PaymentStatusNoCharge PaymentStatus = "no_charge" // snapshot had zero overage, nothing to collect
	PaymentStatusPending  PaymentStatus = "pending"   // invoice issued, awaiting webhook confirmation
	PaymentStatusPaid     PaymentStatus = "paid"      // confirmed via invoice.paid webhook
	PaymentStatusFailed   PaymentStatus = "failed"    // confirmed via invoice.payment_failed webhook
)

type BillingSnapshot struct {
	ID                      string            `json:"id"`
	CompanyID               string            `json:"companyId"`
	Tier                    PlanTier          `json:"tier"`
	Reason                  SnapshotReason    `json:"reason"`
	PeriodStart             time.Time         `json:"periodStart"`
	PeriodEnd               time.Time         `json:"periodEnd"`
	IsCycleEnd              bool              `json:"isCycleEnd"` // true = full billing cycle end (triggers reset)
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

type InvoiceLineItem struct {
	Meter       string `json:"meter"`       // "bandwidth_ingress", "bandwidth_egress"
	OverageQty  int64  `json:"overageQty"`  // absolute overage amount
	UnitLabel   string `json:"unitLabel"`   // "per 1M requests", "per GB"
	RateCents   int    `json:"rateCents"`   // price per unit in cents
	AmountCents int64  `json:"amountCents"` // total charge for this line
}

func (s *BillingSnapshot) HasOverage() bool {
	return s.IngressBalanceFinal < 0 || s.EgressBalanceFinal < 0
}
