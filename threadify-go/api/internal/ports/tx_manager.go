package ports

import (
	"context"

	"threadify-go/api/internal/domain"
)

func WrapAsTxManager(pool DBPool) domain.TxManager {
	return &txManagerAdapter{pool: pool}
}

type txManagerAdapter struct {
	pool DBPool
}

func (a *txManagerAdapter) Begin(ctx context.Context) (domain.Tx, error) {
	return a.pool.Begin(ctx)
}
