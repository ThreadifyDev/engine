package slug

import (
	"regexp"
	"strings"
)

// ToSlug converts a string to a URL-friendly slug format
// Example: "Customer Support Agent" -> "customer_support_agent"
func ToSlug(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	
	// Replace non-alphanumeric characters with underscores
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "_")
	
	// Trim leading/trailing underscores
	return strings.Trim(s, "_")
}
