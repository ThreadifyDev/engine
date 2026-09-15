// Package registryfixture supplies a signed Registry contract for binary tests.
package registryfixture

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"
)

const License = "integration-only-threadify-license"

type TestingT interface {
	Helper()
	Cleanup(func())
}

// New starts a local Registry double. It never contacts production or sends email.
func New(t TestingT, accountID string) *httptest.Server {
	t.Helper()
	snapshot := map[string]any{"status": "ok", "account_id": accountID, "workspace_id": accountID, "email": "owner@example.com", "heartbeat_interval_seconds": 60, "grace_period_seconds": 300, "suspended": false,
		"entitlements": map[string]any{"revision": "test-v1", "input_bandwidth_bytes": int64(-1), "output_bandwidth_bytes": int64(-1), "input_requests_per_second": int64(-1), "entity_profile_limit": int64(-1)}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+License || r.Header.Get("X-Threadify-Installation-ID") == "" {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
			if err != nil {
				http.Error(w, "body", 400)
				return
			}
			mac := hmac.New(sha256.New, []byte(License))
			mac.Write(body)
			if !hmac.Equal([]byte(r.Header.Get("X-Threadify-Signature")), []byte("sha256="+hex.EncodeToString(mac.Sum(nil)))) {
				http.Error(w, "signature", 403)
				return
			}
			var input struct {
				ReportedAt time.Time `json:"reported_at"`
			}
			if json.Unmarshal(body, &input) != nil || time.Since(input.ReportedAt) > 5*time.Minute {
				http.Error(w, "timestamp", 400)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/threadify/handshake", "/api/threadify/heartbeat":
			_ = json.NewEncoder(w).Encode(snapshot)
		case "/api/threadify/usage-reports":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "accepted": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
