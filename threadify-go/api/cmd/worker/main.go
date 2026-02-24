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
	"threadify-go/shared/nats"
)

func main() {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := initDB(cfg.Postgres.URL)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}
	defer db.Close()

	natsClient, err := nats.NewClient(&cfg.NATS)
	if err != nil {
		log.Fatalf("connect NATS: %v", err)
	}
	defer natsClient.Close()

	if err := natsClient.InitializeOutboxStream(); err != nil {
		log.Fatalf("init outbox stream: %v", err)
	}

	authClient, err := sharedauth.NewAuthClientFromSharedConfig(cfg)
	if err != nil {
		log.Fatalf("init auth client: %v", err)
	}

	outboxRepo := repository.NewOutboxRepository(db)
	userRepo := repository.NewUserRepository(db)
	emailSvc, err := service.NewEmailService(
		cfg.WebAPI.Email.PlunkAPIKey,
		cfg.WebAPI.Email.PlunkAPIURL,
		cfg.WebAPI.FrontendURL,
	)

	if err != nil {
		log.Fatalf("init email service: %v", err)
	}

	encryptionKey := strings.TrimSpace(os.Getenv("OUTBOX_ENCRYPTION_KEY"))
	outboxWorker := worker.NewOutboxWorker(outboxRepo, userRepo, authClient, emailSvc, encryptionKey)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runPruner(ctx, outboxRepo)

	log.Println("outbox worker starting")
	outboxWorker.Run(ctx, natsClient.JetStream())
	log.Println("outbox worker stopped")
}

func runPruner(ctx context.Context, repo *repository.OutboxRepository) {
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
				log.Printf("pruner: failed to prune old events: %v", err)
			} else {
				log.Printf("pruner: removed %d events older than %s", count, cutoff.Format(time.DateOnly))
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
