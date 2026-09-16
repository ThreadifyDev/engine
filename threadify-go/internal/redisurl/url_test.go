package redisurl

import (
	"crypto/tls"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		raw, addr, user, password string
		db                        int
		tls                       bool
	}{
		{"redis://localhost", "localhost:6379", "", "", 0, false},
		{"redis://:p%40ss%3A%2F%23@localhost:6380/2", "localhost:6380", "", "p@ss:/#", 2, false},
		{"rediss://worker:secret@redis.example.com:6380/3", "redis.example.com:6380", "worker", "secret", 3, true},
		{"redis://default:secret@[::1]:6381/1", "[::1]:6381", "default", "secret", 1, false},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Addr != tc.addr || got.Username != tc.user || got.Password != tc.password || got.DB != tc.db {
				t.Fatal("incorrect connection options")
			}
			if (got.TLSConfig != nil) != tc.tls {
				t.Fatal("incorrect TLS selection")
			}
			if tc.tls && (got.TLSConfig.ServerName != "redis.example.com" || got.TLSConfig.InsecureSkipVerify || got.TLSConfig.MinVersion < tls.VersionTLS12) {
				t.Fatal("TLS verification not enabled")
			}
		})
	}
}
func TestInvalidURLsDoNotExposeCredentials(t *testing.T) {
	for _, raw := range []string{"", "localhost:6379", "http://private-password@host", "redis:///0", "redis://u:private-password@host:0", "redis://u:private-password@host:99999", "redis://u:private-password@host:notport", "redis://u:private-password@host/-1", "redis://u:private-password@host/a", "redis://u:private-password%zz@host/0", "redis://u:private-password@host/0?password=secret", "redis://u:private-password@host/0#fragment"} {
		_, err := Parse(raw)
		if err == nil {
			t.Fatalf("accepted invalid URL")
		}
		if strings.Contains(err.Error(), "private-password") {
			t.Fatal("credentials leaked")
		}
	}
}
func TestManagedRestrictions(t *testing.T) {
	for _, raw := range []string{"rediss://default:secret@localhost/0", "redis://worker:secret@localhost/0"} {
		if _, err := Managed(raw); err == nil {
			t.Fatal("accepted unsupported managed connection")
		}
	}
	if _, err := Managed("redis://default:secret@localhost/0"); err != nil {
		t.Fatal(err)
	}
}
