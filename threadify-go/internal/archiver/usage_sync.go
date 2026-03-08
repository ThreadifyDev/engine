package archiver

import (
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type UsageSyncEvent struct {
	EventID           string
	CompanyID         string
	Meter             string
	Amount            int64
	BillingCycleStart time.Time
	OccurredAt        time.Time
	Msg               jetstream.Msg
}
