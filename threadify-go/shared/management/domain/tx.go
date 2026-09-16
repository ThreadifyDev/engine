package domain

import "context"

type ExecContext interface{}

type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type TxManager interface {
	Begin(ctx context.Context) (Tx, error)
}
