package registry

import (
	"context"
	"strings"
	"testing"
)

// TestRegistryURLResolution preserves local overrides while making standard deployment configuration optional.
func TestRegistryURLResolution(t *testing.T) {
	for _, test := range []struct {
		name, configured, environment, want string
	}{
		{name: "omitted uses production", want: "https://registry.usefused.com"},
		{name: "environment override", environment: "http://127.0.0.1:62485", want: "http://127.0.0.1:62485"},
		{name: "configuration override", configured: "https://registry.example.test", want: "https://registry.example.test"},
		{name: "configuration precedes environment", configured: "https://config.example.test", environment: "https://env.example.test", want: "https://config.example.test"},
		{name: "invalid explicit configuration stays invalid", configured: "invalid-url", want: "invalid-url"},
		{name: "invalid environment stays invalid", environment: "invalid-url", want: "invalid-url"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("THREADIFY_REGISTRY_URL", test.environment)
			got := resolveConfig(Config{URL: test.configured, LicenseKey: "test-license"})
			// Exact resolution ensures testing overrides cannot silently become production requests.
			if got.URL != test.want || got.LicenseKey != "test-license" {
				t.Fatalf("resolved URL=%q license preserved=%v", got.URL, got.LicenseKey == "test-license")
			}
		})
	}
}

// TestRegistryURLDefaultRequiresLicense proves endpoint convenience does not bypass startup authentication.
func TestRegistryURLDefaultRequiresLicense(t *testing.T) {
	t.Setenv("THREADIFY_REGISTRY_URL", "")
	t.Setenv("THREADIFY_LICENSE_KEY", "")
	for _, endpoint := range []string{"", "http://127.0.0.1:62485"} {
		_, err := Start(context.Background(), Config{URL: endpoint}, nil)
		// Missing credentials must fail before database or network access even with an override.
		if err == nil || !strings.Contains(err.Error(), "license key") {
			t.Fatalf("URL=%q missing-license error=%v", endpoint, err)
		}
	}
	_, err := Start(context.Background(), Config{LicenseKey: "test-license"}, nil)
	// A supplied license with the default URL passes endpoint validation and still requires durable accounting.
	if err == nil || !strings.Contains(err.Error(), "requires PostgreSQL") {
		t.Fatalf("default endpoint validation error=%v", err)
	}
}
