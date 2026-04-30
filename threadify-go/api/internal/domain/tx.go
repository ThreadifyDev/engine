package domain

import "context"

// Execer is an opaque transactional exec context passed into repositories.
// Concrete implementations live in outer layers (e.g., Postgres tx wrappers).
type Execer interface{}

type Tx interface {
	Execer() Execer
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type TxManager interface {
	Begin(ctx context.Context) (Tx, error)
}
