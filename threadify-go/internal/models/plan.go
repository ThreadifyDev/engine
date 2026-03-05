package models

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

type CompanyPlan struct {
	ID               string       `json:"id"`
	CompanyID        string       `json:"companyId"`
	SubscriptionTier PlanTier     `json:"subscriptionTier"`
	BillingCycle     BillingCycle `json:"billingCycle"`
	BillingStart     time.Time    `json:"billingStart"`
	BillingEnd       time.Time    `json:"billingEnd"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

type UsageMeter struct {
	ID                string    `json:"id"`
	CompanyID         string    `json:"companyId"`
	BillingCycleStart time.Time `json:"billingCycleStart"`

	BandwidthIngressBalance int64 `json:"bandwidthIngressBalance"`
	BandwidthEgressBalance  int64 `json:"bandwidthEgressBalance"`

	MaxBandwidthIngress int64     `json:"maxBandwidthIngress"`
	MaxBandwidthEgress  int64     `json:"maxBandwidthEgress"`
	MaxTeamSeats        int       `json:"maxTeamSeats"`
	MaxContractLimit    int       `json:"maxContractLimit"`
	MaxRateLimit        int       `json:"maxRateLimit"`
	MaxPayloadBytes     int64     `json:"maxPayloadBytes"`
	HotStorageDays      int       `json:"hotStorageDays"`
	ColdStorageDays     int       `json:"coldStorageDays"`
	Support             string    `json:"support"`
	BillingEnd          time.Time `json:"billingEnd"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p *CompanyPlan) IsExpired() bool {
	return time.Now().After(p.BillingEnd)
}
