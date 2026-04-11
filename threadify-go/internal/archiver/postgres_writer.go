package archiver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	billing "threadify-go/shared/billing"

	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

var ErrThreadNotFound = errors.New("wait for thread metadata")

const creditUpsertQuery = `
    INSERT INTO credit_accounts (
        id, company_id, billing_cycle_start,
        credit_balance_millicents,
        credit_monthly_charged_millicents,
        last_sync_event_id, created_at, updated_at
    ) VALUES (
        $6, $2, $3,
        $1,
        $5,
        $4, NOW(), NOW()
    )
    ON CONFLICT (company_id, billing_cycle_start) DO UPDATE SET
        credit_balance_millicents         = credit_accounts.credit_balance_millicents + EXCLUDED.credit_balance_millicents,
        credit_monthly_charged_millicents = credit_accounts.credit_monthly_charged_millicents + EXCLUDED.credit_monthly_charged_millicents,
        last_sync_event_id                = EXCLUDED.last_sync_event_id,
        updated_at                        = NOW()
    WHERE credit_accounts.last_sync_event_id IS DISTINCT FROM EXCLUDED.last_sync_event_id`

type rowBuilder struct {
	cols   int
	rows   []string
	Values []interface{}
}

func newRowBuilder(cols int) *rowBuilder {
	return &rowBuilder{cols: cols}
}

func (rb *rowBuilder) add(vals ...interface{}) {
	rb.addRaw(nil, vals...)
}

func (rb *rowBuilder) addRaw(overrides map[int]string, vals ...interface{}) {
	base := len(rb.rows) * rb.cols
	p := make([]string, rb.cols)
	for j := range p {
		if expr, ok := overrides[j]; ok {
			p[j] = fmt.Sprintf(expr, base+j+1)
		} else {
			p[j] = fmt.Sprintf("$%d", base+j+1)
		}
	}
	rb.rows = append(rb.rows, "("+strings.Join(p, ", ")+")")
	rb.Values = append(rb.Values, vals...)
}

func (rb *rowBuilder) len() int             { return len(rb.rows) }
func (rb *rowBuilder) placeholders() string { return strings.Join(rb.rows, ", ") }

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deterministicID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

var activityTopLevelKeys = map[string]bool{
	"threadId": true, "type": true, "stepId": true,
	"actor": true, "actorService": true, "hash": true,
	"prevHash": true, "status": true, "contentHash": true,
	"timestamp": true, "startedAt": true, "finishedAt": true,
	"metadata": true,
}

type PostgresWriter struct {
	db     *database.PostgresDB
	logger *zap.Logger
}

func NewPostgresWriter(db *database.PostgresDB, logger *zap.Logger) *PostgresWriter {
	return &PostgresWriter{db: db, logger: logger}
}

