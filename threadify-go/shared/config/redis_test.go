package config

import (
	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

func TestRedisURLAndLegacyFieldRejection(t *testing.T) {
	t.Setenv("SHARED_REDIS_TEST_URL", "rediss://worker:secret@host:6380/2")
	var cfg Config
	if err := yaml.Unmarshal([]byte("redis:\n  url: '$SHARED_REDIS_TEST_URL'\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.expandEnvVars()
	if cfg.Redis.URL != "rediss://worker:secret@host:6380/2" {
		t.Fatal("URL not expanded")
	}
	for _, field := range []string{"host", "port", "password", "db", "username"} {
		err := yaml.Unmarshal([]byte("redis:\n  url: redis://localhost/0\n  "+field+": obsolete\n"), &cfg)
		if err == nil || !strings.Contains(err.Error(), "no longer supported") {
			t.Fatalf("legacy %s accepted", field)
		}
	}
}
