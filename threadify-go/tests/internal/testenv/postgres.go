package testenv

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/database"
)

type PostgresContainer struct {
	*postgres.PostgresContainer
	ConnectionString string
	Pool             *pgxpool.Pool
}

func StartPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	dbName := "threadify_test"
	dbUser := "postgres"
	dbPassword := "postgres"

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	return &PostgresContainer{
		PostgresContainer: container,
		ConnectionString:  connStr,
		Pool:              pool,
	}, nil
}

func (p *PostgresContainer) InitSchema(ctx context.Context) error {
	db, err := database.NewPostgresDB(p.ConnectionString, 10)
	if err != nil {
		return fmt.Errorf("failed to connect to test database: %w", err)
	}
	defer db.Close()

	if err := db.InitSchema(ctx); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	return nil
}

func (p *PostgresContainer) Terminate(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if p.Pool != nil {
		p.Pool.Close()
	}
	if p.PostgresContainer == nil {
		return nil
	}
	return p.PostgresContainer.Terminate(ctx)
}
