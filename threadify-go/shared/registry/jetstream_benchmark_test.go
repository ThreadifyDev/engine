package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"threadify-go/shared/testutil/natsfixture"
)

type countedUsageKV struct {
	jetstream.KeyValue
	reads, writes, conflicts atomic.Int64
}

func (kv *countedUsageKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.reads.Add(1)
	return kv.KeyValue.Get(ctx, key)
}

func (kv *countedUsageKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	kv.writes.Add(1)
	seq, err := kv.KeyValue.Update(ctx, key, value, revision)
	if errors.Is(err, jetstream.ErrKeyExists) {
		kv.conflicts.Add(1)
	}
	return seq, err
}

// Real broker calls isolate accounting from the SDK, validation and archival work.
func BenchmarkJetStreamAdmission(b *testing.B) {
	for _, workers := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			pool := testPool(b)
			ctx := context.Background()
			s := testSnapshot()
			r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
			if err := r.initializeStore(ctx); err != nil {
				b.Fatal(err)
			}
			if err := r.EnableJetStream(ctx, natsfixture.New(b)); err != nil {
				b.Fatal(err)
			}
			kv := &countedUsageKV{KeyValue: r.meter.Load().kv}
			r.meter.Load().kv = kv
			var next atomic.Int64
			var wg sync.WaitGroup
			b.ResetTimer()
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for next.Add(1) <= int64(b.N) {
						if err := r.CheckBatch(ctx, Usage{InputRequests, 1}, Usage{InputBytes, 256}); err != nil {
							b.Error(err)
							return
						}
					}
				}()
			}
			wg.Wait()
			b.StopTimer()
			b.ReportMetric(float64(kv.reads.Load())/float64(b.N), "reads/op")
			b.ReportMetric(float64(kv.writes.Load())/float64(b.N), "writes/op")
			b.ReportMetric(float64(kv.conflicts.Load())/float64(b.N), "conflicts/op")
		})
	}
}
