package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"threadify-go/shared/logger"

	"github.com/threadify/engine/internal/archiver"
	appconfig "github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
)

const shutdownTimeout = 10 * time.Second

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	flag.Parse()

	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	if err := run(*configPath, appLogger); err != nil {
		appLogger.Fatal("archiver exited with error", zap.Error(err))
	}
}

func run(configPath string, logger *zap.Logger) error {
	viper.SetConfigFile(configPath)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	cfg, err := appconfig.LoadFromViper()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if !cfg.Archiver.Enabled {
		logger.Info("archiver disabled in config, exiting")
		return nil
	}

	db, err := database.NewPostgresDB(cfg.Postgres.URL, cfg.Postgres.MaxConnections)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()

	logger.Info("connected to postgres", zap.String("url", maskURL(cfg.Postgres.URL)))

	if err := db.InitSchema(context.Background()); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stepStateConsumer, natsConn, err := startNATSConsumers(ctx, cfg, db, logger)
	if err != nil {
		logger.Warn("NATS consumers not started", zap.Error(err))
	}

	metricsSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Archiver.MetricsPort),
		Handler: promhttp.Handler(),
	}
	go func() {
		logger.Info("metrics server listening", zap.String("addr", metricsSrv.Addr))
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", zap.Error(err))
		}
	}()

	logger.Info("archiver ready")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received, stopping...")

	cancel()

	if stepStateConsumer != nil {
		stepStateConsumer.Stop()
	}
	if natsConn != nil {
		natsConn.Drain()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("metrics server shutdown error", zap.Error(err))
	}

	logger.Info("archiver stopped")
	return nil
}

func startNATSConsumers(
	ctx context.Context,
	cfg *appconfig.Config,
	db *database.PostgresDB,
	logger *zap.Logger,
) (stepState *archiver.StepStateConsumer, natsConn *nats.Conn, err error) {
	natsURL := cfg.NATS.URL
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect nats: %w", err)
	}

	hostname, _ := os.Hostname()
	consumerPrefix := fmt.Sprintf("archiver-%s-%d", hostname, os.Getpid())

	natsConsumer, err := archiver.NewNATSConsumer(
		nc, db,
		cfg.Archiver.Streams.BatchSize,
		cfg.Archiver.Streams.BlockTimeout,
		consumerPrefix+"-nats",
		cfg,
		logger,
	)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("create nats consumer: %w", err)
	}

	go func() {
		if err := natsConsumer.Start(ctx); err != nil {
			logger.Error("nats consumer exited with error", zap.Error(err))
		}
	}()

	flushInterval := cfg.Archiver.Streams.StepStateFlushInterval
	if flushInterval == 0 {
		flushInterval = 5 * time.Second
	}

	stepStateConsumer, err := archiver.NewStepStateConsumer(
		nc, db,
		cfg.Archiver.Streams.BatchSize,
		flushInterval,
		consumerPrefix+"-step-state",
		logger,
	)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("create step state consumer: %w", err)
	}

	if err := stepStateConsumer.Start(ctx); err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("start step state consumer: %w", err)
	}

	logger.Info("nats consumers started", zap.String("url", natsURL))
	return stepStateConsumer, nc, nil
}

func maskURL(url string) string {
	idx := strings.Index(url, "@")
	if idx < 0 {
		return url
	}
	colonIdx := strings.LastIndex(url[:idx], ":")
	if colonIdx < 0 {
		return url
	}
	return url[:colonIdx+1] + "****" + url[idx:]
}
