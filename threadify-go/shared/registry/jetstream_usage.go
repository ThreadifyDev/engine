package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go/jetstream"
)

const usageBucketName = "THREADIFY_USAGE"

// usageState keeps cumulative observations, never plan limits or license secrets.
// A compacted KV update is both the admission decision and its durable usage record.
type usageState struct {
	Version      int                   `json:"version"`
	Epoch        string                `json:"epoch"`
	Account      string                `json:"account"`
	Installation string                `json:"installation"`
	Totals       map[string]usageTotal `json:"totals"`
	RateStart    int64                 `json:"rate_start"`
	RateCount    int64                 `json:"rate_count"`
}
type usageTotal struct {
	Metric  string `json:"metric"`
	Start   int64  `json:"start"`
	Seconds int64  `json:"seconds"`
	Count   int64  `json:"count"`
}
type usageMeter struct {
	kv           jetstream.KeyValue
	key, epoch   string
	admission    chan struct{}
	state        *usageState // Protected by admission; never sufficient to allow without CAS.
	revision     uint64
	projectionMu sync.Mutex
	projected    uint64
}

// EnableJetStream binds this runtime to the shared durable coordinator before
// serving traffic. Missing state after migration is an error, never a fresh quota.
func (r *Runtime) EnableJetStream(ctx context.Context, js jetstream.JetStream) error {
	if r == nil || js == nil {
		return errors.New("JetStream is required for usage accounting")
	}
	if r.meter.Load() != nil {
		return errors.New("JetStream usage is already initialized")
	}
	kv, err := js.KeyValue(ctx, usageBucketName)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: usageBucketName, Description: "Threadify durable usage counters", History: 1, Storage: jetstream.FileStorage})
		if err != nil {
			kv, err = js.KeyValue(ctx, usageBucketName)
		}
	}
	if err != nil {
		return fmt.Errorf("open usage KV: %w", err)
	}
	status, err := kv.Status(ctx)
	if err != nil {
		return err
	}
	// Expiry/eviction or memory-only storage could silently replenish an allowance.
	cfg := status.Config()
	if cfg.Storage != jetstream.FileStorage || cfg.TTL != 0 {
		return errors.New("usage KV requires file storage and no TTL")
	}
	digest := sha256.Sum256([]byte(r.accountID + ":" + r.cfg.InstallationID))
	meter := &usageMeter{kv: kv, key: hex.EncodeToString(digest[:]), admission: make(chan struct{}, 1)}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "threadify:usage:migration:"+meter.key); err != nil {
		return err
	}
	var epoch string
	var revision int64
	// Keep the checkpoint and following KV read consistent with a concurrent projector.
	err = tx.QueryRow(ctx, `SELECT epoch,revision FROM threadify_registry_projection WHERE singleton=true FOR UPDATE`).Scan(&epoch, &revision)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	initialized := err == nil
	entry, getErr := kv.Get(ctx, meter.key)
	if getErr != nil && !errors.Is(getErr, jetstream.ErrKeyNotFound) {
		return getErr
	}
	if getErr != nil {
		if initialized {
			return errors.New("durable usage state is missing; restore the JetStream store before admitting traffic")
		}
		state := usageState{Version: 1, Epoch: newID(), Account: r.accountID, Installation: r.cfg.InstallationID, Totals: map[string]usageTotal{}}
		// The one-time migration imports existing usage and keeps unsent reports intact.
		rows, err := tx.Query(ctx, `SELECT metric,bucket_start,bucket_seconds,count FROM threadify_registry_usage WHERE account_id=$1`, r.accountID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var total usageTotal
			var start time.Time
			if err = rows.Scan(&total.Metric, &start, &total.Seconds, &total.Count); err != nil {
				rows.Close()
				return err
			}
			total.Start = start.Unix()
			if total.Seconds == 1 {
				if total.Metric == InputRequests && total.Start > state.RateStart {
					state.RateStart = total.Start
					state.RateCount = total.Count
				}
			} else {
				state.Totals[totalKey(total)] = total
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		raw, _ := json.Marshal(state)
		if _, err = kv.Create(ctx, meter.key, raw); err != nil {
			return err
		}
		entry, err = kv.Get(ctx, meter.key)
		if err != nil {
			return err
		}
	}
	state, err := r.decodeUsage(entry.Value(), "")
	if err != nil {
		return err
	}
	if initialized && state.Epoch != epoch {
		return errors.New("usage state generation differs from database binding")
	}
	// An old broker backup must not undo a newer durable SQL checkpoint.
	if initialized && entry.Revision() < uint64(revision) {
		return errors.New("usage KV is older than its database checkpoint; restore current broker state")
	}
	rows, err := tx.Query(ctx, `SELECT metric,bucket_start,bucket_seconds,count FROM threadify_registry_usage WHERE account_id=$1 AND bucket_seconds>1`, r.accountID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var total usageTotal
		var start time.Time
		if err := rows.Scan(&total.Metric, &start, &total.Seconds, &total.Count); err != nil {
			rows.Close()
			return err
		}
		total.Start = start.Unix()
		if total.Count < 0 || state.Totals[totalKey(total)].Count < total.Count {
			rows.Close()
			return errors.New("usage KV is behind persisted counters; restore current broker state")
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	meter.epoch = state.Epoch
	if initialized {
		meter.projected = uint64(revision)
	} else {
		// Importing old counters creates no new reporting delta. If a prior startup
		// created KV but failed its DB commit, normal projection still computes deltas.
		if _, err = tx.Exec(ctx, `INSERT INTO threadify_registry_projection(singleton,epoch,revision) VALUES(true,$1,0)`, state.Epoch); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	meter.state, meter.revision = &state, entry.Revision()
	if !r.meter.CompareAndSwap(nil, meter) {
		return errors.New("JetStream usage initialized concurrently")
	}
	return nil
}

func totalKey(total usageTotal) string {
	return total.Metric + ":" + strconv.FormatInt(total.Start, 10)
}

// decodeUsage rejects deleted, foreign, corrupt or replaced state before spending.
func (r *Runtime) decodeUsage(raw []byte, epoch string) (usageState, error) {
	var state usageState
	if json.Unmarshal(raw, &state) != nil || state.Version != 1 || state.Epoch == "" || state.Account != r.accountID || state.Installation != r.cfg.InstallationID || state.Totals == nil || state.RateStart < 0 || state.RateCount < 0 || epoch != "" && state.Epoch != epoch {
		return state, errors.New("invalid durable usage state")
	}
	for key, total := range state.Totals {
		if key != totalKey(total) || total.Count < 0 || total.Seconds <= 1 {
			return state, errors.New("invalid durable usage counter")
		}
		switch total.Metric {
		case InputBytes, OutputBytes, InputRequests, OutputMessages:
		default:
			return state, errors.New("invalid durable usage metric")
		}
		start := time.Unix(total.Start, 0).UTC()
		month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		if !start.Equal(month) || total.Seconds != int64(month.AddDate(0, 1, 0).Sub(month)/time.Second) {
			return state, errors.New("invalid durable usage window")
		}
	}
	return state, nil
}

// reserveMany performs no PostgreSQL work. CAS retries reread authoritative
// counters; an uncertain acknowledgement is never retried as another increment.
func (r *Runtime) reserveMany(ctx context.Context, reservations []usageReservation, now time.Time) error {
	meter := r.meter.Load()
	if meter == nil {
		return errors.New("JetStream usage accounting is not initialized")
	}
	// A key can accept only one CAS at a time. Queue local callers rather than
	// making them repeatedly collide at the broker; cancelled callers do not debit.
	select {
	case meter.admission <- struct{}{}:
		defer func() { <-meter.admission }()
	case <-ctx.Done():
		return ctx.Err()
	}
	for attempt := 0; attempt < 256; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if meter.state == nil {
			entry, err := meter.kv.Get(ctx, meter.key)
			if err != nil {
				return fmt.Errorf("load durable usage: %w", err)
			}
			state, err := r.decodeUsage(entry.Value(), meter.epoch)
			if err != nil {
				return err
			}
			meter.state, meter.revision = &state, entry.Revision()
		}
		// Only an acknowledged revision is cached. Another process's write forces
		// a CAS conflict and reload before admission; this is not a local quota lease.
		state := *meter.state
		state.Totals = maps.Clone(state.Totals)
		month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		for _, reservation := range reservations {
			total := usageTotal{Metric: reservation.metric, Start: month.Unix(), Seconds: int64(month.AddDate(0, 1, 0).Sub(month) / time.Second)}
			key := totalKey(total)
			total.Count = state.Totals[key].Count
			if reservation.n < 0 || total.Count > math.MaxInt64-reservation.n || reservation.monthly >= 0 && (reservation.n > reservation.monthly || total.Count > reservation.monthly-reservation.n) {
				return ErrLimit
			}
			if reservation.metric == InputRequests {
				second := now.Unix()
				// Clock skew or a delayed writer cannot reset a newer replica's window.
				if second > state.RateStart {
					state.RateStart = second
					state.RateCount = 0
				}
				if state.RateCount > math.MaxInt64-reservation.n || reservation.rate >= 0 && (reservation.n > reservation.rate || state.RateCount > reservation.rate-reservation.n) {
					return ErrLimit
				}
				state.RateCount += reservation.n
			}
			total.Count += reservation.n
			state.Totals[key] = total
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		revision, err := meter.kv.Update(ctx, meter.key, raw, meter.revision)
		if err == nil {
			meter.state, meter.revision = &state, revision
			return nil
		}
		// An uncertain acknowledgement may have committed. Discard the cache so
		// the next caller reads the durable result instead of forgetting that debit.
		meter.state = nil
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("reserve durable usage: %w", err)
		}
	}
	return errors.New("usage coordination contention exceeded retry budget")
}

// ProjectUsage checkpoints cumulative totals and reporting deltas in one SQL
// transaction. Revision and row locks make duplicate/out-of-order workers safe.
func (r *Runtime) ProjectUsage(ctx context.Context) error {
	meter := r.meter.Load()
	if meter == nil {
		return nil
	}
	meter.projectionMu.Lock()
	defer meter.projectionMu.Unlock()
	entry, err := meter.kv.Get(ctx, meter.key)
	if err != nil {
		return err
	}
	state, err := r.decodeUsage(entry.Value(), meter.epoch)
	if err != nil {
		return err
	}
	if entry.Revision() <= meter.projected {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var epoch string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT epoch,revision FROM threadify_registry_projection WHERE singleton=true FOR UPDATE`).Scan(&epoch, &revision); err != nil {
		return err
	}
	if epoch != state.Epoch {
		return errors.New("usage projection generation differs")
	}
	if entry.Revision() <= uint64(revision) {
		meter.projected = uint64(revision)
		return nil
	}
	// Fetch prior totals once and batch changed rows. Polling must not rewrite
	// every historical month or perform one network round trip per SQL statement.
	rows, err := tx.Query(ctx, `SELECT metric,bucket_start,bucket_seconds,count FROM threadify_registry_usage WHERE account_id=$1 AND bucket_seconds>1`, r.accountID)
	if err != nil {
		return err
	}
	priorTotals := make(map[string]int64)
	for rows.Next() {
		var total usageTotal
		var start time.Time
		if err := rows.Scan(&total.Metric, &start, &total.Seconds, &total.Count); err != nil {
			rows.Close()
			return err
		}
		total.Start = start.Unix()
		priorTotals[totalKey(total)] = total.Count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	keys := make([]string, 0, len(state.Totals))
	for key := range state.Totals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	batch := &pgx.Batch{}
	for _, key := range keys {
		total := state.Totals[key]
		prior := priorTotals[key]
		if prior < 0 || total.Count < prior {
			return errors.New("usage projection would roll back a durable counter")
		}
		if delta := total.Count - prior; delta > 0 {
			start := time.Unix(total.Start, 0).UTC()
			batch.Queue(`INSERT INTO threadify_registry_usage(account_id,metric,bucket_start,bucket_seconds,count) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,metric,bucket_start,bucket_seconds) DO UPDATE SET count=EXCLUDED.count`, r.accountID, total.Metric, start, total.Seconds, total.Count)
			batch.Queue(`INSERT INTO threadify_registry_outbox(report_id,account_id,installation_id,metric,bucket_start,bucket_seconds,count) VALUES($1,$2,$3,$4,$5,$6,$7)`, newID(), r.accountID, r.cfg.InstallationID, total.Metric, start, total.Seconds, delta)
		}
	}
	if state.RateStart > 0 {
		batch.Queue(`INSERT INTO threadify_registry_usage(account_id,metric,bucket_start,bucket_seconds,count) VALUES($1,$2,$3,1,$4) ON CONFLICT(account_id,metric,bucket_start,bucket_seconds) DO UPDATE SET count=GREATEST(threadify_registry_usage.count,EXCLUDED.count)`, r.accountID, InputRequests, time.Unix(state.RateStart, 0).UTC(), state.RateCount)
	}
	batch.Queue(`UPDATE threadify_registry_projection SET revision=$1 WHERE singleton=true`, int64(entry.Revision()))
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	meter.projected = entry.Revision()
	return nil
}

// projectLoop runs independently of request admission and Registry heartbeats.
func (r *Runtime) projectLoop(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := r.ProjectUsage(bounded)
			cancel()
			if err != nil && ctx.Err() == nil {
				r.logProjectionError(err)
			}
		}
	}
}
