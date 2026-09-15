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

	"go.uber.org/zap"

	"threadify-go/api/app"
	"threadify-go/shared/config"
	"threadify-go/shared/logger"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		if err := runHealthCheck(); err != nil {
			log.Printf("health check failed: %v", err)
			os.Exit(1)
		}
		return
	}

	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	rootCtx, rootCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer rootCancel()

	cfg, err := loadConfig(appLogger)
	if err != nil {
		appLogger.Fatal("load config", zap.Error(err))
	}

	rbacPaths, err := app.ResolveRBACPaths(appLogger)
	if err != nil {
		appLogger.Fatal("resolve rbac paths", zap.Error(err))
	}

	application, err := app.New(rootCtx, cfg, rbacPaths, appLogger)
	if err != nil {
		appLogger.Fatal("initialize app", zap.Error(err))
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebAPI.Port),
		Handler:      application.Handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		appLogger.Info("web API starting", zap.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatal("server error", zap.Error(err))
		}
	}()

	<-rootCtx.Done()

	appLogger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("server forced shutdown", zap.Error(err))
	}

	if err := application.Close(shutdownCtx, appLogger); err != nil {
		appLogger.Error("app close error", zap.Error(err))
	}

	appLogger.Info("shutdown complete")
}

func runHealthCheck() error {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/app/config/config.yaml"
	}

	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", cfg.WebAPI.Port))
	if err != nil {
		return fmt.Errorf("request health endpoint: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}

func loadConfig(logger *zap.Logger) (*config.Config, error) {
	path, err := resolveConfigPath(logger)
	if err != nil {
		return nil, err
	}
	return config.Load(path)
}

func resolveConfigPath(logger *zap.Logger) (string, error) {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		logger.Info("using config path from CONFIG_PATH env", zap.String("path", p))
		return p, nil
	}
	if _, err := os.Stat("/app/config/config.yaml"); err == nil {
		logger.Info("using config path", zap.String("path", "/app/config/config.yaml"))
		return "/app/config/config.yaml", nil
	}
	devPath := "../config/config.yaml"
	if _, err := os.Stat(devPath); err == nil {
		logger.Info("using config path (dev fallback)", zap.String("path", devPath))
		return devPath, nil
	}
	return "", errors.New("config file not found: set CONFIG_PATH env var or provide /app/config/config.yaml")
}
