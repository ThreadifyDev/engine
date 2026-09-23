package main

import (
	"strings"
	"testing"
)

// Release inspection must not need a configuration file, database, or broker.
func TestVersionWithoutConfiguration(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir()+"/missing.yaml")
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("version command requires configuration: %v", err)
	}
}

func TestAgentConflictFailsBeforeConnectingToEngineServices(t *testing.T) {
	t.Setenv("THREADIFY_AGENT_URL", "http://127.0.0.1:8090")
	err := run([]string{"--config", "../../config/config.selfhost.yaml", "--with-agent"})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected activation conflict before service startup, got %v", err)
	}
}

func TestAgentRuntimeInstallDoesNotLoadEngineConfiguration(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir()+"/missing.yaml")
	err := run([]string{"agent", "install-runtime", "--agent-cache-dir", t.TempDir(), "--agent-runtime-archive", t.TempDir() + "/missing.tar.gz"})
	if err == nil || !strings.Contains(err.Error(), "install agent runtime") || strings.Contains(err.Error(), "read config") {
		t.Fatalf("runtime provisioning must not initialize the Engine, got %v", err)
	}
}

func TestAgentArchiveRequiresExplicitActivation(t *testing.T) {
	err := run([]string{"--agent-runtime-archive", "/missing.tar.gz"})
	if err == nil || !strings.Contains(err.Error(), "requires --with-agent") {
		t.Fatalf("expected explicit activation requirement, got %v", err)
	}
}
