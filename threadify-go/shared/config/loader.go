package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"threadify-go/shared/registry"

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

type CreditConfig struct {
	IngressCostMillicents                   int64 `yaml:"ingress_cost_millicents" mapstructure:"ingress_cost_millicents" json:"ingress_cost_millicents"`
	EgressCostMillicents                    int64 `yaml:"egress_cost_millicents" mapstructure:"egress_cost_millicents" json:"egress_cost_millicents"`
	SeatCostMillicents                      int64 `yaml:"seat_cost_millicents" mapstructure:"seat_cost_millicents" json:"seat_cost_millicents"`
	ContractCostMillicents                  int64 `yaml:"contract_cost_millicents" mapstructure:"contract_cost_millicents" json:"contract_cost_millicents"`
	LLMTokenCostMillicents                  int64 `yaml:"llm_token_cost_millicents" mapstructure:"llm_token_cost_millicents" json:"llm_token_cost_millicents"`
	RateLimitTPS                            int64 `yaml:"rate_limit_tps" mapstructure:"rate_limit_tps" json:"rate_limit_tps"`
	PayloadLimitBytes                       int64 `yaml:"payload_limit_bytes" mapstructure:"payload_limit_bytes" json:"payload_limit_bytes"`
	CustomMetricCostPerComplexityMillicents int64 `yaml:"custom_metric_cost_per_complexity_millicents" mapstructure:"custom_metric_cost_per_complexity_millicents" json:"custom_metric_cost_per_complexity_millicents"`
}

type SubscriptionConfig struct {
	SignupCreditsMillicents int64        `yaml:"signup_credits_millicents" mapstructure:"signup_credits_millicents" json:"signup_credits_millicents"`
	Credit                  CreditConfig `yaml:"credit" mapstructure:"credit"`
}

type BillingConfig struct {
	Provider   string `yaml:"provider" mapstructure:"provider"`
	SuccessURL string `yaml:"success_url" mapstructure:"success_url"`
	CancelURL  string `yaml:"cancel_url" mapstructure:"cancel_url"`
}

type Config struct {
	Registry registry.Config `yaml:"registry"`
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
		Enabled                 bool   `yaml:"enabled"`
		Port                    int    `yaml:"port"`
		Host                    string `yaml:"host"`
		FrontendURL             string `yaml:"frontend_url"`
		OutboxEncryptionKey     string `yaml:"outbox_encryption_key"`
		SignupCreditsMillicents int64  `yaml:"signup_credits_millicents"`
		Email                   struct {
			Provider  string `yaml:"provider"`
			APIKey    string `yaml:"api_key"`
			FromEmail string `yaml:"from_email"`
			APIURL    string `yaml:"api_url"`
		} `yaml:"email"`
		ThreadifyEngine struct {
			URL        string `yaml:"url"`
			GraphQLURL string `yaml:"graphql_url"`
		} `yaml:"threadify_engine"`
		OpenAIAPIKey   string `yaml:"openai_api_key"`
		APIKeyTTLHours int    `yaml:"api_key_ttl_hours"`
		Agent          struct {
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
	c.Registry.URL = expand(c.Registry.URL)
	c.Registry.LicenseKey = expand(c.Registry.LicenseKey)
	c.Registry.InstallationID = expand(c.Registry.InstallationID)
	c.Registry.CompanyID = expand(c.Registry.CompanyID)
	c.Registry.BrowserOrigin = expand(c.Registry.BrowserOrigin)

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

	c.Billing.Provider = expand(c.Billing.Provider)

	c.WebAPI.FrontendURL = expand(c.WebAPI.FrontendURL)
	c.WebAPI.OutboxEncryptionKey = expand(c.WebAPI.OutboxEncryptionKey)
	c.WebAPI.OpenAIAPIKey = expand(c.WebAPI.OpenAIAPIKey)
	c.WebAPI.Email.Provider = expand(c.WebAPI.Email.Provider)
	c.WebAPI.Email.APIKey = expand(c.WebAPI.Email.APIKey)
	c.WebAPI.Email.FromEmail = expand(c.WebAPI.Email.FromEmail)
	c.WebAPI.Email.APIURL = expand(c.WebAPI.Email.APIURL)
	c.WebAPI.ThreadifyEngine.URL = expand(c.WebAPI.ThreadifyEngine.URL)
	c.WebAPI.ThreadifyEngine.GraphQLURL = expand(c.WebAPI.ThreadifyEngine.GraphQLURL)

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
