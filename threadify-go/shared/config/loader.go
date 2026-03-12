package config

import (
	"fmt"
	"os"
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

type BillingConfig struct {
	SecretKey     string               `yaml:"secret_key" mapstructure:"secret_key"`
	Provider      string               `yaml:"provider" mapstructure:"provider"`
	TierPrices    map[string]TierPrice `yaml:"tier_prices"    mapstructure:"tier_prices"`
	WebhookSecret string               `yaml:"webhook_secret" mapstructure:"webhook_secret"`
	SuccessURL    string               `yaml:"success_url"`
	CancelURL     string               `yaml:"cancel_url"`
}

type TierPrice struct {
	PriceID string `yaml:"price_id" mapstructure:"price_id"`
}

func (b *BillingConfig) GetProviderParams(tier string) (map[string]interface{}, error) {
	tp, ok := b.TierPrices[tier]
	if !ok || tp.PriceID == "" {
		return nil, fmt.Errorf("no price configured for tier %q", tier)
	}
	return map[string]interface{}{
		"price_id": tp.PriceID,
	}, nil
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

// Load reads and parses config.yaml with environment variable expansion
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Postgres config
	cfg.Postgres.URL = expandEnv(cfg.Postgres.URL)

	// Redis config
	cfg.Redis.Host = expandEnv(cfg.Redis.Host)
	cfg.Redis.Password = expandEnv(cfg.Redis.Password)

	// NATS config
	cfg.NATS.URL = expandEnv(cfg.NATS.URL)
	cfg.NATS.ClusterID = expandEnv(cfg.NATS.ClusterID)
	cfg.NATS.ClientID = expandEnv(cfg.NATS.ClientID)
	cfg.NATS.StreamName = expandEnv(cfg.NATS.StreamName)

	// WebAPI config
	cfg.WebAPI.FrontendURL = expandEnv(cfg.WebAPI.FrontendURL)
	cfg.WebAPI.OutboxEncryptionKey = expandEnv(cfg.WebAPI.OutboxEncryptionKey)
	cfg.WebAPI.Email.PlunkAPIKey = expandEnv(cfg.WebAPI.Email.PlunkAPIKey)
	cfg.WebAPI.Email.PlunkFromEmail = expandEnv(cfg.WebAPI.Email.PlunkFromEmail)
	cfg.WebAPI.Email.PlunkAPIURL = expandEnv(cfg.WebAPI.Email.PlunkAPIURL)
	cfg.WebAPI.ThreadifyEngine.URL = expandEnv(cfg.WebAPI.ThreadifyEngine.URL)
	cfg.WebAPI.ThreadifyEngine.GraphQLURL = expandEnv(cfg.WebAPI.ThreadifyEngine.GraphQLURL)
	cfg.WebAPI.OpenAIAPIKey = expandEnv(cfg.WebAPI.OpenAIAPIKey)

	// Supabase config
	cfg.Supabase.URL = expandEnv(cfg.Supabase.URL)
	cfg.Supabase.PublishableKey = expandEnv(cfg.Supabase.PublishableKey)
	cfg.Supabase.SecretKey = expandEnv(cfg.Supabase.SecretKey)

	// JWKS config
	cfg.JWKS.URL = expandEnv(cfg.JWKS.URL)
	cfg.JWKS.Audience = expandEnv(cfg.JWKS.Audience)
	cfg.JWKS.Issuer = expandEnv(cfg.JWKS.Issuer)

	return &cfg, nil
}

func expandEnv(value string) string {
	if !strings.HasPrefix(value, "$") {
		return value
	}

	parts := strings.SplitN(value[1:], ":", 2)
	envVar := parts[0]
	defaultVal := ""
	if len(parts) == 2 {
		defaultVal = parts[1]
	}

	if val := os.Getenv(envVar); val != "" {
		return val
	}
	return defaultVal
}
