package config

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizePublicURL accepts an HTTP(S) deployment base, including a reverse-proxy path prefix.
func NormalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 {
		return "", fmt.Errorf("public URL must be an absolute HTTP or HTTPS URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return "", fmt.Errorf("public URL must use HTTP or HTTPS without credentials, a query or a fragment")
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." || strings.ContainsAny(segment, "\\\x00\r\n") {
			return "", fmt.Errorf("public URL contains an invalid path")
		}
	}
	u.Host = strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/"), nil
}
