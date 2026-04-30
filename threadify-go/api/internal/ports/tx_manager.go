package ports

import (
	"context"

	"threadify-go/api/internal/domain"
)

// WrapAsTxManager adapts a ports.DBPool into the domain.TxManager interface.
// This is mainly useful in tests where a DBPool mock already exists.
func WrapAsTxManager(pool DBPool) domain.TxManager {
	return &txManagerAdapter{pool: pool}
}

type txManagerAdapter struct {
	pool DBPool
}

func (m *txManagerAdapter) Begin(ctx context.Context) (domain.Tx, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &txAdapter{tx: tx}, nil
}

type txAdapter struct {
	tx Tx
}

func (t *txAdapter) Execer() domain.Execer              { return t.tx }
func (t *txAdapter) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *txAdapter) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
