package app

import (
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationAndAssetsOutsideSourceTree(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "custom.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  port: 8081\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "subscription.yaml"), []byte("subscription:\n  credit:\n    ingress_cost_millicents: 17\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	// The release container selects its mounted data volume without adding
	// broker settings to the user's configuration file.
	storeDir := filepath.Join(t.TempDir(), "mounted-jetstream")
	t.Setenv("NATS_STORE_DIR", storeDir)
	cfg, err := LoadConfigPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Subscription.Credit.IngressCostMillicents != 17 {
		t.Fatal("subscription not loaded beside selected config")
	}
	if cfg.NATS.StoreDir != storeDir {
		t.Fatalf("container storage override ignored: %q", cfg.NATS.StoreDir)
	}
	loader, err := loadRBAC()
	if err != nil || len(loader.GetAllRuntimeLevelRoles()) == 0 {
		t.Fatalf("embedded RBAC: %v", err)
	}
	if _, err := service.NewStepEventService(nil, nil, nil, cfg, zap.NewNop()); err != nil {
		t.Fatalf("embedded Lua: %v", err)
	}
}
