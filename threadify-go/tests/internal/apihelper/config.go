package apihelper

import (
	"fmt"
	"os"
	"path/filepath"
)

type APIServerConfig struct {
	PostgresURL string
	NATSURL     string
	Port        int

	SupabaseURL string
	PubKey      string
	SecretKey   string

	PlunkAPIURL string
	PlunkAPIKey string

	OutboxEncryptionKey string
}

func WriteConfig(dir string, cfg APIServerConfig) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("dir is required")
	}
	if cfg.PostgresURL == "" || cfg.NATSURL == "" || cfg.SupabaseURL == "" || cfg.PlunkAPIURL == "" {
		return "", fmt.Errorf("missing required config fields")
	}
	if cfg.Port <= 0 {
		return "", fmt.Errorf("port is required")
	}

	if cfg.PubKey == "" {
		cfg.PubKey = "pk_test"
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "sk_test"
	}
	if cfg.PlunkAPIKey == "" {
		cfg.PlunkAPIKey = "plunk_test"
	}
	if cfg.OutboxEncryptionKey == "" {
		cfg.OutboxEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	}

	path := filepath.Join(dir, "config.yaml")
	contents := fmt.Sprintf(`
postgres:
  url: %q
nats:
  url: %q
jwks:
  url: %q
web_api:
  port: %d
  frontend_url: "http://localhost"
  cors_origins: "*"
  outbox_encryption_key: %q
  email:
    plunk_api_key: %q
    plunk_api_url: %q
auth_provider: "supabase"
supabase:
  url: %q
  publishable_key: %q
  secret_key: %q
`, cfg.PostgresURL, cfg.NATSURL, cfg.SupabaseURL+"/auth/v1/.well-known/jwks.json", cfg.Port, cfg.OutboxEncryptionKey, cfg.PlunkAPIKey, cfg.PlunkAPIURL, cfg.SupabaseURL, cfg.PubKey, cfg.SecretKey)

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	return path, nil
}
