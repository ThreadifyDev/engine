package config

import "testing"

func TestPublicURLConfigAndEnvironment(t *testing.T) {
	t.Setenv("THREADIFY_PUBLIC_URL", "https://env.example.test/engine/")
	for _, source := range []string{"{}", "server:\n  public_url: $THREADIFY_PUBLIC_URL\n"} {
		cfg, err := runtimeConfig(t, source)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Server.PublicURL != "https://env.example.test/engine" {
			t.Fatalf("wrong public URL: %q", cfg.Server.PublicURL)
		}
	}
	cfg, err := runtimeConfig(t, "server:\n  public_url: https://config.example.test\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.PublicURL != "https://config.example.test" {
		t.Fatal("explicit config should take precedence over the environment default")
	}
	if _, err = runtimeConfig(t, "server:\n  public_url: https://user:password@engine.test\n"); err == nil {
		t.Fatal("accepted embedded credentials")
	}
}
