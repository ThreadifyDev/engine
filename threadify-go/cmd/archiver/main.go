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
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"threadify-go/shared/logger"

	"github.com/threadify/engine/internal/archiver"
	appconfig "github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/workerpool"
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
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		logger.Warn("failed to read config file, using environment variables", zap.String("path", configPath), zap.Error(err))
	} else {
		logger.Info("loaded config", zap.String("path", configPath))
	}

	cfg, err := appconfig.LoadFromViper(v)
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

	if err := db.InitSchema(context.Background()); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize enrichment worker pool
	enrichmentPool := initEnrichmentWorkerPool(cfg, logger)
	logger.Info("enrichment worker pool initialized",
		zap.Int("min_workers", cfg.Archiver.EnrichmentWorkers.MinWorkers),
		zap.Int("max_workers", cfg.Archiver.EnrichmentWorkers.MaxWorkers),
		zap.Int("queue_size", cfg.Archiver.EnrichmentWorkers.QueueSize),
	)

	stepStateConsumer, intelligenceConsumer, natsConn, err := startNATSConsumers(ctx, cfg, db, logger)
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

	// Shutdown enrichment worker pool first (allow pending jobs to complete)
	logger.Info("shutting down enrichment worker pool...")
	if err := enrichmentPool.Shutdown(context.Background()); err != nil {
		logger.Warn("enrichment pool shutdown error", zap.Error(err))
	}

	if stepStateConsumer != nil {
		stepStateConsumer.Stop()
	}
	if intelligenceConsumer != nil {
		intelligenceConsumer.Stop()
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
) (stepState *archiver.StepStateConsumer, intel *archiver.IntelligenceConsumer, natsConn *nats.Conn, err error) {
	natsURL := cfg.NATS.URL
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect nats: %w", err)
	}

	hostname, _ := os.Hostname()
	consumerPrefix := fmt.Sprintf("archiver-%s-%d", hostname, os.Getpid())

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, nil, fmt.Errorf("create jetstream: %w", err)
	}

	natsConsumer, err := archiver.NewNATSConsumer(
		js, db.Pool,
		cfg.Archiver.Streams.BatchSize,
		cfg.Archiver.Streams.BlockTimeout,
		consumerPrefix+"-nats",
		cfg,
		logger,
	)
	if err != nil {
		nc.Close()
		return nil, nil, nil, fmt.Errorf("create nats consumer: %w", err)
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
		js, db.Pool,
		cfg.Archiver.Streams.BatchSize,
		flushInterval,
		consumerPrefix+"-step-state",
		logger,
	)
	if err != nil {
		nc.Close()
		return nil, nil, nil, fmt.Errorf("create step state consumer: %w", err)
	}

	if err := stepStateConsumer.Start(ctx); err != nil {
		nc.Close()
		return nil, nil, nil, fmt.Errorf("start step state consumer: %w", err)
	}

	intelligenceConsumer, err := archiver.NewIntelligenceConsumer(
		js, db.Pool,
		cfg.Archiver.Streams.BatchSize,
		flushInterval,
		consumerPrefix+"-intelligence",
		logger,
	)
	if err != nil {
		nc.Close()
		return nil, nil, nil, fmt.Errorf("create intelligence consumer: %w", err)
	}

	if err := intelligenceConsumer.Start(ctx); err != nil {
		nc.Close()
		return nil, nil, nil, fmt.Errorf("start intelligence consumer: %w", err)
	}

	logger.Info("nats consumers started", zap.String("url", natsURL))
	return stepStateConsumer, intelligenceConsumer, nc, nil
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

// initEnrichmentWorkerPool creates and starts the enrichment worker pool
func initEnrichmentWorkerPool(cfg *appconfig.Config, logger *zap.Logger) *workerpool.Pool {
	poolCfg := workerpool.Config{
		Name:             "enrichment",
		MinWorkers:       cfg.Archiver.EnrichmentWorkers.MinWorkers,
		MaxWorkers:       cfg.Archiver.EnrichmentWorkers.MaxWorkers,
		QueueSize:        cfg.Archiver.EnrichmentWorkers.QueueSize,
		ScaleUpThreshold: cfg.Archiver.EnrichmentWorkers.ScaleUpThreshold,
		ScaleDownAfter:   time.Duration(cfg.Archiver.EnrichmentWorkers.ScaleDownAfterMs) * time.Millisecond,
		JobTimeout:       time.Duration(cfg.Archiver.EnrichmentWorkers.JobTimeoutMs) * time.Millisecond,
		SubmitRetryWait:  time.Duration(cfg.Archiver.EnrichmentWorkers.SubmitRetryWaitMs) * time.Millisecond,
		Metrics:          workerpool.NoOpMetrics{},
	}

	return workerpool.New(poolCfg)
}
