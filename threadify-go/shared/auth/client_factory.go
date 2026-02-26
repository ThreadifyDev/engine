package auth

import (
	"errors"
	"fmt"
	"strings"

	sharedconfig "threadify-go/shared/config"
)

const (
	providerSupabase = "supabase"
)

type AuthProviderConfig struct {
	Provider string
	Supabase SupabaseAuthConfig
}

func NewAuthClientFromConfig(cfg AuthProviderConfig) (AuthClient, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		return nil, errors.New("auth provider must be specified")
	}

	switch provider {
	case providerSupabase:
		return NewSupabaseClient(cfg.Supabase)
	default:
		return nil, fmt.Errorf("unsupported auth provider %q", provider)
	}
}

func NewAuthClientFromSharedConfig(cfg *sharedconfig.Config) (AuthClient, error) {
	if cfg == nil {
		return nil, errors.New("nil config")
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.AuthProvider))
	if provider == "" {
		return nil, errors.New("auth provider must be specified")
	}

	switch provider {
	case providerSupabase:
		return NewSupabaseClient(SupabaseAuthConfig{
			URL:                   cfg.Supabase.URL,
			PublishableKey:        cfg.Supabase.PublishableKey,
			SecretKey:             cfg.Supabase.SecretKey,
			RequestTimeoutSeconds: cfg.Supabase.RequestTimeoutSeconds,
		})
	default:
		return nil, fmt.Errorf("unsupported auth provider %q", provider)
	}
}
