// Package managedvalkey owns a real Valkey child process. External mode never
// starts or stops a server, including when its configured endpoint is unavailable.
package managedvalkey

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

type Options struct {
	Mode, Host, Bind, Password, StoreDir, BinaryPath string
	Port                                             int
	StartupTimeout                                   time.Duration
}

type Runtime struct {
	cmd      *exec.Cmd
	done     chan struct{}
	stopOnce sync.Once
	waitErr  error // Published by closing done.
}

func Start(ctx context.Context, cfg Options) (_ *Runtime, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Mode == "external" || cfg.Mode == "" {
		return &Runtime{}, nil
	}
	if cfg.Mode != "managed" {
		return nil, fmt.Errorf("Valkey mode must be managed or external")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("managed Valkey requires Linux or macOS; use redis.mode=external on this platform")
	}
	ip := net.ParseIP(cfg.Bind)
	if ip == nil {
		return nil, fmt.Errorf("redis.bind must be a single IP address")
	}
	if !ip.IsLoopback() && cfg.Password == "" {
		return nil, fmt.Errorf("a password is required when managed Valkey listens beyond loopback")
	}
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.StartupTimeout <= 0 {
		return nil, fmt.Errorf("invalid managed Valkey connection or startup timeout")
	}
	if !filepath.IsAbs(cfg.BinaryPath) || !filepath.IsAbs(cfg.StoreDir) {
		return nil, fmt.Errorf("managed Valkey binary and store paths must be absolute")
	}
	if info, err := os.Stat(cfg.BinaryPath); err != nil || info.IsDir() {
		return nil, fmt.Errorf("bundled valkey-server missing at %s; reinstall the complete release or set redis.binary_path for development", cfg.BinaryPath)
	}
	if err := os.MkdirAll(cfg.StoreDir, 0700); err != nil {
		return nil, fmt.Errorf("create Valkey store: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(cfg.StoreDir, ".threadify.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open Valkey store lock: %w", err)
	}
	if err := lockFile(lock); err != nil {
		lock.Close()
		return nil, fmt.Errorf("Valkey store already owned by another process: %w", err)
	}
	started := false
	defer func() {
		if !started {
			lock.Close()
		}
	}()
	// Never silently turn an existing installation into an empty cache. A missing
	// or damaged AOF requires explicit operator recovery, not automatic repair.
	marker := filepath.Join(cfg.StoreDir, ".threadify-managed")
	if _, err := os.Stat(marker); err == nil {
		if info, err := os.Stat(filepath.Join(cfg.StoreDir, "appendonlydir", "appendonly.aof.manifest")); err != nil || info.Size() == 0 {
			return nil, fmt.Errorf("managed Valkey persistence is missing; restore the complete Valkey store")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		entries, err := os.ReadDir(cfg.StoreDir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Name() != ".threadify.lock" {
				return nil, fmt.Errorf("unrecognized Valkey store; use an empty directory or restore a complete managed store")
			}
		}
	}
	// Configuration goes through stdin so passwords do not appear in process args.
	configuration := fmt.Sprintf("bind %s\nport %d\ndir %s\nrequirepass %s\ndaemonize no\nprotected-mode yes\nappendonly yes\nappendfsync always\nno-appendfsync-on-rewrite no\naof-load-truncated no\nmaxmemory-policy noeviction\nsave \"\"\nlogfile \"\"\n", quote(cfg.Bind), cfg.Port, quote(cfg.StoreDir), quote(cfg.Password))
	output := &tailBuffer{secret: cfg.Password}
	cmd := exec.Command(cfg.BinaryPath, "-")
	cmd.Stdin = strings.NewReader(configuration)
	cmd.Stdout, cmd.Stderr = output, output
	// Keep the store locked if the Engine is killed before it can reap the child.
	// An orphan is never mistaken for permission to start a fresh state store.
	cmd.ExtraFiles = []*os.File{lock}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bundled Valkey: %w", err)
	}
	started = true
	r := &Runtime{cmd: cmd, done: make(chan struct{})}
	go func() { r.waitErr = cmd.Wait(); lock.Close(); close(r.done) }()
	defer func() {
		if retErr != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = r.Close(stopCtx)
		}
	}()
	probe := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), Password: cfg.Password, MaxRetries: -1, DialTimeout: 200 * time.Millisecond, ReadTimeout: 200 * time.Millisecond, WriteTimeout: 200 * time.Millisecond, ContextTimeoutEnabled: true})
	defer probe.Close()
	startup, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		// INFO also verifies identity: a process already occupying this port must
		// never satisfy the new child's readiness check.
		info, err := probe.Info(startup, "server").Result()
		if err == nil && strings.Contains(info, "\r\nprocess_id:"+strconv.Itoa(cmd.Process.Pid)+"\r\n") && probe.Ping(startup).Err() == nil && r.IsHealthy() {
			if err := writeMarker(marker); err != nil {
				return nil, fmt.Errorf("record managed store: %w", err)
			}
			return r, nil
		}
		select {
		case <-r.done:
			return nil, fmt.Errorf("bundled Valkey exited during startup: %v; %s", r.waitErr, output.String())
		case <-startup.Done():
			return nil, fmt.Errorf("managed Valkey not ready: %w; %s", startup.Err(), output.String())
		case <-tick.C:
		}
	}
}

func (r *Runtime) IsHealthy() bool {
	if r == nil || r.cmd == nil {
		return true
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

func (r *Runtime) Close(ctx context.Context) error {
	if r == nil || r.cmd == nil {
		return nil
	}
	r.stopOnce.Do(func() { _ = r.cmd.Process.Signal(syscall.SIGTERM) })
	select {
	case <-r.done:
		return r.waitErr
	case <-ctx.Done():
		_ = r.cmd.Process.Kill()
		return fmt.Errorf("Valkey shutdown: %w", ctx.Err())
	}
}

func writeMarker(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString("threadify-managed-valkey-v1\n"); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Valkey's config parser accepts C-style byte escapes, not Go's Unicode escapes.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 32 || c == 127:
			fmt.Fprintf(&b, "\\x%02x", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

type tailBuffer struct {
	mu           sync.Mutex
	text, secret string
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text += string(p)
	if len(b.text) > 8192 {
		b.text = b.text[len(b.text)-8192:]
	}
	return len(p), nil
}
func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.text
	if b.secret != "" {
		s = strings.ReplaceAll(s, b.secret, "[redacted]")
		s = strings.ReplaceAll(s, quote(b.secret), "[redacted]")
	}
	return s
}
