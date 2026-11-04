package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLicenseSettingsUseNewKeys(t *testing.T) {
	t.Setenv("TEST_THREADIFY_LICENSE", "lic_env")
	cfg, err := runtimeConfig(t, "license_key: $TEST_THREADIFY_LICENSE\nserver:\n  browser_origin: https://engine.example.test\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LicenseKey != "lic_env" || cfg.Registry.LicenseKey != "lic_env" {
		t.Fatalf("license not resolved: %q / %q", cfg.LicenseKey, cfg.Registry.LicenseKey)
	}
	if cfg.Server.BrowserOrigin != "https://engine.example.test" || cfg.Registry.BrowserOrigin != "https://engine.example.test" {
		t.Fatalf("browser origin not resolved: %q / %q", cfg.Server.BrowserOrigin, cfg.Registry.BrowserOrigin)
	}
}

func TestLicenseSettingsAcceptLegacyKeys(t *testing.T) {
	cfg, err := runtimeConfig(t, "registry:\n  license_key: lic_legacy\n  browser_origin: http://localhost:8081\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LicenseKey != "lic_legacy" || cfg.Registry.LicenseKey != "lic_legacy" {
		t.Fatalf("legacy license not resolved: %q / %q", cfg.LicenseKey, cfg.Registry.LicenseKey)
	}
	if cfg.Server.BrowserOrigin != "http://localhost:8081" || cfg.Registry.BrowserOrigin != "http://localhost:8081" {
		t.Fatalf("legacy browser origin not resolved: %q / %q", cfg.Server.BrowserOrigin, cfg.Registry.BrowserOrigin)
	}
}

func TestLicenseSettingsMatchingDuplicatesAllowed(t *testing.T) {
	if _, err := runtimeConfig(t, "license_key: lic_a\nregistry:\n  license_key: lic_a\n"); err != nil {
		t.Fatal(err)
	}
}

func TestLicenseSettingsRejectConflicts(t *testing.T) {
	for _, source := range []string{
		"license_key: lic_a\nregistry:\n  license_key: lic_b\n",
		"server:\n  browser_origin: https://a.example.test\nregistry:\n  browser_origin: https://b.example.test\n",
	} {
		_, err := runtimeConfig(t, source)
		if err == nil || !strings.Contains(err.Error(), "both set to different values") {
			t.Fatalf("expected conflict error for %q, got %v", source, err)
		}
	}
}

func TestShippedTemplatesUseLicenseKeys(t *testing.T) {
	t.Setenv("THREADIFY_LICENSE_KEY", "lic_template")
	t.Setenv("THREADIFY_BROWSER_ORIGIN", "https://engine.example.test")
	for _, name := range []string{"config.selfhost.yaml", "config.yaml", "config.docker.yaml", "config.local.yaml"} {
		source, err := os.ReadFile(filepath.Join("..", "..", "config", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "\nregistry:") || strings.HasPrefix(string(source), "registry:") {
			t.Fatalf("%s still uses the legacy registry section", name)
		}
		cfg, err := runtimeConfig(t, string(source))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cfg.Registry.BrowserOrigin != "https://engine.example.test" {
			t.Fatalf("%s: browser origin %q", name, cfg.Registry.BrowserOrigin)
		}
		if name == "config.selfhost.yaml" && cfg.Registry.LicenseKey != "lic_template" {
			t.Fatalf("%s: license %q", name, cfg.Registry.LicenseKey)
		}
	}
}
