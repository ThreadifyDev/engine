// Package broker owns the optional in-process NATS JetStream server.
package broker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type Options struct {
	Mode           string
	StoreDir       string
	MaxMemoryBytes int64
	MaxStoreBytes  int64
}

// Runtime owns the broker, but not the client connections or consumers. Close
// those after draining their work and before closing this runtime.
type Runtime struct {
	server    *server.Server
	storeLock *os.File
	started   chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
	done      chan struct{}
}

func Start(ctx context.Context, cfg Options) (*Runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r := &Runtime{done: make(chan struct{})}
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "external":
		return r, nil
	case "embedded":
	default:
		return nil, fmt.Errorf("NATS mode must be embedded or external")
	}
	if strings.TrimSpace(cfg.StoreDir) == "" {
		return nil, fmt.Errorf("embedded NATS requires a persistent store_dir")
	}
	if cfg.MaxMemoryBytes <= 0 || cfg.MaxStoreBytes <= 0 {
		return nil, fmt.Errorf("embedded NATS memory and storage limits must be positive")
	}
	storeDir, err := filepath.Abs(cfg.StoreDir)
	if err != nil {
		return nil, fmt.Errorf("resolve embedded NATS storage: %w", err)
	}
	if err := os.MkdirAll(storeDir, 0700); err != nil {
		return nil, fmt.Errorf("create embedded NATS storage: %w", err)
	}
	// Fail before starting any goroutines if the persistent directory is unusable.
	probe, err := os.CreateTemp(storeDir, ".threadify-write-check-*")
	if err != nil {
		return nil, fmt.Errorf("embedded NATS storage is not writable: %w", err)
	}
	probe.Close()
	if err := os.Remove(probe.Name()); err != nil {
		return nil, fmt.Errorf("clean embedded NATS storage probe: %w", err)
	}
	// Keep the lock file in place across shutdowns so another process cannot
	// lock a replacement inode while an existing process still owns this one.
	r.storeLock, err = os.OpenFile(filepath.Join(storeDir, ".threadify.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open embedded NATS storage lock: %w", err)
	}
	if err := lockFile(r.storeLock); err != nil {
		r.storeLock.Close()
		return nil, fmt.Errorf("lock embedded NATS storage (another Threadify process may be using store_dir): %w", err)
	}
	r.server, err = server.NewServer(&server.Options{
		ServerName:         "threadify",
		DontListen:         true,
		NoSigs:             true,
		NoLog:              true,
		JetStream:          true,
		StoreDir:           storeDir,
		JetStreamMaxMemory: cfg.MaxMemoryBytes,
		JetStreamMaxStore:  cfg.MaxStoreBytes,
	})
	if err != nil {
		r.storeLock.Close()
		return nil, fmt.Errorf("configure embedded NATS: %w", err)
	}
	r.started = make(chan struct{})
	go func() {
		defer close(r.started)
		r.server.Start()
	}()
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		if err := startupCtx.Err(); err != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_ = r.Close(cleanupCtx)
			return nil, fmt.Errorf("start embedded NATS: %w", err)
		}
		if r.server.ReadyForConnections(100*time.Millisecond) && r.IsHealthy() {
			return r, nil
		}
	}
}

// ClientOptions routes clients through in-process connections. No TCP listener
// is opened in embedded mode. External connections retain their configured URL.
func (r *Runtime) ClientOptions() []nats.Option {
	if r == nil || r.server == nil {
		return nil
	}
	return []nats.Option{nats.InProcessServer(r.server)}
}

// IsHealthy reports the owned broker's status. External broker connectivity is
// checked by the application's client pool instead.
func (r *Runtime) IsHealthy() bool {
	if r == nil || r.closed.Load() {
		return false
	}
	if r.server == nil {
		return true
	}
	return r.server.Running() && r.server.JetStreamEnabled() &&
		r.server.Healthz(&server.HealthzOptions{JSEnabled: true}).Status == "ok"
}

func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		go func() {
			defer close(r.done)
			if r.server != nil {
				// DontListen makes Start return after initialization. Let it
				// finish before shutdown, even when startup was canceled.
				<-r.started
				r.server.Shutdown()
				r.server.WaitForShutdown()
			}
			if r.storeLock != nil {
				r.storeLock.Close()
			}
		}()
	})
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop embedded NATS: %w", ctx.Err())
	}
}
