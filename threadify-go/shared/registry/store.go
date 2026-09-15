package registry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Runtime) initializeStore(ctx context.Context) error {
	if r.pool == nil {
		return errors.New("Registry usage accounting requires PostgreSQL")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('threadify:registry:schema',0))`); err != nil {
		return err
	}
	// No license key, plan, entitlement revision or quota values are persisted.
	_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS threadify_registry_binding (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), account_id text NOT NULL, company_id text NOT NULL);
 CREATE TABLE IF NOT EXISTS threadify_registry_delegations(nonce text PRIMARY KEY, expires_at timestamptz NOT NULL);
 CREATE TABLE IF NOT EXISTS threadify_registry_usage (
 account_id text NOT NULL, metric text NOT NULL, bucket_start timestamptz NOT NULL, bucket_seconds bigint NOT NULL, count bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(account_id,metric,bucket_start,bucket_seconds));
 CREATE TABLE IF NOT EXISTS threadify_registry_outbox (
 report_id text PRIMARY KEY, account_id text NOT NULL, installation_id text NOT NULL, metric text NOT NULL, bucket_start timestamptz NOT NULL, bucket_seconds bigint NOT NULL, count bigint NOT NULL);`)
	if err != nil {
		return fmt.Errorf("initialize Registry accounting: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO threadify_registry_binding(singleton,account_id,company_id) VALUES(true,$1,$2) ON CONFLICT DO NOTHING`, r.accountID, r.cfg.CompanyID); err != nil {
		return err
	}
	var account, company string
	if err = tx.QueryRow(ctx, `SELECT account_id,company_id FROM threadify_registry_binding WHERE singleton=true FOR UPDATE`).Scan(&account, &company); err != nil {
		return err
	}
	if account != r.accountID || company != r.cfg.CompanyID {
		return errors.New("database is already bound to another Registry account or company")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO companies(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING`, r.cfg.CompanyID, "Threadify "+r.snapshot.Email); err != nil {
		return fmt.Errorf("reconcile Registry company: %w", err)
	}
	return tx.Commit(ctx)
}
func (r *Runtime) reserve(ctx context.Context, metric string, n, monthly, rate int64, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	seconds := int64(month.AddDate(0, 1, 0).Sub(month) / time.Second)
	// Lock all writers to this metric, across engine/API processes and replicas.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, r.accountID+":"+metric); err != nil {
		return err
	}
	buckets := []struct {
		start          time.Time
		seconds, limit int64
	}{{month, seconds, monthly}}
	// Bandwidth has a monthly byte budget. Only incoming request counts need
	// a per-second bucket; byte size cannot consume a request-rate allowance.
	if metric == InputRequests {
		buckets = append(buckets, struct {
			start          time.Time
			seconds, limit int64
		}{now.Truncate(time.Second), 1, rate})
	}
	for _, b := range buckets {
		if _, err = tx.Exec(ctx, `INSERT INTO threadify_registry_usage(account_id,metric,bucket_start,bucket_seconds,count) VALUES($1,$2,$3,$4,0) ON CONFLICT DO NOTHING`, r.accountID, metric, b.start, b.seconds); err != nil {
			return err
		}
		tag, e := tx.Exec(ctx, `UPDATE threadify_registry_usage SET count=count+$5 WHERE account_id=$1 AND metric=$2 AND bucket_start=$3 AND bucket_seconds=$4 AND count <= 9223372036854775807-$5 AND ($6::bigint=-1 OR ($5 <= $6 AND count <= $6-$5))`, r.accountID, metric, b.start, b.seconds, n, b.limit)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrLimit
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO threadify_registry_outbox(report_id,account_id,installation_id,metric,bucket_start,bucket_seconds,count) VALUES($1,$2,$3,$4,$5,$6,$7)`, newID(), r.accountID, r.cfg.InstallationID, metric, month, seconds, n)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var errOutboxEmpty = errors.New("Registry usage outbox is empty")

// FlushUsage drains bounded batches so normal traffic cannot accumulate behind
// a single 500-record-per-heartbeat delivery. Unacknowledged rows remain durable.
func (r *Runtime) FlushUsage(ctx context.Context) error {
	if r == nil {
		return nil
	}
	cycleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := r.pool.Exec(cycleCtx, `DELETE FROM threadify_registry_delegations WHERE expires_at < NOW()`); err != nil {
		return err
	}
	for batch := 0; batch < 32; batch++ {
		err := r.flushUsageBatch(cycleCtx)
		if errors.Is(err, errOutboxEmpty) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) flushUsageBatch(ctx context.Context) error {
	if r == nil {
		return nil
	}
	// Lock pending reports through acknowledgement. Stable IDs make retries after
	// a lost HTTP response or failed commit safe at Registry.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT report_id,metric,bucket_start,bucket_seconds,count FROM threadify_registry_outbox WHERE account_id=$1 AND installation_id=$2 ORDER BY bucket_start,report_id LIMIT 500 FOR UPDATE SKIP LOCKED`, r.accountID, r.cfg.InstallationID)
	if err != nil {
		return err
	}
	reports := []Report{}
	ids := []string{}
	for rows.Next() {
		var report Report
		if err = rows.Scan(&report.ReportID, &report.Metric, &report.BucketStart, &report.BucketSeconds, &report.Count); err != nil {
			rows.Close()
			return err
		}
		reports = append(reports, report)
		ids = append(ids, report.ReportID)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(reports) == 0 {
		return errOutboxEmpty
	}
	body := map[string]any{"reported_at": time.Now().UTC(), "installation_id": r.cfg.InstallationID, "version": "threadify", "build_hash": "", "reports": reports}
	var response struct {
		Status   string `json:"status"`
		Accepted *int64 `json:"accepted"`
	}
	if err = r.request(ctx, "POST", "/api/threadify/usage-reports", body, &response); err != nil {
		return err
	}
	// Zero is a valid duplicate retry, but a missing or impossible count is not
	// an acknowledgement and must never cause durable reports to be deleted.
	if response.Status != "ok" || response.Accepted == nil || *response.Accepted < 0 || *response.Accepted > int64(len(reports)) {
		return errors.New("Registry did not acknowledge usage")
	}
	if _, err = tx.Exec(ctx, `DELETE FROM threadify_registry_outbox WHERE report_id=ANY($1)`, ids); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM threadify_registry_usage WHERE bucket_seconds=1 AND bucket_start < NOW()-interval '1 minute'`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GuardProfileCreation must execute in the same transaction as profile insertion.
// An advisory lock serializes explicit and automatic profile creation globally.
// Existing profiles can still be updated when a limit has been reduced.
func GuardProfileCreation(ctx context.Context, tx pgx.Tx, companyID string, newCount int64) error {
	r := Default()
	if r == nil {
		return nil
	}
	if err := r.CheckCompany(companyID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "threadify:profiles:"+companyID); err != nil {
		return err
	}
	s, err := r.Snapshot()
	if err != nil {
		return err
	}
	if newCount <= 0 || s.Entitlements.EntityProfileLimit == -1 {
		return nil
	}
	var count int64
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM entity_profile WHERE company_id=$1`, companyID).Scan(&count); err != nil {
		return err
	}
	limit := s.Entitlements.EntityProfileLimit
	if newCount > limit || count > limit-newCount {
		return ErrLimit
	}
	return nil
}
