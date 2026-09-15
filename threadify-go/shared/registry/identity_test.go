package registry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestIdentityProtocol uses the Registry's nested error envelope and verifies product-local authentication headers.
func TestIdentityProtocol(t *testing.T) {
	pending := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer license" || r.Header.Get("X-Threadify-Installation-ID") != "installation" {
			t.Error("missing product identity")
		}
		switch r.URL.Path {
		case "/api/threadify/identity/transactions":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["purpose"] != "browser_login" || in["engine_verifier"] != "verifier" || in["enrollment_ref"] != "local" {
				t.Error("incorrect start payload")
			}
			_ = json.NewEncoder(w).Encode(IdentityTransaction{TransactionID: "remote", VerificationURL: "https://registry.example.test/login", ExpiresAt: time.Now().Add(time.Minute)})
		case "/api/threadify/identity/transactions/remote/exchange":
			if pending {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"error":{"code":"transaction_unavailable","message":"transaction_unavailable"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(IdentityAssertion{TransactionID: "remote"})
		case "/api/threadify/identity/logout":
			_ = json.NewEncoder(w).Encode(map[string]string{"logout_url": "https://identity.example.test/logout"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	runtime := &Runtime{cfg: Config{URL: server.URL, LicenseKey: "license", InstallationID: "installation"}, snapshot: testSnapshot(), verified: time.Now(), client: server.Client()}
	tx, err := runtime.StartIdentity(context.Background(), "verifier", "local")
	if err != nil || tx.TransactionID != "remote" {
		t.Fatalf("start failed: %v", err)
	}
	_, err = runtime.ExchangeIdentity(context.Background(), "remote", "verifier")
	if !errors.Is(err, ErrIdentityPending) {
		t.Fatalf("pending classification: %v", err)
	}
	pending = false
	a, err := runtime.ExchangeIdentity(context.Background(), "remote", "verifier")
	if err != nil || a.TransactionID != "remote" {
		t.Fatalf("exchange failed: %v", err)
	}
	url, err := runtime.LogoutIdentity(context.Background(), "logout", "https://engine.example.test/login")
	if err != nil || url == "" {
		t.Fatalf("logout failed: %v", err)
	}
	_, err = runtime.ExchangeIdentity(context.Background(), "unknown", "verifier")
	if !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("missing route confused with pending: %v", err)
	}
}