func (w *PostgresWriter) batchExec(ctx context.Context, op, query string, values []interface{}) error {
	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (w *PostgresWriter) queryExistingIDs(ctx context.Context, table, column string, ids []string) (map[string]bool, error) {
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := w.db.Pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ANY($1::text[])`, column, table, column),
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("query existing %s.%s: %w", table, column, err)
	}
	defer rows.Close()

	existing := make(map[string]bool, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan %s.%s: %w", table, column, err)
		}
		existing[id] = true
	}
	return existing, rows.Err()
}

func partitionThreadEvents(events []StreamEvent) (insert, completion, ref []StreamEvent) {
	for _, e := range events {
		switch {
		case e.Data["action"] == "ref_added":
			ref = append(ref, e)
		case e.Data["status"] == "completed" || e.Data["status"] == "cancelled":
			completion = append(completion, e)
		default:
			insert = append(insert, e)
		}
	}
	return
}

func (w *PostgresWriter) WriteThreadMetadata(ctx context.Context, events []StreamEvent) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}
	w.logger.Info("received thread metadata events", zap.Int("count", len(events)))

	insert, completion, ref := partitionThreadEvents(events)

	if err := w.writeNewThreads(ctx, insert); err != nil {
		return nil, err
	}
	profileIDs, err := w.WriteThreadRefs(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := w.batchUpdateThreadStatus(ctx, completion); err != nil {
		return nil, err
	}
	return profileIDs, nil
}

func (w *PostgresWriter) writeNewThreads(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	companyIDs := make([]string, 0, len(events))
	ownerIDs := make([]string, 0, len(events))
	for _, e := range events {
		if id := e.Data["companyId"]; id != "" {
			companyIDs = append(companyIDs, id)
		}
		if id := e.Data["ownerId"]; id != "" {
			ownerIDs = append(ownerIDs, id)
		}
	}

	validCompanies, err := w.queryExistingIDs(ctx, "companies", "id", companyIDs)
	if err != nil {
		return err
	}
	validOwners, err := w.queryExistingIDs(ctx, "service_accounts", "id", ownerIDs)
	if err != nil {
		return err
	}

	tsOverride := map[int]string{
		8: "COALESCE($%d::timestamptz, NOW())",
		9: "COALESCE($%d::timestamptz, NOW())",
	}

	const cols = 10
	rb := newRowBuilder(cols)
	var skipped int

	for _, e := range events {
		companyID := e.Data["companyId"]
		if companyID == "" || !validCompanies[companyID] {
			w.logger.Warn("skipping thread with invalid company_id",
				zap.String("thread_id", e.Data["threadId"]),
				zap.String("company_id", companyID),
			)
			skipped++
			continue
		}

		var ownerID interface{}
		if id := e.Data["ownerId"]; id != "" && validOwners[id] {
			ownerID = id
		}

		var contractVersion interface{}
		if cv := e.Data["contractVersion"]; cv != "" && cv != "0" {
			contractVersion = cv
		}

		ts := e.Data["createdAt"]
		if ts == "" {
			ts = e.Data["startedAt"]
		}

		rb.addRaw(tsOverride,
			e.Data["threadId"],
			companyID,
			e.Data["contractId"],
			e.Data["contractName"],
			contractVersion,
			ownerID,
			e.Data["error"],
			e.Data["status"],
			nullableStr(ts),
			nullableStr(ts),
		)
	}

	if rb.len() == 0 {
		w.logger.Info("wrote thread metadata", zap.Int("count", 0), zap.Int("skipped", skipped))
		return nil
	}

	query := `INSERT INTO threads (
		id, company_id, contract_id, contract_name, contract_version,
		owner_id, error, status, created_at, updated_at
	) VALUES ` + rb.placeholders() + `
	ON CONFLICT (id) DO UPDATE SET
		company_id       = EXCLUDED.company_id,
		contract_id      = EXCLUDED.contract_id,
		contract_name    = EXCLUDED.contract_name,
		contract_version = EXCLUDED.contract_version,
		owner_id         = COALESCE(EXCLUDED.owner_id, threads.owner_id),
		error            = EXCLUDED.error,
		status           = EXCLUDED.status,
		updated_at       = COALESCE(EXCLUDED.updated_at, NOW())`

	if err := w.batchExec(ctx, "batch upsert threads", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote thread metadata", zap.Int("count", rb.len()), zap.Int("skipped", skipped))
	return nil
}

func (w *PostgresWriter) batchUpdateThreadStatus(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	const cols = 4
	rb := newRowBuilder(cols)
	for _, e := range events {
		completedAt := e.Data["completedAt"]
		if completedAt == "" {
			completedAt = e.Data["startedAt"]
		}

		rb.add(
			e.Data["threadId"],
			e.Data["status"],
			nullableStr(completedAt),
			nullableStr(e.Data["companyId"]),
		)
	}

	query := `UPDATE threads SET
		status       = v.status,
		completed_at = v.completed_at::timestamptz,
		updated_at   = v.completed_at::timestamptz,
		company_id   = COALESCE(v.company_id, threads.company_id)
	FROM (VALUES ` + rb.placeholders() + `) AS v(thread_id, status, completed_at, company_id)
	WHERE threads.id::text = v.thread_id`

	if err := w.batchExec(ctx, "batch update thread status", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("updated thread statuses", zap.Int("count", rb.len()))
	return nil
}

func (w *PostgresWriter) WriteThreadRefs(ctx context.Context, events []StreamEvent) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}

	threadIDs := make([]string, 0, len(events))
	for _, e := range events {
		threadIDs = append(threadIDs, e.Data["threadId"])
	}

	type refKey struct {
		threadID string
		key      string
	}
	unique := make(map[refKey]StreamEvent, len(events))
	ordered := make([]refKey, 0, len(events))
	for _, e := range events {
		k := refKey{e.Data["threadId"], e.Data["refKey"]}
		if _, exists := unique[k]; !exists {
			ordered = append(ordered, k)
		}
		unique[k] = e
	}

	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].threadID != ordered[j].threadID {
			return ordered[i].threadID < ordered[j].threadID
		}
		return ordered[i].key < ordered[j].key
	})

	tx, err := w.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin WriteThreadRefs tx: %w", err)
	}
	defer tx.Rollback(ctx)

	validRows, err := tx.Query(ctx, `SELECT id FROM threads WHERE id = ANY($1::text[])`, threadIDs)
	if err != nil {
		return nil, fmt.Errorf("query existing threads: %w", err)
	}
	validThreads := make(map[string]bool, len(threadIDs))
	for validRows.Next() {
		var id string
		if err := validRows.Scan(&id); err != nil {
			validRows.Close()
			return nil, fmt.Errorf("scan thread id: %w", err)
		}
		validThreads[id] = true
	}
	validRows.Close()
	if err := validRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate thread rows: %w", err)
	}

	const cols = 3
	rb := newRowBuilder(cols)
	var skipped int
	for _, k := range ordered {
		e := unique[k]
		if !validThreads[k.threadID] {
			w.logger.Warn("skipping ref for non-existent thread",
				zap.String("thread_id", k.threadID),
			)
			skipped++
			continue
		}
		rb.add(k.threadID, k.key, e.Data["refValue"])
	}

	if rb.len() == 0 {
		return nil, nil
	}

	refsQuery := `
		INSERT INTO thread_refs (thread_id, ref_key, ref_value)
		VALUES ` + rb.placeholders() + `
		ON CONFLICT (thread_id, ref_key) DO UPDATE SET
			ref_value  = EXCLUDED.ref_value,
			updated_at = NOW()`

	if _, err := tx.Exec(ctx, refsQuery, rb.Values...); err != nil {
		return nil, fmt.Errorf("write thread refs: %w", err)
	}

	profilesQuery := `
    WITH matched_types AS (
        SELECT id AS type_id, company_id, type
        FROM entity_profile_type
        WHERE archived_at IS NULL
    ),
    candidates AS (
        SELECT DISTINCT ON (t.company_id, mt.type_id, v.ref_value)
            t.company_id,
            mt.type_id,
            v.ref_value::varchar
        FROM (VALUES ` + rb.placeholders() + `) AS v(thread_id, ref_key, ref_value)
        JOIN threads t ON t.id = v.thread_id
        JOIN matched_types mt ON mt.company_id = t.company_id AND mt.type = v.ref_key
    )
    INSERT INTO entity_profile (
        id, company_id, entity_profile_type_id,
        name, ref_key, created_at, last_active_at
    )
    SELECT
        gen_random_uuid()::varchar,
        company_id,
        type_id,
        ref_value,
        ref_value,
        NOW(),
        NOW()
    FROM candidates
    ON CONFLICT (company_id, entity_profile_type_id, ref_key)
    DO UPDATE SET last_active_at = NOW()
    RETURNING id`

	profileRows, err := tx.Query(ctx, profilesQuery, rb.Values...)
	if err != nil {
		return nil, fmt.Errorf("auto-create entity profiles: %w", err)
	}

	seenProfiles := make(map[string]struct{})
	var profileIDs []string
	for profileRows.Next() {
		var id string
		if err := profileRows.Scan(&id); err != nil {
			profileRows.Close()
			return nil, fmt.Errorf("scan profile id: %w", err)
		}
		if _, exists := seenProfiles[id]; !exists {
			seenProfiles[id] = struct{}{}
			profileIDs = append(profileIDs, id)
		}
	}
	profileRows.Close()
	if err := profileRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit WriteThreadRefs tx: %w", err)
	}

	w.logger.Info("wrote thread refs and synced profiles",
		zap.Int("refs", rb.len()),
		zap.Int("profiles_touched", len(profileIDs)),
		zap.Int("skipped", skipped),
	)
	return profileIDs, nil
}

func (w *PostgresWriter) WriteThreadAccess(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	threadIDs := make([]string, 0, len(events))
	for _, e := range events {
		threadIDs = append(threadIDs, e.Data["threadId"])
	}
	validThreads, err := w.queryExistingIDs(ctx, "threads", "id", threadIDs)
	if err != nil {
		return err
	}

	const cols = 8
	rb := newRowBuilder(cols)
	var skipped int

	for _, e := range events {
		if !validThreads[e.Data["threadId"]] {
			w.logger.Debug("skipping access for non-existent thread",
				zap.String("thread_id", e.Data["threadId"]),
			)
			skipped++
			continue
		}

		var permissions []string
		if permStr := e.Data["permissions"]; permStr != "" {
			if err := json.Unmarshal([]byte(permStr), &permissions); err != nil {
				w.logger.Error("failed to parse permissions JSON",
					zap.String("thread_id", e.Data["threadId"]),
					zap.Error(err),
				)
				permissions = []string{}
			}
		}

		rb.add(
			e.Data["threadId"],
			e.Data["userId"],
			e.Data["roles"],
			e.Data["runtime_role"],
			permissions,
			e.Data["grantedBy"],
			nullableStr(e.Data["grantedAt"]),
			e.Data["status"],
		)
	}

	if rb.len() == 0 {
		if skipped > 0 {
			return ErrThreadNotFound
		}
		return nil
	}

	query := `INSERT INTO thread_access (
		thread_id, user_id, roles, runtime_role, permissions, granted_by, granted_at, status
	) VALUES ` + rb.placeholders() + `
	ON CONFLICT (thread_id, user_id) DO UPDATE SET
		roles        = EXCLUDED.roles,
		runtime_role = EXCLUDED.runtime_role,
		permissions  = EXCLUDED.permissions,
		granted_by   = EXCLUDED.granted_by,
		granted_at   = EXCLUDED.granted_at,
		status       = EXCLUDED.status`

	if err := w.batchExec(ctx, "write thread access", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote thread access events",
		zap.Int("success", rb.len()),
		zap.Int("skipped", skipped),
	)
	return nil
}

func (w *PostgresWriter) WriteValidationResults(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	const cols = 14
	rb := newRowBuilder(cols)
	for _, e := range events {
		rb.add(
			e.Data["validationID"],
			e.Data["threadID"],
			e.Data["stepID"],
			e.Data["stepName"],
			e.Data["idempotencyKey"],
			nullableStr(e.Data["timestamp"]),
			e.Data["validations"],
			e.Data["overallStatus"],
			e.Data["hasCriticalViolation"],
			e.Data["criticalCount"],
			e.Data["warningCount"],
			e.Data["minorCount"],
			e.Data["infoCount"],
			e.Data["totalValidations"],
		)
	}

	query := `INSERT INTO thread_validations (
		validation_id, thread_id, step_id, step_name, idempotency_key,
		timestamp, validations, overall_status, has_critical_violation,
		critical_count, warning_count, minor_count, info_count, total_validations
	) VALUES ` + rb.placeholders() + ` ON CONFLICT (validation_id) DO NOTHING`

	if err := w.batchExec(ctx, "write validation results", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote validation results", zap.Int("count", rb.len()))
	return nil
}

func (w *PostgresWriter) WriteThreadNotifications(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	const cols = 14
	rb := newRowBuilder(cols)
	for _, e := range events {
		rb.add(
			e.Data["notificationID"],
			e.Data["threadID"],
			e.Data["stepID"],
			e.Data["stepName"],
			e.Data["idempotencyKey"],
			e.Data["source"],
			e.Data["notificationType"],
			e.Data["stepStatus"],
			e.Data["validationStatus"],
			e.Data["violationType"],
			e.Data["severity"],
			e.Data["message"],
			e.Data["details"],
			nullableStr(e.Data["timestamp"]),
		)
	}

	query := `INSERT INTO thread_notifications (
		notification_id, thread_id, step_id, step_name, idempotency_key,
		source, notification_type, step_status, validation_status,
		violation_type, severity, message, details, timestamp
	) VALUES ` + rb.placeholders() + ` ON CONFLICT (notification_id) DO NOTHING`

	if err := w.batchExec(ctx, "write thread notifications", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote thread notifications", zap.Int("count", rb.len()))
	return nil
}

func (w *PostgresWriter) WriteActivityLog(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(events))
	deduped := make([]StreamEvent, 0, len(events))
	for _, e := range events {
		h := e.Data["hash"]
		if h == "" {
			if e.Data["type"] == "step_recorded" {
				w.logger.Warn("step activity event missing hash, including without dedup",
					zap.String("thread_id", e.Data["threadId"]),
				)
			} else {
				w.logger.Debug("non-step activity lacks hash, including without dedup",
					zap.String("type", e.Data["type"]),
				)
			}
			deduped = append(deduped, e)
			continue
		}
		if _, dup := seen[h]; !dup {
			seen[h] = struct{}{}
			deduped = append(deduped, e)
		}
	}

	if len(seen) > 0 {
		hashes := make([]string, 0, len(seen))
		for h := range seen {
			hashes = append(hashes, h)
		}
		existing, err := w.queryExistingIDs(ctx, "thread_activities", "hash", hashes)
		if err != nil {
			return err
		}
		filtered := deduped[:0]
		for _, e := range deduped {
			if !existing[e.Data["hash"]] {
				filtered = append(filtered, e)
			}
		}
		deduped = filtered
	}

	if len(deduped) == 0 {
		return nil
	}

	tsOverride := map[int]string{6: "COALESCE($%d::timestamptz, NOW())"}

	const cols = 14
	rb := newRowBuilder(cols)

	for _, e := range deduped {
		payload := make(map[string]interface{})
		for k, v := range e.Data {
			if !activityTopLevelKeys[k] {
				payload[k] = v
			}
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal activity payload: %w", err)
		}

		var metadata interface{}
		if s := e.Data["metadata"]; s != "" {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(s), &m); err == nil {
				metadata = m
			}
		}

		rb.addRaw(tsOverride,
			e.Data["threadId"],
			e.Data["type"],
			e.Data["stepId"],
			e.Data["actor"],
			e.Data["actorService"],
			string(payloadJSON),
			nullableStr(e.Data["timestamp"]),
			e.Data["hash"],
			e.Data["prevHash"],
			e.Data["status"],
			e.Data["contentHash"],
			nullableStr(e.Data["startedAt"]),
			nullableStr(e.Data["finishedAt"]),
			metadata,
		)
	}

	query := `INSERT INTO thread_activities (
		thread_id, activity_type, step_id, actor, actor_service, payload,
		recorded_at, hash, prev_hash, status, content_hash, started_at, finished_at, metadata
	) VALUES ` + rb.placeholders()

	if err := w.batchExec(ctx, "write activity log", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote activity log events",
		zap.Int("total", len(events)),
		zap.Int("unique", rb.len()),
	)
	return nil
}

func (w *PostgresWriter) WriteSubSteps(ctx context.Context, subSteps []map[string]interface{}) error {
	if len(subSteps) == 0 {
		return nil
	}

	tsOverride := map[int]string{6: "COALESCE($%d::timestamptz, NOW())"}

	const cols = 7
	rb := newRowBuilder(cols)

	for _, ss := range subSteps {
		str := func(key string) string { s, _ := ss[key].(string); return s }

		payloadJSON := []byte("{}")
		if p, ok := ss["payload"].(map[string]interface{}); ok && len(p) > 0 {
			payloadJSON, _ = json.Marshal(p)
		}

		rb.addRaw(tsOverride,
			deterministicID(str("threadId"), str("stepId"), str("name")),
			str("threadId"),
			str("stepId"),
			str("name"),
			str("status"),
			string(payloadJSON),
			nullableStr(str("recordedAt")),
		)
	}

	query := `INSERT INTO step_substeps (
		id, thread_id, step_id, name, status, payload, recorded_at
	) VALUES ` + rb.placeholders() + ` ON CONFLICT (id) DO NOTHING`

	if err := w.batchExec(ctx, "write sub-steps", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote sub-steps", zap.Int("count", rb.len()))
	return nil
}

func (w *PostgresWriter) WriteThreadStepState(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	type dedupKey struct {
		threadID,
		stepName,
		idempKey string
	}
	seen := make(map[dedupKey]StreamEvent, len(events))
	for _, e := range events {
		seen[dedupKey{
			e.Data["threadId"],
			e.Data["stepName"],
			e.Data["idempotencyKey"],
		}] = e
	}

	const cols = 14
	rb := newRowBuilder(cols)
	for _, e := range seen {
		rb.add(
			e.Data["stepId"],
			e.Data["threadId"],
			e.Data["stepName"],
			e.Data["idempotencyKey"],
			e.Data["status"],
			e.Data["retryCount"],
			nullableStr(e.Data["firstSeenAt"]),
			nullableStr(e.Data["lastUpdatedAt"]),
			nullableStr(e.Data["startedAt"]),
			nullableStr(e.Data["finishedAt"]),
			e.Data["previousStep"],
			e.Data["actor"],
			e.Data["actorService"],
			e.Data["latestContext"],
		)
	}

	query := `INSERT INTO thread_step_states (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, started_at, finished_at,
		previous_step, actor, actor_service, latest_context
	) VALUES ` + rb.placeholders() + `
	ON CONFLICT (thread_id, step_name, idempotency_key) DO UPDATE SET
		status          = EXCLUDED.status,
		retry_count     = EXCLUDED.retry_count,
		last_updated_at = EXCLUDED.last_updated_at,
		started_at      = EXCLUDED.started_at,
		finished_at     = EXCLUDED.finished_at,
		previous_step   = EXCLUDED.previous_step,
		actor           = EXCLUDED.actor,
		actor_service   = EXCLUDED.actor_service,
		latest_context  = EXCLUDED.latest_context`

	if err := w.batchExec(ctx, "write step state", query, rb.Values); err != nil {
		return err
	}

	w.logger.Info("wrote step state events",
		zap.Int("total", len(events)),
		zap.Int("unique", rb.len()),
	)
	return nil
}

func (w *PostgresWriter) SyncUsageMeters(ctx context.Context, events []UsageSyncEvent) error {
	if len(events) == 0 {
		return nil
	}

	var processed, duplicates, skipped int

	for _, event := range events {
		var expectedSign int
		var chargedDelta int64

		switch event.Meter {
		case billing.MeterCreditSpend:
			expectedSign = -1
			chargedDelta = -event.Amount
		case billing.MeterCreditTopup:
			expectedSign = 1
			chargedDelta = 0
		default:
			w.logger.Warn("SyncUsageMeters: unrecognised meter, skipping",
				zap.String("meter", event.Meter),
				zap.String("company_id", event.CompanyID),
				zap.String("event_id", event.EventID),
			)
			skipped++
			continue
		}

		if (expectedSign > 0 && event.Amount <= 0) || (expectedSign < 0 && event.Amount >= 0) {
			w.logger.Warn("SyncUsageMeters: invalid amount sign for meter, skipping",
				zap.String("meter", event.Meter),
				zap.Int64("amount", event.Amount),
				zap.String("company_id", event.CompanyID),
				zap.String("event_id", event.EventID),
			)
			skipped++
			continue
		}

		id := deterministicID(event.CompanyID, event.BillingCycleStart.Format(time.RFC3339Nano), event.EventID)

		tag, err := w.db.Pool.Exec(ctx, creditUpsertQuery,
			event.Amount,
			event.CompanyID,
			event.BillingCycleStart,
			event.EventID,
			chargedDelta,
			id,
		)
		if err != nil {
			return fmt.Errorf("sync usage event %s: %w", event.EventID, err)
		}

		if tag.RowsAffected() == 0 {
			duplicates++
			continue
		}
		processed++
	}

	if processed > 0 || duplicates > 0 || skipped > 0 {
		w.logger.Info("synced usage meters to postgres",
			zap.Int("processed", processed),
			zap.Int("duplicates", duplicates),
			zap.Int("skipped", skipped),
		)
	}

	return nil
}
