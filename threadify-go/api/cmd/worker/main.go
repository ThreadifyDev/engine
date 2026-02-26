package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/worker"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/config"
	"threadify-go/shared/logger"
	"threadify-go/shared/nats"

	"go.uber.org/zap"
)

const production = "production"

func main() {
	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == production)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		appLogger.Fatal("load config", zap.Error(err))
	}

	db, err := initDB(cfg.Postgres.URL)
	if err != nil {
		appLogger.Fatal("init database", zap.Error(err))
	}
	defer db.Close()

	natsClient, err := nats.NewClient(&cfg.NATS, appLogger)
	if err != nil {
		appLogger.Fatal("connect NATS", zap.Error(err))
	}
	defer natsClient.Close()

	if err := natsClient.InitializeOutboxStream(); err != nil {
		appLogger.Fatal("init outbox stream", zap.Error(err))
	}

	authClient, err := sharedauth.NewAuthClientFromSharedConfig(cfg)
	if err != nil {
		appLogger.Fatal("init auth client", zap.Error(err))
	}

	outboxRepo := repository.NewOutboxRepository(db)
	userRepo := repository.NewUserRepository(db)
	companyRepo := repository.NewCompanyRepository(db)
	emailSvc, err := service.NewEmailService(
		cfg.WebAPI.Email.PlunkAPIKey,
		cfg.WebAPI.Email.PlunkAPIURL,
		cfg.WebAPI.FrontendURL,
	)

	if err != nil {
		appLogger.Fatal("init email service", zap.Error(err))
	}

	encryptionKey := strings.TrimSpace(cfg.WebAPI.OutboxEncryptionKey)
	if encryptionKey == "" {
		appLogger.Fatal("outbox_encryption_key is required in config.yaml (or set OUTBOX_ENCRYPTION_KEY env var)")
	}
	outboxWorker := worker.NewOutboxWorker(outboxRepo, userRepo, companyRepo, authClient, emailSvc, encryptionKey, appLogger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runPruner(ctx, outboxRepo, appLogger)

	appLogger.Info("outbox worker starting")
	outboxWorker.Run(ctx, natsClient.JetStream())
	appLogger.Info("outbox worker stopped")
}

func runPruner(ctx context.Context, repo *repository.OutboxRepository, logger *zap.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-7 * 24 * time.Hour)
			count, err := repo.PruneProcessed(cutoff)
			if err != nil {
				logger.Error("pruner: failed to prune old events", zap.Error(err))
			} else {
				logger.Info("pruner: removed events",
					zap.Int64("count", count),
					zap.String("cutoff", cutoff.Format(time.DateOnly)),
				)
			}
		}
	}
}

func initDB(url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

func resolveConfigPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	if _, err := os.Stat("/app/config/config.yaml"); err == nil {
		return "/app/config/config.yaml"
	}
	return "../config/config.yaml"
}
