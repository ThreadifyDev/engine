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
	postgresrepo "github.com/threadify/engine/internal/repository/postgres"
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

	valkeyClient, err := database.NewValkeyService(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.PoolSize,
		cfg.Redis.MinIdleConns,
		cfg.Redis.MaxIdleConns,
		cfg.Redis.MaxRetries,
		cfg.Redis.DialTimeoutMs,
		cfg.Redis.ReadTimeoutMs,
		cfg.Redis.WriteTimeoutMs,
		cfg.Redis.PoolTimeoutMs,
		cfg.Redis.ConnMaxIdleTimeMs,
	)
	if err != nil {
		return fmt.Errorf("connect valkey: %w", err)
	}
	defer valkeyClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsRepo := postgresrepo.NewMetricsRepository(db.Pool, valkeyClient, logger)

	runtime, natsConn, err := startNATSConsumers(ctx, cfg, db, metricsRepo, logger)
	if err != nil {
		return fmt.Errorf("start persistence: %w", err)
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
	defer signal.Stop(quit)
	<-quit

	logger.Info("shutdown signal received, stopping...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	persistenceErr := runtime.Close(shutdownCtx)
	cancel()
	if natsConn != nil {
		natsConn.Close()
	}

	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("metrics server shutdown error", zap.Error(err))
	}

	logger.Info("archiver stopped")
	return persistenceErr
}

func startNATSConsumers(
	ctx context.Context,
	cfg *appconfig.Config,
	db *database.PostgresDB,
	metricsInvalidator archiver.MetricsInvalidator,
	logger *zap.Logger,
) (runtime *archiver.Runtime, natsConn *nats.Conn, err error) {
	natsURL := cfg.NATS.URL
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("create jetstream: %w", err)
	}

	runtime, err = archiver.NewRuntime(js, db.Pool, metricsInvalidator, cfg, logger)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("create persistence runtime: %w", err)
	}
	if err := runtime.Start(ctx); err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("start persistence runtime: %w", err)
	}

	logger.Info("nats consumers started", zap.String("url", natsURL))
	return runtime, nc, nil
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
