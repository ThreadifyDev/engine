package config

type SubscriptionConfig struct {
	Tiers map[string]TierLimits `yaml:"tiers" mapstructure:"tiers"`
}

type BillingConfig struct {
	Provider      string                       `yaml:"provider" mapstructure:"provider"`
	Providers     map[string]map[string]string `yaml:"providers" mapstructure:"providers"`
	WebhookSecret string                       `yaml:"webhook_secret" mapstructure:"webhook_secret"`
}

type TierLimits struct {
	BandwidthIngress                       int64  `yaml:"bandwidth_ingress" mapstructure:"bandwidth_ingress"`
	BandwidthIngressHardCap                int64  `yaml:"bandwidth_ingress_hard_cap" mapstructure:"bandwidth_ingress_hard_cap"`
	BandwidthIngressOverageCentsPerMillion int    `yaml:"bandwidth_ingress_overage_cents_per_million" mapstructure:"bandwidth_ingress_overage_cents_per_million"`
	BandwidthEgress                        int64  `yaml:"bandwidth_egress" mapstructure:"bandwidth_egress"`
	BandwidthEgressOverageCentsPerGB       int    `yaml:"bandwidth_egress_overage_cents_per_gb" mapstructure:"bandwidth_egress_overage_cents_per_gb"`
	TeamSeats                              int    `yaml:"team_seats" mapstructure:"team_seats"`
	SeatOverageCentsPerMonth               int    `yaml:"seat_overage_cents_per_month" mapstructure:"seat_overage_cents_per_month"`
	ContractLimit                          int    `yaml:"contract_limit" mapstructure:"contract_limit"`
	RateLimit                              int    `yaml:"rate_limit" mapstructure:"rate_limit"`
	MaxPayloadBytes                        int64  `yaml:"max_payload_bytes" mapstructure:"max_payload_bytes"`
	HotStorageDays                         int    `yaml:"hot_storage_days" mapstructure:"hot_storage_days"`
	ColdStorageDays                        int    `yaml:"cold_storage_days" mapstructure:"cold_storage_days"`
	ColdStorageOverageCentsPerGB           int    `yaml:"cold_storage_overage_cents_per_gb" mapstructure:"cold_storage_overage_cents_per_gb"`
	Support                                string `yaml:"support" mapstructure:"support"`
	OverageAllowed                         bool   `yaml:"overage_allowed" mapstructure:"overage_allowed"`
}

func (s *SubscriptionConfig) GetTierLimits(tierName string) *TierLimits {
	if s == nil || s.Tiers == nil {
		return nil
	}
	limits, ok := s.Tiers[tierName]
	if !ok {
		return nil
	}
	return &limits
}
