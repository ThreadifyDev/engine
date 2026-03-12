package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SupabaseSettings holds Supabase project credentials.
type SupabaseSettings struct {
	URL                   string `yaml:"url"`
	PublishableKey        string `yaml:"publishable_key"`
	SecretKey             string `yaml:"secret_key"`
	RequestTimeoutSeconds int    `yaml:"request_timeout_seconds"`
}

// JWKSSettings holds the JWKS endpoint configuration used by the engine to
// validate bearer tokens issued by any OIDC-compliant auth provider.
type JWKSSettings struct {
	URL      string `yaml:"url"`
	Audience string `yaml:"audience"`
	Issuer   string `yaml:"issuer"`
}

type NATSConfig struct {
	URL            string `yaml:"url"`
	ClusterID      string `yaml:"cluster_id"`
	ClientID       string `yaml:"client_id"`
	StreamName     string `yaml:"stream_name"`
	AckWaitSeconds int    `yaml:"ack_wait_seconds"`
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

type SubscriptionConfig struct {
	Tiers map[string]TierLimits `yaml:"tiers" mapstructure:"tiers"`
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

type TierPrice struct {
	MonthlyPriceID string `yaml:"monthly_price_id" mapstructure:"monthly_price_id"`
	YearlyPriceID  string `yaml:"yearly_price_id" mapstructure:"yearly_price_id"`
}

type BillingConfig struct {
	SecretKey     string               `yaml:"secret_key" mapstructure:"secret_key"`
	Provider      string               `yaml:"provider" mapstructure:"provider"`
	TierPrices    map[string]TierPrice `yaml:"tier_prices" mapstructure:"tier_prices"`
	WebhookSecret string               `yaml:"webhook_secret" mapstructure:"webhook_secret"`
	SuccessURL    string               `yaml:"success_url" mapstructure:"success_url"`
	CancelURL     string               `yaml:"cancel_url" mapstructure:"cancel_url"`
}

func (b *BillingConfig) GetProviderParams(tier, billingCycle string) (map[string]string, error) {
	tp, ok := b.TierPrices[tier]
	if !ok {
		return nil, fmt.Errorf("no price configured for tier %q", tier)
	}

	priceIDs := map[string]string{
		"monthly": tp.MonthlyPriceID,
		"yearly":  tp.YearlyPriceID,
	}

	priceID, ok := priceIDs[billingCycle]
	if !ok {
		return nil, fmt.Errorf("invalid billing cycle: %q", billingCycle)
	}
	if priceID == "" {
		return nil, fmt.Errorf("no price ID configured for tier %q and cycle %q", tier, billingCycle)
	}

	return map[string]string{"price_id": priceID}, nil
}

type Config struct {
	Postgres struct {
		URL                string `yaml:"url"`
		Host               string `yaml:"host"`
		Port               int    `yaml:"port"`
		User               string `yaml:"user"`
		Password           string `yaml:"password"`
		Name               string `yaml:"name"`
		MaxConnections     int    `yaml:"max_connections"`
		MaxIdleConnections int    `yaml:"max_idle_connections"`
	} `yaml:"postgres"`

	Server struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	} `yaml:"server"`

	Redis struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`

	AuthProvider string             `yaml:"auth_provider"`
	Supabase     SupabaseSettings   `yaml:"supabase"`
	JWKS         JWKSSettings       `yaml:"jwks"`
	NATS         NATSConfig         `yaml:"nats"`
	Billing      BillingConfig      `yaml:"billing"`
	Subscription SubscriptionConfig `yaml:"subscription"`

	WebAPI struct {
		Enabled             bool   `yaml:"enabled"`
		Port                int    `yaml:"port"`
		Host                string `yaml:"host"`
		CORSOrigins         string `yaml:"cors_origins"`
		FrontendURL         string `yaml:"frontend_url"`
		OutboxEncryptionKey string `yaml:"outbox_encryption_key"`
		Email               struct {
			PlunkAPIKey    string `yaml:"plunk_api_key"`
			PlunkFromEmail string `yaml:"plunk_from_email"`
			PlunkAPIURL    string `yaml:"plunk_api_url"`
		} `yaml:"email"`
		ThreadifyEngine struct {
			URL        string `yaml:"url"`
			GraphQLURL string `yaml:"graphql_url"`
		} `yaml:"threadify_engine"`
		OpenAIAPIKey   string `yaml:"openai_api_key"`
		APIKeyTTLHours int    `yaml:"api_key_ttl_hours"`
		RateLimit      struct {
			Requests      int `yaml:"requests"`
			WindowMinutes int `yaml:"window_minutes"`
		} `yaml:"rate_limit"`
		Agent struct {
			MaxMessages      int `yaml:"max_messages"`
			MaxTokens        int `yaml:"max_tokens"`
			SummaryMaxTokens int `yaml:"summary_max_tokens"`
		} `yaml:"agent"`
	} `yaml:"web_api"`
}

func Load(path string) (*Config, error) {
	cfg, err := parseYAML[Config](path)
	if err != nil {
		return nil, err
	}

	subPath := filepath.Join(filepath.Dir(path), "subscription.yaml")
	if _, err := os.Stat(subPath); err == nil {
		if err := mergeYAML(subPath, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse subscription config: %w", err)
		}
	}

	cfg.expandEnvVars()
	return cfg, nil
}

func (c *Config) expandEnvVars() {
	expand := expandEnv

	c.Postgres.URL = expand(c.Postgres.URL)

	c.Redis.Host = expand(c.Redis.Host)
	c.Redis.Password = expand(c.Redis.Password)

	c.NATS.URL = expand(c.NATS.URL)
	c.NATS.ClusterID = expand(c.NATS.ClusterID)
	c.NATS.ClientID = expand(c.NATS.ClientID)
	c.NATS.StreamName = expand(c.NATS.StreamName)

	c.Supabase.URL = expand(c.Supabase.URL)
	c.Supabase.PublishableKey = expand(c.Supabase.PublishableKey)
	c.Supabase.SecretKey = expand(c.Supabase.SecretKey)

	c.JWKS.URL = expand(c.JWKS.URL)
	c.JWKS.Audience = expand(c.JWKS.Audience)
	c.JWKS.Issuer = expand(c.JWKS.Issuer)

	c.Billing.SecretKey = expand(c.Billing.SecretKey)
	c.Billing.WebhookSecret = expand(c.Billing.WebhookSecret)
	c.Billing.SuccessURL = expand(c.Billing.SuccessURL)
	c.Billing.CancelURL = expand(c.Billing.CancelURL)
	c.Billing.Provider = expand(c.Billing.Provider)
	for name, tp := range c.Billing.TierPrices {
		tp.MonthlyPriceID = expand(tp.MonthlyPriceID)
		tp.YearlyPriceID = expand(tp.YearlyPriceID)
		c.Billing.TierPrices[name] = tp
	}

	c.WebAPI.FrontendURL = expand(c.WebAPI.FrontendURL)
	c.WebAPI.OutboxEncryptionKey = expand(c.WebAPI.OutboxEncryptionKey)
	c.WebAPI.OpenAIAPIKey = expand(c.WebAPI.OpenAIAPIKey)
	c.WebAPI.Email.PlunkAPIKey = expand(c.WebAPI.Email.PlunkAPIKey)
	c.WebAPI.Email.PlunkFromEmail = expand(c.WebAPI.Email.PlunkFromEmail)
	c.WebAPI.Email.PlunkAPIURL = expand(c.WebAPI.Email.PlunkAPIURL)
	c.WebAPI.ThreadifyEngine.URL = expand(c.WebAPI.ThreadifyEngine.URL)
	c.WebAPI.ThreadifyEngine.GraphQLURL = expand(c.WebAPI.ThreadifyEngine.GraphQLURL)

	for name, tier := range c.Subscription.Tiers {
		tier.Support = expand(tier.Support)
		c.Subscription.Tiers[name] = tier
	}
}

func parseYAML[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}
	return &v, nil
}

func mergeYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", path, err)
	}
	return yaml.Unmarshal(data, v)
}

func expandEnv(value string) string {
	if !strings.HasPrefix(value, "$") {
		return value
	}
	name, def, _ := strings.Cut(value[1:], ":")
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
