package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	APIKeyPrefix = "td_"
	APIKeyLength = 32 // bytes, will be 43 chars in base64
)

// GenerateAPIKey generates a new API key with format: td_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, APIKeyLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64 and remove padding
	key := base64.URLEncoding.EncodeToString(bytes)
	key = strings.TrimRight(key, "=")

	return APIKeyPrefix + key, nil
}

// HashAPIKey creates a SHA-256 hash of the API key for storage
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash)
}

// GetKeyPrefix extracts the first 10 characters for display (e.g., "td_abc1234")
func GetKeyPrefix(key string) string {
	if len(key) < 10 {
		return key
	}
	return key[:10]
}
