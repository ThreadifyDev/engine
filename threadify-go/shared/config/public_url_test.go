package config

import "testing"

func TestNormalizePublicURL(t *testing.T) {
	for input, want := range map[string]string{"": "", " https://Engine.Example.com/ ": "https://engine.example.com", "http://127.0.0.1:8083/": "http://127.0.0.1:8083", "https://example.com/threadify/": "https://example.com/threadify"} {
		got, err := NormalizePublicURL(input)
		if err != nil || got != want {
			t.Errorf("NormalizePublicURL(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"engine.example.com", "javascript:alert(1)", "ftp://engine.test", "https://user:secret@engine.test", "https://engine.test?token=key", "https://engine.test#part", "https://engine.test/a/../b", "https://engine.test/%2e%2e/b", "https://engine.test/%5cpath"} {
		if _, err := NormalizePublicURL(input); err == nil {
			t.Errorf("accepted invalid URL %q", input)
		}
	}
}
