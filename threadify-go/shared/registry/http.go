package registry

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// StatusCode distinguishes quota exhaustion from unavailable verification/accounting.
func StatusCode(err error) int {
	if errors.Is(err, ErrLimit) {
		return http.StatusTooManyRequests
	}
	return http.StatusServiceUnavailable
}

// Wrap meters HTTP at its stream boundary. WebSocket frames are checked by the
// upgraded transport; keeping headers pending lets an initial denial return 429.
func (r *Runtime) Wrap(next http.Handler) http.Handler { return r.wrap(next, false) }

// WrapEngine alone accepts authenticated one-use proofs that the API already
// accounts for this hop. Normal API Wrap never trusts a caller-supplied proof.
func (r *Runtime) WrapEngine(next http.Handler) http.Handler { return r.wrap(next, true) }

func (r *Runtime) wrap(next http.Handler, acceptDelegated bool) http.Handler {
	if r == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Operational probes must remain available to diagnose a Registry outage.
		if req.URL.Path == "/health" || req.URL.Path == "/metrics" {
			next.ServeHTTP(w, req)
			return
		}
		if acceptDelegated && req.Header.Get(AccountingDelegationHeader) != "" {
			cleanup, err := r.acceptDelegation(req)
			if err != nil {
				status := StatusCode(err)
				if errors.Is(err, ErrDelegation) {
					status = http.StatusUnauthorized
				}
				// Invalid or replayed proofs are ordinary metered rejections;
				// merely adding a header must never bypass request/output quotas.
				r.wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, http.StatusText(status), status) }), false).ServeHTTP(w, req)
				return
			}
			defer cleanup()
			next.ServeHTTP(w, req)
			return
		}
		if err := r.Check(req.Context(), InputRequests, 1); err != nil {
			http.Error(w, http.StatusText(StatusCode(err)), StatusCode(err))
			return
		}
		writer := &meteredWriter{ResponseWriter: w, r: r, ctx: req.Context()}
		if req.Body != nil {
			req.Body = &meteredReader{ReadCloser: req.Body, r: r, ctx: req.Context(), onError: writer.setInputError}
		}
		next.ServeHTTP(writer, req)
		// Empty responses consume a response unit too, while hijacked streams own their framing.
		if !writer.started && !writer.hijacked {
			writer.finish()
		}
	})
}

type meteredReader struct {
	io.ReadCloser
	r       *Runtime
	ctx     context.Context
	onError func(error)
}

// Read admits bytes before the application can parse or persist them.
func (m *meteredReader) Read(p []byte) (int, error) {
	n, err := m.ReadCloser.Read(p)
	if n > 0 {
		if e := m.r.Check(m.ctx, InputBytes, int64(n)); e != nil {
			if m.onError != nil {
				m.onError(e)
			}
			_ = m.ReadCloser.Close()
			return 0, e
		}
	}
	return n, err
}

type meteredWriter struct {
	http.ResponseWriter
	r                                 *Runtime
	ctx                               context.Context
	started, messageCounted, hijacked bool
	status                            int
	inputError                        error
	errorMu                           sync.Mutex
}

// Request bodies may be consumed concurrently with a streaming response.
func (m *meteredWriter) setInputError(err error) {
	m.errorMu.Lock()
	defer m.errorMu.Unlock()
	if m.inputError == nil {
		m.inputError = err
	}
}
func (m *meteredWriter) getInputError() error {
	m.errorMu.Lock()
	defer m.errorMu.Unlock()
	return m.inputError
}

// WriteHeader defers the final status so rejected initial output is never a false 200.
func (m *meteredWriter) WriteHeader(code int) {
	if m.started || m.status != 0 {
		return
	}
	if code >= 100 && code < 200 {
		m.ResponseWriter.WriteHeader(code)
		return
	}
	m.status = code
}

// admitMessage counts HTTP responses once, including zero-length responses.
func (m *meteredWriter) admitMessage() error {
	if err := m.getInputError(); err != nil {
		return err
	}
	if !m.messageCounted {
		if err := m.r.Check(m.ctx, OutputMessages, 1); err != nil {
			return err
		}
		m.messageCounted = true
	}
	return nil
}

// Write checks each output chunk before handing it to the network writer.
func (m *meteredWriter) Write(p []byte) (int, error) {
	if err := m.admitMessage(); err != nil {
		return m.reject(err)
	}
	if err := m.r.Check(m.ctx, OutputBytes, int64(len(p))); err != nil {
		return m.reject(err)
	}
	m.commit()
	return m.ResponseWriter.Write(p)
}

// commit preserves the application's selected status after successful admission.
func (m *meteredWriter) commit() {
	if m.started {
		return
	}
	status := m.status
	if status == 0 {
		status = http.StatusOK
	}
	m.ResponseWriter.WriteHeader(status)
	m.started = true
}

// finish accounts for handlers which only set headers or return without a body.
func (m *meteredWriter) finish() {
	if err := m.admitMessage(); err != nil {
		_, _ = m.reject(err)
		return
	}
	m.commit()
}

// reject drops stale entity headers when a denial replaces an unsent response.
func (m *meteredWriter) reject(err error) (int, error) {
	if !m.started {
		m.Header().Del("Content-Length")
		m.Header().Del("Content-Encoding")
		m.ResponseWriter.WriteHeader(StatusCode(err))
		m.started = true
	}
	return 0, err
}

// Flush starts a metered response before preserving streaming semantics.
func (m *meteredWriter) Flush() {
	if err := m.admitMessage(); err != nil {
		_, _ = m.reject(err)
		return
	}
	m.commit()
	if f, ok := m.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	// SSE flushes delimit emitted event batches, each of which consumes an output unit.
	if strings.HasPrefix(strings.ToLower(m.Header().Get("Content-Type")), "text/event-stream") {
		m.messageCounted = false
	}
}

// Hijack transfers accounting to the WebSocket message hooks after a successful upgrade.
func (m *meteredWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := m.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking unsupported")
	}
	conn, rw, err := h.Hijack()
	if err == nil {
		m.hijacked = true
	}
	return conn, rw, err
}

// Unwrap supports standard HTTP response controllers without changing ownership.
func (m *meteredWriter) Unwrap() http.ResponseWriter { return m.ResponseWriter }
