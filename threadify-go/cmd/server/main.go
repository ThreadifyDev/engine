package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/threadify/engine/internal/app"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
	"threadify-go/shared/logger"
)

// Release builds inject these values through Go linker flags.
var (
	version = "dev"
	commit  = "unknown"
)

const shutdownTimeout = 30 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("threadify", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Threadify Engine. Use the separate threadify-cli client for login, contracts, profiles and threads.")
		flags.PrintDefaults()
	}
	configPath := flags.String("config", os.Getenv("CONFIG_PATH"), "Engine configuration file")
	mode := flags.String("mode", "", "combined (default), engine, or writer; split modes require external NATS")
	showVersion := flags.Bool("version", false, "print version and commit, then exit")
	healthcheck := flags.Bool("healthcheck", false, "check the configured server's health and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	// Version inspection must work without configuration or external services.
	if *showVersion {
		fmt.Printf("threadify %s (%s)\n", version, commit)
		return nil
	}
	cfg, err := app.LoadConfigPath(*configPath)
	if err != nil {
		return err
	}
	if *mode != "" {
		cfg.RuntimeMode = *mode
	}
	if err := config.ValidateRuntime(cfg); err != nil {
		return err
	}
	if *healthcheck {
		return checkHealth(cfg)
	}

	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer appLogger.Sync() //nolint:errcheck
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	engineApp, err := app.New(ctx, cfg, appLogger)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}
	srv := &http.Server{
		Addr: net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port)), Handler: engineApp.Handler,
		ReadTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		appLogger.Info("starting Threadify", zap.String("address", srv.Addr), zap.String("mode", cfg.RuntimeMode), zap.String("broker", cfg.NATS.Mode))
		serveErr <- srv.ListenAndServe()
	}()
	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	}
	engineApp.BeginShutdown()
	appLogger.Info("shutting down")
	// Request cancellation must not cancel persistence's final writes.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, err)
		_ = srv.Close()
	}
	runErr = errors.Join(runErr, engineApp.Close(shutdownCtx))
	appLogger.Info("shutdown complete")
	return runErr
}

func checkHealth(cfg *config.Config) error {
	host := cfg.Server.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Server.Port)) + "/health")
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: HTTP %d", response.StatusCode)
	}
	return nil
}
