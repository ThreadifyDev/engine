package repository

import (
	"context"
	"fmt"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Common interfaces for transaction support - infrastructure specific
type DBExecer interface {
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
}

type txManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) domain.TxManager {
	return &txManager{pool: pool}
}

func (m *txManager) Begin(ctx context.Context) (domain.Tx, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &txAdapter{tx: tx}, nil
}

type txAdapter struct {
	tx pgx.Tx
}

func (a *txAdapter) Execer() domain.Execer {
	return a.tx
}

func (a *txAdapter) Commit(ctx context.Context) error {
	return a.tx.Commit(ctx)
}

func (a *txAdapter) Rollback(ctx context.Context) error {
	return a.tx.Rollback(ctx)
}

type txManagerFromDBPool struct {
	pool ports.DBPool
}

// NewTxManagerFromDBPool adapts a ports.DBPool into the domain.TxManager interface.
// Useful for tests and other call sites that already work with the DBPool port.
func NewTxManagerFromDBPool(pool ports.DBPool) domain.TxManager {
	return &txManagerFromDBPool{pool: pool}
}

func (m *txManagerFromDBPool) Begin(ctx context.Context) (domain.Tx, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &txAdapterFromPort{tx: tx}, nil
}

type txAdapterFromPort struct {
	tx ports.Tx
}

func (a *txAdapterFromPort) Execer() domain.Execer {
	return a.tx
}

func (a *txAdapterFromPort) Commit(ctx context.Context) error {
	return a.tx.Commit(ctx)
}

func (a *txAdapterFromPort) Rollback(ctx context.Context) error {
	return a.tx.Rollback(ctx)
}
