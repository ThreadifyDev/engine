package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadSource(t *testing.T, source string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestSharedLicenseSettings(t *testing.T) {
	cfg, err := loadSource(t, "license_key: lic_new\nserver:\n  browser_origin: https://engine.example.test\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Registry.LicenseKey != "lic_new" || cfg.Registry.BrowserOrigin != "https://engine.example.test" {
		t.Fatalf("new keys not resolved: %+v", cfg.Registry)
	}
	cfg, err = loadSource(t, "registry:\n  license_key: lic_old\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LicenseKey != "lic_old" {
		t.Fatalf("legacy key not resolved: %q", cfg.LicenseKey)
	}
	if _, err = loadSource(t, "license_key: a\nregistry:\n  license_key: b\n"); err == nil {
		t.Fatal("accepted conflicting license keys")
	}
}
