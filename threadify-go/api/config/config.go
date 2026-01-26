package config

import (
	"log"
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

	// Rate Limiting
	RateLimitRequests int
	RateLimitWindow   time.Duration
}

func Load() *Config {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		DatabaseURL:               getEnv("DATABASE_URL", "postgres://td_engine:tdtdtd@localhost:5434/threadify?sslmode=disable"),
		JWTSecret:                 getEnv("JWT_SECRET", "dev-secret-key"),
		JWTExpiration:             parseDuration(getEnv("JWT_EXPIRATION", "15m")),
		RefreshTokenExpiration:    parseDuration(getEnv("REFRESH_TOKEN_EXPIRATION", "168h")), // 7 days
		APIKeyTTL:                 parseDuration(getEnv("API_KEY_TTL", "8760h")),             // 365 days
		PlunkAPIKey:               getEnv("PLUNK_API_KEY", ""),
		PlunkFromEmail:            getEnv("PLUNK_FROM_EMAIL", "noreply@threadify.com"),
		ThreadifyEngineURL:        getEnv("THREADIFY_ENGINE_URL", "http://localhost:8081"),
		ThreadifyEngineGraphQLURL: getEnv("THREADIFY_ENGINE_GRAPHQL_URL", "http://localhost:8081/graphql"),
		ThreadifyEngineUserID:     getEnv("THREADIFY_ENGINE_USER_ID", "123456"),
		Port:                      getEnv("PORT", "3001"),
		CORSOrigins:               getEnv("CORS_ORIGINS", "http://localhost:3000"),
		RateLimitRequests:         100,
		RateLimitWindow:           time.Minute,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Fatalf("Invalid duration: %s", s)
	}
	return d
}
