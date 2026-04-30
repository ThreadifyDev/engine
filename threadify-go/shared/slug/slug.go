package slug

import (
	"regexp"
	"strings"
)

func ToSlug(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace non-alphanumeric characters with underscores
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "_")

	// Trim leading/trailing underscores
	return strings.Trim(s, "_")
}
