package archiver

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go/jetstream"
)

//go:generate mockgen -package=mocks -destination=mocks/archiver_mocks.go -source=interfaces.go

type DBExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

type ConsumerWriter interface {
	WriteActivityLog(ctx context.Context, events []StreamEvent) error
	WriteSubSteps(ctx context.Context, subSteps []map[string]interface{}) error
	SyncUsageMeters(ctx context.Context, events []UsageSyncEvent) error
	WriteThreadMetadata(ctx context.Context, events []StreamEvent) ([]string, error)
	WriteThreadAccess(ctx context.Context, events []StreamEvent) error
	WriteThreadValidations(ctx context.Context, events []StreamEvent) error
}

type JetStreamPublisher interface {
	CreateOrUpdateConsumer(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error)
	Publish(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}
