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
	"sync"
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

	if err := startNATSConsumers(ctx, cfg, db, logger); err != nil {
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("metrics server shutdown error", zap.Error(err))
	}

	cancel()
	logger.Info("archiver stopped")
	return nil
}

func startNATSConsumers(
	ctx context.Context,
	cfg *appconfig.Config,
	db *database.PostgresDB,
	logger *zap.Logger,
) error {
	natsURL := cfg.NATS.URL
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}

	if err := ensureArchiverStreams(ctx, nc, logger); err != nil {
		nc.Close()
		return fmt.Errorf("ensure archiver streams: %w", err)
	}

	natsConsumer, err := archiver.NewNATSConsumer(
		nc, db,
		cfg.Archiver.Streams.BatchSize,
		cfg.Archiver.Streams.BlockTimeout,
		"archiver-nats-1",
		cfg,
		logger,
	)
	if err != nil {
		nc.Close()
		return fmt.Errorf("create nats consumer: %w", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := natsConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("nats consumer error", zap.Error(err))
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
		"archiver-step-state-1",
		logger,
	)
	if err != nil {
		return fmt.Errorf("create step state consumer: %w", err)
	}
	go func() {
		defer wg.Done()
		if err := stepStateConsumer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("step state consumer error", zap.Error(err))
		}
		stepStateConsumer.Stop()
	}()

	go func() {
		wg.Wait()
		nc.Close()
	}()

	logger.Info("nats consumers started", zap.String("url", natsURL))
	return nil
}

func ensureArchiverStreams(ctx context.Context, nc *nats.Conn, log *zap.Logger) error {
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("create jetstream context: %w", err)
	}

	streams := []struct {
		name     string
		subjects []string
	}{
		{"activity_log", []string{"activity.log"}},
		{"thread_metadata", []string{"metadata.thread"}},
		{"thread_access", []string{"access.thread"}},
		{"thread_validations", []string{"validations.thread"}},
		{"state_step", []string{"state.step"}},
	}

	for _, s := range streams {
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     s.name,
			Subjects: s.subjects,
			Storage:  jetstream.FileStorage,
		})
		if err != nil {
			return fmt.Errorf("ensure stream %q: %w", s.name, err)
		}
		log.Info("ensured NATS stream", zap.String("stream", s.name))
	}
	return nil
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
