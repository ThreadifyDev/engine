package config

import (
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runtimeConfig(t *testing.T, source string) (*Config, error) {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(source)); err != nil {
		t.Fatal(err)
	}
	return LoadFromViper(v)
}
func TestCombinedRuntimeDefaults(t *testing.T) {
	cfg, err := runtimeConfig(t, "nats:\n  url: nats://old-host:4222\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NATS.Mode != "embedded" || cfg.RuntimeMode != "combined" || !cfg.Archiver.Enabled {
		t.Fatalf("wrong defaults: mode=%s broker=%s writer=%v", cfg.RuntimeMode, cfg.NATS.Mode, cfg.Archiver.Enabled)
	}
	if cfg.NATS.ArchivalMaxAgeHours != 0 || cfg.NATS.ArchivalMaxBytes <= 0 || cfg.Archiver.Streams.BlockTimeout <= 0 {
		t.Fatal("missing durable bounded defaults")
	}
}
func TestExternalWriterConfiguration(t *testing.T) {
	t.Setenv("TEST_NATS_URL", "nats://127.0.0.1:4222")
	cfg, err := runtimeConfig(t, "runtime_mode: writer\nnats:\n  mode: external\n  url: '$TEST_NATS_URL'\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NATS.URL != "nats://127.0.0.1:4222" {
		t.Fatal("URL not expanded")
	}
}
func TestInvalidRuntimeConfiguration(t *testing.T) {
	for _, source := range []string{
		"runtime_mode: writer\n", "runtime_mode: typo\n", "nats:\n  mode: external\n", "nats:\n  mode: typo\n",
		"nats:\n  store_dir: ''\n", "nats:\n  archival_max_bytes: -1\n", "archiver:\n  streams:\n    batch_size: 0\n",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := runtimeConfig(t, source); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
func TestPersistenceCanBeDisabledExplicitly(t *testing.T) {
	cfg, err := runtimeConfig(t, "archiver:\n  enabled: false\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Archiver.Enabled {
		t.Fatal("explicit false overridden")
	}
}

// A launch from another directory cannot silently select a different broker store.
func TestEmbeddedStorageFollowsInstalledBinary(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg, err := runtimeConfig(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(executable), "data", "jetstream")
	if cfg.NATS.StoreDir != want || cfg.NATS.StreamName != "NOTIFICATIONS" {
		t.Fatalf("storage=%q want=%q stream=%q", cfg.NATS.StoreDir, want, cfg.NATS.StreamName)
	}
}

// Explicit absolute paths preserve externally mounted or separately managed storage.
func TestEmbeddedStorageAbsoluteOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	cfg, err := runtimeConfig(t, "nats:\n  store_dir: '"+dir+"'\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NATS.StoreDir != dir {
		t.Fatalf("storage override changed: %q", cfg.NATS.StoreDir)
	}
}
