package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Database
	DatabaseURL string

	// JWT
	JWTSecret              string
	JWTExpiration          time.Duration
	RefreshTokenExpiration time.Duration

	// API Keys
	APIKeyTTL time.Duration

	// Plunk Email Service
	PlunkAPIKey    string
	PlunkFromEmail string

	// ThreadifyEngine URLs
	ThreadifyEngineURL        string
	ThreadifyEngineGraphQLURL string
	ThreadifyEngineUserID     string

	// Server
	Port        string
	CORSOrigins string
	FrontendURL string

	// Rate Limiting
	RateLimitRequests int
	RateLimitWindow   time.Duration
}

func Load() (*Config, error) {
	// Load .env file
	err := godotenv.Load() // ignore error, default to environment variables
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
		RefreshTokenExpiration:    refreshTokenExp, // 7 days
		APIKeyTTL:                 apiKeyTTL,       // 365 days
		PlunkAPIKey:               getEnv("PLUNK_API_KEY", ""),
		PlunkFromEmail:            getEnv("PLUNK_FROM_EMAIL", "noreply@threadify.com"),
		ThreadifyEngineURL:        getEnv("THREADIFY_ENGINE_URL", "http://localhost:8081"),
		ThreadifyEngineGraphQLURL: getEnv("THREADIFY_ENGINE_GRAPHQL_URL", "http://localhost:8081/graphql"),
		ThreadifyEngineUserID:     getEnv("THREADIFY_ENGINE_USER_ID", "123456"),
		Port:                      getEnv("PORT", "3001"),
		CORSOrigins:               getEnv("CORS_ORIGINS", "http://localhost:3000"),
		FrontendURL:               getEnv("FRONTEND_URL", "http://localhost:3000"),
		RateLimitRequests:         100,
		RateLimitWindow:           time.Minute,
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
