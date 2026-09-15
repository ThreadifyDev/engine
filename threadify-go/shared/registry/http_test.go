package registry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A handler's optimistic 200 must not mask a quota denial before any body is sent.
func TestMeteredResponseDefersSuccessUntilAdmission(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Length", "100")
	rec.Header().Set("Content-Encoding", "gzip")
	writer := &meteredWriter{ResponseWriter: rec, ctx: context.Background(), inputError: ErrLimit}
	writer.WriteHeader(http.StatusOK)
	n, err := writer.Write([]byte("must not leak"))
	if err != ErrLimit || n != 0 || rec.Code != http.StatusTooManyRequests || rec.Body.Len() != 0 {
		t.Fatalf("denied response: n=%d err=%v code=%d body=%q", n, err, rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "" || rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("denial retained stale entity headers")
	}
}

// Body-read verification errors must override generic JSON parser 400 responses.
func TestMeteredInputErrorReachesResponseBoundary(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := &Runtime{snapshot: Snapshot{Suspended: true}}
	writer := &meteredWriter{ResponseWriter: rec, ctx: context.Background()}
	reader := &meteredReader{ReadCloser: io.NopCloser(strings.NewReader("{}")), r: runtime, ctx: context.Background(), onError: writer.setInputError}
	payload, err := io.ReadAll(reader)
	if err != ErrUnverified || len(payload) != 0 {
		t.Fatalf("body exposed despite suspended license: %q %v", payload, err)
	}
	writer.WriteHeader(http.StatusBadRequest)
	_, _ = writer.Write([]byte("invalid JSON"))
	if rec.Code != http.StatusServiceUnavailable || rec.Body.Len() != 0 {
		t.Fatalf("parser masked license denial: %d %s", rec.Code, rec.Body.String())
	}
}

// An allowed explicit status survives deferred headers and normal body delivery.
func TestMeteredResponsePreservesAllowedStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := &meteredWriter{ResponseWriter: rec, ctx: context.Background()}
	writer.WriteHeader(http.StatusCreated)
	if _, err := writer.Write([]byte("created")); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated || rec.Body.String() != "created" {
		t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
	}
}
