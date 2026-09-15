package main

import "testing"

// Release inspection must not need a configuration file, database, or broker.
func TestVersionWithoutConfiguration(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir()+"/missing.yaml")
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("version command requires configuration: %v", err)
	}
}
