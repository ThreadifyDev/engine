package database

import (
	"context"

	"threadify-go/shared/management/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	// All table creation is now handled by the main engine's InitSchema
	// at /threadify-go/internal/database/postgres.go
	return nil
}

type poolWrapper struct {
	pool *pgxpool.Pool
}

func (w *poolWrapper) Begin(ctx context.Context) (ports.Tx, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &txWrapper{tx: tx}, nil
}

func (w *poolWrapper) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return w.pool.Exec(ctx, query, args...)
}
func (w *poolWrapper) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return w.pool.QueryRow(ctx, query, args...)
}
func (w *poolWrapper) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return w.pool.Query(ctx, query, args...)
}

type txWrapper struct {
	tx pgx.Tx
}

func (w *txWrapper) Commit(ctx context.Context) error   { return w.tx.Commit(ctx) }
func (w *txWrapper) Rollback(ctx context.Context) error { return w.tx.Rollback(ctx) }
func (w *txWrapper) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return w.tx.Exec(ctx, query, args...)
}
func (w *txWrapper) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return w.tx.QueryRow(ctx, query, args...)
}
func (w *txWrapper) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return w.tx.Query(ctx, query, args...)
}

func WrapPool(pool *pgxpool.Pool) ports.DBPool {
	return &poolWrapper{pool: pool}
}
