package utils

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// GenerateContextHash creates a deterministic hash from context fields
// This enables automatic retry tracking without explicit idempotency keys
func GenerateContextHash(context map[string]string) string {
	if len(context) == 0 {
		return ""
	}

	// Sort keys for deterministic ordering
	var keys []string
	for k := range context {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build ordered string
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, context[k]))
	}
	orderedString := strings.Join(parts, "&")

	// Generate SHA-256 hash
	hash := sha256.Sum256([]byte(orderedString))
	return fmt.Sprintf("sha256:%x", hash)
}

// GenerateStepHash creates a hash for step identification
func GenerateStepHash(threadID, stepName, idempotencyKey string) string {
	data := fmt.Sprintf("%s:%s:%s", threadID, stepName, idempotencyKey)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("sha256:%x", hash)
}
