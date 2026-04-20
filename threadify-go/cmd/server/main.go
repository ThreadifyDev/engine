package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"threadify-go/shared/logger"

	"github.com/threadify/engine/internal/app"
	"go.uber.org/zap"
)

const (
	pprofAddr       = "localhost:6060"
	shutdownTimeout = 30 * time.Second
)

func main() {
	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	cfg, err := app.LoadConfig()
	if err != nil {
		appLogger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engineApp, err := app.New(ctx, cfg, appLogger)
	if err != nil {
		appLogger.Fatal("failed to initialize app", zap.Error(err))
	}
	defer engineApp.Close(ctx)

	go startPprof(appLogger)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      engineApp.Handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		appLogger.Info("starting server", zap.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatal("server error", zap.Error(err))
		}
	}()

	waitForShutdown(appLogger, srv, engineApp)
}

func startPprof(logger *zap.Logger) {
	logger.Info("starting pprof server", zap.String("address", pprofAddr))
	if err := http.ListenAndServe(pprofAddr, nil); err != nil {
		logger.Error("pprof server error", zap.Error(err))
	}
}

func waitForShutdown(logger *zap.Logger, srv *http.Server, a *app.App) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}

	if err := a.Close(ctx); err != nil {
		logger.Error("error closing app dependencies", zap.Error(err))
	}

	logger.Info("server exited")
}
