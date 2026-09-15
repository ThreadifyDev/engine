package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"threadify-go/shared/registry"
	"time"

	"github.com/gorilla/websocket"
)

// Clients receive an explicit transport reason instead of a silent connection
// drop, including when application output is not allowed by the license.
func TestRegistryDenialSendsWebSocketCloseReason(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		code   int
		reason string
	}{
		{"allowance", registry.ErrLimit, websocket.ClosePolicyViolation, "registry_allowance_exceeded"},
		{"license", registry.ErrUnverified, websocket.ClosePolicyViolation, "license_unavailable"},
		{"storage", errors.New("private database error"), websocket.CloseTryAgainLater, "accounting_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				(&WSSession{conn: conn}).closeForRegistryError(test.err)
			}))
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := conn.ReadMessage()
			var closed *websocket.CloseError
			if !errors.As(err, &closed) || closed.Code != test.code || closed.Text != test.reason || len(payload) != 0 {
				t.Fatalf("close=%v payload=%q", err, payload)
			}
		})
	}
}
