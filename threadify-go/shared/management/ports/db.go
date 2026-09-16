package ports

import (
	"context"
	"threadify-go/shared/management/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type SQLExecutor interface {
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
}

type Tx interface {
	domain.Tx
	SQLExecutor
}

type DBPool interface {
	Begin(ctx context.Context) (Tx, error)
	SQLExecutor
}
