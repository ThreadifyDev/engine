package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	JWT struct {
		Secret          string `yaml:"secret"`
		Issuer          string `yaml:"issuer"`
		Audience        string `yaml:"audience"`
		Realm           string `yaml:"realm"`
		ExpirationHours int    `yaml:"expiration_hours"`
	} `yaml:"jwt"`

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

	NATS struct {
		URL string `yaml:"url"`
	} `yaml:"nats"`

	WebAPI struct {
		Enabled     bool   `yaml:"enabled"`
		Port        int    `yaml:"port"`
		Host        string `yaml:"host"`
		CORSOrigins string `yaml:"cors_origins"`
		FrontendURL string `yaml:"frontend_url"`
		Email       struct {
			PlunkAPIKey    string `yaml:"plunk_api_key"`
			PlunkFromEmail string `yaml:"plunk_from_email"`
		} `yaml:"email"`
		ThreadifyEngine struct {
			URL        string `yaml:"url"`
			GraphQLURL string `yaml:"graphql_url"`
		} `yaml:"threadify_engine"`
		APIKeyTTLHours int `yaml:"api_key_ttl_hours"`
		RateLimit      struct {
			Requests      int `yaml:"requests"`
			WindowMinutes int `yaml:"window_minutes"`
		} `yaml:"rate_limit"`
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

	// Expand environment variables
	cfg.JWT.Secret = expandEnv(cfg.JWT.Secret)
	cfg.JWT.Issuer = expandEnv(cfg.JWT.Issuer)
	cfg.JWT.Audience = expandEnv(cfg.JWT.Audience)
	cfg.JWT.Realm = expandEnv(cfg.JWT.Realm)

	// Postgres config
	cfg.Postgres.URL = expandEnv(cfg.Postgres.URL)

	// Redis config
	cfg.Redis.Host = expandEnv(cfg.Redis.Host)
	cfg.Redis.Password = expandEnv(cfg.Redis.Password)

	// NATS config
	cfg.NATS.URL = expandEnv(cfg.NATS.URL)

	// WebAPI config
	cfg.WebAPI.FrontendURL = expandEnv(cfg.WebAPI.FrontendURL)
	cfg.WebAPI.Email.PlunkAPIKey = expandEnv(cfg.WebAPI.Email.PlunkAPIKey)
	cfg.WebAPI.Email.PlunkFromEmail = expandEnv(cfg.WebAPI.Email.PlunkFromEmail)
	cfg.WebAPI.ThreadifyEngine.URL = expandEnv(cfg.WebAPI.ThreadifyEngine.URL)
	cfg.WebAPI.ThreadifyEngine.GraphQLURL = expandEnv(cfg.WebAPI.ThreadifyEngine.GraphQLURL)

	return &cfg, nil
}

// expandEnv expands environment variables in format: $VAR:default
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
