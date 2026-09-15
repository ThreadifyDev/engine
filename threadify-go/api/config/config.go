package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string

	JWTSecret              string
	JWTExpiration          time.Duration
	RefreshTokenExpiration time.Duration

	APIKeyTTL time.Duration

	EmailAPIKey    string
	EmailFromEmail string

	ThreadifyEngineURL        string
	ThreadifyEngineGraphQLURL string
	ThreadifyEngineUserID     string

	Port        string
	FrontendURL string
}

func Load() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("load .env file: %w", err)
	}

	jwtExp, err := parseDuration(getEnv("JWT_EXPIRATION", "15m"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRATION: %w", err)
	}
	refreshTokenExp, err := parseDuration(getEnv("REFRESH_TOKEN_EXPIRATION", "168h"))
	if err != nil {
		return nil, fmt.Errorf("invalid REFRESH_TOKEN_EXPIRATION: %w", err)
	}
	apiKeyTTL, err := parseDuration(getEnv("API_KEY_TTL", "8760h"))
	if err != nil {
		return nil, fmt.Errorf("invalid API_KEY_TTL: %w", err)
	}

	return &Config{
		DatabaseURL:               getEnv("DATABASE_URL", "postgres://td_engine:tdtdtd@localhost:5434/threadify?sslmode=disable"),
		JWTSecret:                 getEnv("JWT_SECRET", "dev-secret-key"),
		JWTExpiration:             jwtExp,
		RefreshTokenExpiration:    refreshTokenExp,
		APIKeyTTL:                 apiKeyTTL,
		EmailAPIKey:               getEnv("EMAIL_API_KEY", ""),
		EmailFromEmail:            getEnv("EMAIL_FROM_EMAIL", "noreply@threadify.dev"),
		ThreadifyEngineURL:        getEnv("THREADIFY_ENGINE_URL", "http://localhost:8081"),
		ThreadifyEngineGraphQLURL: getEnv("THREADIFY_ENGINE_GRAPHQL_URL", "http://localhost:8081/graphql"),
		ThreadifyEngineUserID:     getEnv("THREADIFY_ENGINE_USER_ID", "123456"),
		Port:                      getEnv("PORT", "3001"),
		FrontendURL:               getEnv("FRONTEND_URL", "http://localhost:3000"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %s", s)
	}
	return d, nil
}
