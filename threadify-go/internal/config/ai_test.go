package config

import "testing"

func TestAIGatewayYAML(t *testing.T) {
	cfg, err := runtimeConfig(t, `ai:
  enabled: false
  agent:
    url: https://agent.test
  gateway:
    auth: custom
    base_url: https://gateway.test/v1
    model: org/model
    api_key_env: LOCAL_AI_KEY
    tls:
      ca_file: certs/ca.pem
      cert_file: certs/client.pem
      key_file: certs/client-key.pem
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI == nil || cfg.AI.Enabled == nil || *cfg.AI.Enabled {
		t.Fatal("AI disable flag was lost")
	}
	if cfg.AI.Agent.URL != "https://agent.test" {
		t.Fatal("external agent URL was lost")
	}
	if cfg.AI.Gateway.Auth != "custom" || cfg.AI.Gateway.Model != "org/model" || cfg.AI.Gateway.APIKeyEnv != "LOCAL_AI_KEY" || cfg.AI.Gateway.TLS.KeyFile != "certs/client-key.pem" {
		t.Fatal("gateway config was lost")
	}
}
