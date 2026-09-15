package config

import (
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"runtime"
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

func TestValkeyModeAndPaths(t *testing.T) {
	cfg, err := runtimeConfig(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	wantMode := "managed"
	if runtime.GOOS == "windows" {
		wantMode = "external"
	}
	if cfg.Redis.Mode != wantMode {
		t.Fatalf("mode=%s", cfg.Redis.Mode)
	}
	if wantMode == "managed" {
		executable, _ := os.Executable()
		executable, _ = filepath.EvalSymlinks(executable)
		if cfg.Redis.StoreDir != filepath.Join(filepath.Dir(executable), "data", "valkey") || cfg.Redis.BinaryPath != filepath.Join(filepath.Dir(executable), "libexec", "valkey-server") {
			t.Fatalf("paths: %+v", cfg.Redis)
		}
	}
	legacy, err := runtimeConfig(t, "redis:\n  host: old-valkey\n  port: 6380\n")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Redis.Mode != "external" || legacy.Redis.Host != "old-valkey" {
		t.Fatal("legacy endpoint changed")
	}
	explicit, err := runtimeConfig(t, "redis:\n  mode: managed\n  host: 127.0.0.1\n  port: 6380\n")
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Redis.Mode != "managed" || explicit.Redis.Port != 6380 {
		t.Fatal("explicit ownership ignored")
	}
}

func TestReloadDoesNotChangeValkeyOwnership(t *testing.T) {
	v := viper.New()
	first, err := LoadFromViper(v)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadFromViper(v)
	if err != nil {
		t.Fatal(err)
	}
	if first.Redis.Mode != second.Redis.Mode {
		t.Fatal("loading defaults changed ownership")
	}
}

func TestInvalidValkeyConfig(t *testing.T) {
	for _, source := range []string{"redis:\n  mode: typo", "redis:\n  port: 0", "redis:\n  db: -1", "redis:\n  mode: managed\n  store_dir: ''", "redis:\n  mode: managed\n  startup_timeout_seconds: 0"} {
		if _, err := runtimeConfig(t, source); err == nil {
			t.Fatalf("accepted %s", source)
		}
	}
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
