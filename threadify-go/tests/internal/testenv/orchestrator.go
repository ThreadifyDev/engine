package testenv

import (
	"context"
	"fmt"
	"log"

	"github.com/testcontainers/testcontainers-go"
)

type Environment struct {
	Postgres *PostgresContainer
	Valkey   *ValkeyContainer
	Nats     *NatsContainer
}

func Start(ctx context.Context) (*Environment, error) {
	log.Println("Starting test environment containers...")
	// Preflight so callers can reliably distinguish "Docker isn't available" from
	// other container or initialization failures.
	if _, err := testcontainers.NewDockerClientWithOpts(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDockerUnavailable, err)
	}

	pg, err := StartPostgresContainer(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("Postgres started at: %s\n", pg.ConnectionString)

	if err := pg.InitSchema(ctx); err != nil {
		pg.Terminate(ctx)
		return nil, fmt.Errorf("postgres schema init failed: %w", err)
	}
	log.Println("Postgres schema initialized")

	vk, err := StartValkeyContainer(ctx)
	if err != nil {
		pg.Terminate(ctx)
		return nil, err
	}
	log.Printf("Valkey started at: %s\n", vk.URI)

	nt, err := StartNatsContainer(ctx)
	if err != nil {
		pg.Terminate(ctx)
		vk.Terminate(ctx)
		return nil, err
	}
	log.Printf("NATS started at: %s\n", nt.URI)

	return &Environment{
		Postgres: pg,
		Valkey:   vk,
		Nats:     nt,
	}, nil
}

func (e *Environment) Stop(ctx context.Context) {
	log.Println("Stopping test environment containers...")
	if e.Nats != nil {
		e.Nats.Terminate(ctx)
	}
	if e.Valkey != nil {
		e.Valkey.Terminate(ctx)
	}
	if e.Postgres != nil {
		e.Postgres.Terminate(ctx)
	}
}
