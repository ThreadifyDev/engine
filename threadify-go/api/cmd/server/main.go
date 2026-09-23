// The Web API binary is the stateless hosted AI gateway. Customer management lives in the Engine.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"threadify-go/api/gateway"
)

func main() {
	cfg, err := gateway.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		if err := healthcheck(cfg.Port); err != nil {
			log.Fatal(err)
		}
		return
	}
	application, err := gateway.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: application, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 32 << 10}
	// No fixed WriteTimeout: inference SSE is bounded by the per-request context.
	stopped := make(chan error, 1)
	go func() { log.Printf("Threadify AI gateway listening on %s", srv.Addr); stopped <- srv.ListenAndServe() }()
	select {
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("gateway listener failed")
		}
	case <-ctx.Done():
		shutdown, release := context.WithTimeout(context.Background(), 30*time.Second)
		defer release()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
		}
	}
}

func healthcheck(port int) error {
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return errors.New("gateway health check failed")
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusOK {
		return errors.New("gateway is unhealthy")
	}
	return nil
}
