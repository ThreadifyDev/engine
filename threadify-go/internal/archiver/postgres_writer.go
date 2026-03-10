package archiver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

var ErrThreadNotFound = errors.New("wait for thread metadata")

var meterColumns = map[string]string{
	"bandwidth_ingress": "bandwidth_ingress_balance",
	"bandwidth_egress":  "bandwidth_egress_balance",
}

var meterUpdateQueries = map[string]string{
	"bandwidth_ingress": `UPDATE usage_meters SET bandwidth_ingress_balance = bandwidth_ingress_balance - $1, updated_at = NOW() WHERE company_id = $2 AND billing_cycle_start = $3`,
	"bandwidth_egress":  `UPDATE usage_meters SET bandwidth_egress_balance = bandwidth_egress_balance - $1, updated_at = NOW() WHERE company_id = $2 AND billing_cycle_start = $3`,
}

type PostgresWriter struct {
	db     *database.PostgresDB
	logger *zap.Logger
}

func NewPostgresWriter(db *database.PostgresDB, logger *zap.Logger) *PostgresWriter {
	return &PostgresWriter{db: db, logger: logger}
}

func buildPlaceholders(rowCount, colCount int) string {
	rows := make([]string, rowCount)
	for i := range rows {
		cols := make([]string, colCount)
		for j := range cols {
			cols[j] = fmt.Sprintf("$%d", i*colCount+j+1)
		}
		rows[i] = "(" + strings.Join(cols, ", ") + ")"
	}
	return strings.Join(rows, ", ")
}

func deterministicUUID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func parseTimestamp(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
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

func (w *PostgresWriter) WriteThreadMetadata(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	w.logger.Info("received thread metadata events", zap.Int("count", len(events)))

	var refEvents, completionEvents, insertEvents []StreamEvent
	for _, event := range events {
		switch {
		case event.Data["action"] == "ref_added":
			refEvents = append(refEvents, event)
		case event.Data["status"] == "completed" || event.Data["status"] == "cancelled":
			completionEvents = append(completionEvents, event)
		default:
			insertEvents = append(insertEvents, event)
		}
	}

	if len(refEvents) > 0 {
		if err := w.WriteThreadRefs(ctx, refEvents); err != nil {
			return err
		}
	}

	if len(insertEvents) > 0 {

		companyIDs := make([]string, 0, len(insertEvents))
		ownerIDs := make([]string, 0, len(insertEvents))
		for _, event := range insertEvents {
			if id := event.Data["companyId"]; id != "" {
				companyIDs = append(companyIDs, id)
			}
			if id := event.Data["ownerId"]; id != "" {
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

		const cols = 10
		placeholderRows := make([]string, 0, len(insertEvents))
		values := make([]interface{}, 0, len(insertEvents)*cols)
		var skipped int

		for _, event := range insertEvents {
			companyID := event.Data["companyId"]
			if companyID == "" {
				w.logger.Warn("skipping thread with missing company_id",
					zap.String("thread_id", event.Data["threadId"]),
				)
				skipped++
				continue
			}
			if !validCompanies[companyID] {
				w.logger.Warn("skipping thread with unknown company_id",
					zap.String("thread_id", event.Data["threadId"]),
					zap.String("company_id", companyID),
				)
				skipped++
				continue
			}

			var ownerIDParam interface{}
			if ownerID := event.Data["ownerId"]; ownerID != "" && validOwners[ownerID] {
				ownerIDParam = ownerID
			}

			var contractVersionParam interface{}
			cv := event.Data["contractVersion"]
			if cv != "" && cv != "0" {
				contractVersionParam = cv
			}

			ts := event.Data["createdAt"]
			if ts == "" {
				ts = event.Data["startedAt"]
			}
			createdAt := parseTimestamp(ts)

			// Columns 9,10 (created_at, updated_at) are NOT NULL.
			base := len(placeholderRows) * cols
			p := make([]string, cols)
			for j := range p {
				p[j] = fmt.Sprintf("$%d", base+j+1)
			}
			p[8] = fmt.Sprintf("COALESCE($%d::timestamptz, NOW())", base+9)
			p[9] = fmt.Sprintf("COALESCE($%d::timestamptz, NOW())", base+10)
			placeholderRows = append(placeholderRows, "("+strings.Join(p, ", ")+")")

			values = append(values,
				event.Data["threadId"],
				companyID,
				event.Data["contractId"],
				event.Data["contractName"],
				contractVersionParam,
				ownerIDParam,
				event.Data["error"],
				event.Data["status"],
				createdAt,
				createdAt,
			)
		}

		validCount := len(placeholderRows)
		if validCount == 0 {
			w.logger.Info("wrote thread metadata", zap.Int("count", 0), zap.Int("skipped", skipped))
			return nil
		}

		query := `INSERT INTO threads (
		id, company_id, contract_id, contract_name, contract_version,
		owner_id, error, status, created_at, updated_at
	) VALUES ` + strings.Join(placeholderRows, ", ") + `
	ON CONFLICT (id) DO UPDATE SET
		company_id       = EXCLUDED.company_id,
		contract_id      = EXCLUDED.contract_id,
		contract_name    = EXCLUDED.contract_name,
		contract_version = EXCLUDED.contract_version,
		owner_id         = COALESCE(EXCLUDED.owner_id, threads.owner_id),
		error            = EXCLUDED.error,
		status           = EXCLUDED.status,
		updated_at       = COALESCE(EXCLUDED.updated_at, NOW())`

		if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
			return fmt.Errorf("batch upsert threads: %w", err)
		}

		w.logger.Info("wrote thread metadata", zap.Int("count", validCount), zap.Int("skipped", skipped))
	}

	if len(completionEvents) > 0 {
		if err := w.batchUpdateThreadStatus(ctx, completionEvents); err != nil {
			return fmt.Errorf("batch update thread status: %w", err)
		}
	}

	return nil
}

func (w *PostgresWriter) batchUpdateThreadStatus(ctx context.Context, events []StreamEvent) error {
	const cols = 4
	values := make([]interface{}, 0, len(events)*cols)
	for _, event := range events {
		completedAt := event.Data["completedAt"]
		if completedAt == "" {
			completedAt = event.Data["startedAt"]
		}
		values = append(values,
			event.Data["threadId"],
			event.Data["status"],
			parseTimestamp(completedAt),
			event.Data["companyId"],
		)
	}

	query := `UPDATE threads SET
		status       = v.status,
		completed_at = v.completed_at::timestamptz,
		updated_at   = v.completed_at::timestamptz,
		company_id   = COALESCE(v.company_id, threads.company_id)
	FROM (VALUES ` + buildPlaceholders(len(events), cols) + `) AS v(thread_id, status, completed_at, company_id)
	WHERE threads.id::text = v.thread_id`

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("batch update thread status: %w", err)
	}

	w.logger.Info("updated thread statuses", zap.Int("count", len(events)))
	return nil
}

func (w *PostgresWriter) WriteThreadRefs(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	threadIDs := make([]string, 0, len(events))
	for _, event := range events {
		threadIDs = append(threadIDs, event.Data["threadId"])
	}
	validThreads, err := w.queryExistingIDs(ctx, "threads", "id", threadIDs)
	if err != nil {
		return err
	}

	const cols = 3
	values := make([]interface{}, 0, len(events)*cols)
	var skipped int
	for _, event := range events {
		if !validThreads[event.Data["threadId"]] {
			w.logger.Warn("skipping ref for non-existent thread",
				zap.String("thread_id", event.Data["threadId"]),
			)
			skipped++
			continue
		}
		values = append(values,
			event.Data["threadId"],
			event.Data["refKey"],
			event.Data["refValue"],
		)
	}

	validCount := len(events) - skipped
	if validCount == 0 {
		return nil
	}

	query := `INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
		VALUES ` + buildPlaceholders(validCount, cols) + `
		ON CONFLICT (thread_id, ref_key) DO UPDATE SET
			ref_value  = EXCLUDED.ref_value,
			updated_at = NOW()`

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write thread refs: %w", err)
	}

	w.logger.Info("wrote thread refs", zap.Int("count", validCount), zap.Int("skipped", skipped))
	return nil
}

func (w *PostgresWriter) WriteThreadAccess(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	threadIDs := make([]string, 0, len(events))
	for _, event := range events {
		threadIDs = append(threadIDs, event.Data["threadId"])
	}
	validThreads, err := w.queryExistingIDs(ctx, "threads", "id", threadIDs)
	if err != nil {
		return err
	}

	var success, skipped int
	for _, event := range events {
		if !validThreads[event.Data["threadId"]] {
			w.logger.Debug("skipping access for non-existent thread",
				zap.String("thread_id", event.Data["threadId"]),
			)
			skipped++
			continue
		}

		var permissions []string
		if permStr := event.Data["permissions"]; permStr != "" {
			if err := json.Unmarshal([]byte(permStr), &permissions); err != nil {
				w.logger.Error("failed to parse permissions JSON",
					zap.String("thread_id", event.Data["threadId"]),
					zap.Error(err),
				)
				permissions = []string{}
			}
		}

		_, err := w.db.Pool.Exec(ctx, `
			INSERT INTO thread_access (
				thread_id, user_id, roles, runtime_role, permissions, granted_by, granted_at, status
			) VALUES ($1, $2, $3::jsonb, $4, $5::text[], $6, $7, $8)
			ON CONFLICT (thread_id, user_id) DO UPDATE SET
				roles        = EXCLUDED.roles,
				runtime_role = EXCLUDED.runtime_role,
				permissions  = EXCLUDED.permissions,
				granted_by   = EXCLUDED.granted_by,
				granted_at   = EXCLUDED.granted_at,
				status       = EXCLUDED.status`,
			event.Data["threadId"],
			event.Data["userId"],
			event.Data["roles"],
			event.Data["runtime_role"],
			permissions,
			event.Data["grantedBy"],
			parseTimestamp(event.Data["grantedAt"]),
			event.Data["status"],
		)
		if err != nil {
			return fmt.Errorf("write thread access for thread %s: %w", event.Data["threadId"], err)
		}
		success++
	}

	w.logger.Info("wrote thread access events", zap.Int("success", success), zap.Int("skipped", skipped))

	// If every event was skipped because its thread doesn't exist yet, signal the
	// caller to NAK and retry — the thread metadata batch just hasn't landed yet.
	if success == 0 && skipped > 0 {
		return ErrThreadNotFound
	}
	return nil
}

func (w *PostgresWriter) WriteValidationResults(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	const cols = 14
	values := make([]interface{}, 0, len(events)*cols)
	for _, event := range events {
		values = append(values,
			event.Data["validationID"],
			event.Data["threadID"],
			event.Data["stepID"],
			event.Data["stepName"],
			event.Data["idempotencyKey"],
			parseTimestamp(event.Data["timestamp"]),
			event.Data["validations"],
			event.Data["overallStatus"],
			event.Data["hasCriticalViolation"],
			event.Data["criticalCount"],
			event.Data["warningCount"],
			event.Data["minorCount"],
			event.Data["infoCount"],
			event.Data["totalValidations"],
		)
	}

	query := `INSERT INTO thread_validations (
		validation_id, thread_id, step_id, step_name, idempotency_key,
		timestamp, validations, overall_status, has_critical_violation,
		critical_count, warning_count, minor_count, info_count, total_validations
	) VALUES ` + buildPlaceholders(len(events), cols) + ` ON CONFLICT (validation_id) DO NOTHING`

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write validation results: %w", err)
	}

	w.logger.Info("wrote validation results", zap.Int("count", len(events)))
	return nil
}

func (w *PostgresWriter) WriteThreadNotifications(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	const cols = 14
	values := make([]interface{}, 0, len(events)*cols)
	for _, event := range events {
		values = append(values,
			event.Data["notificationID"],
			event.Data["threadID"],
			event.Data["stepID"],
			event.Data["stepName"],
			event.Data["idempotencyKey"],
			event.Data["source"],
			event.Data["notificationType"],
			event.Data["stepStatus"],
			event.Data["validationStatus"],
			event.Data["violationType"],
			event.Data["severity"],
			event.Data["message"],
			event.Data["details"],
			parseTimestamp(event.Data["timestamp"]),
		)
	}

	query := `INSERT INTO thread_notifications (
		notification_id, thread_id, step_id, step_name, idempotency_key,
		source, notification_type, step_status, validation_status,
		violation_type, severity, message, details, timestamp
	) VALUES ` + buildPlaceholders(len(events), cols) + ` ON CONFLICT (notification_id) DO NOTHING`

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write thread notifications: %w", err)
	}

	w.logger.Info("wrote thread notifications", zap.Int("count", len(events)))
	return nil
}

func (w *PostgresWriter) WriteActivityLog(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(events))
	deduped := make([]StreamEvent, 0, len(events))
	for _, event := range events {
		h := event.Data["hash"]
		if h == "" {
			deduped = append(deduped, event)
			continue
		}
		if _, exists := seen[h]; !exists {
			seen[h] = struct{}{}
			deduped = append(deduped, event)
		}
	}

	hashes := make([]string, 0, len(seen))
	for h := range seen {
		hashes = append(hashes, h)
	}
	if len(hashes) > 0 {
		existing, err := w.queryExistingIDs(ctx, "thread_activities", "hash", hashes)
		if err != nil {
			return err
		}
		filtered := deduped[:0]
		for _, event := range deduped {
			if _, dup := existing[event.Data["hash"]]; !dup {
				filtered = append(filtered, event)
			}
		}
		deduped = filtered
	}

	if len(deduped) == 0 {
		return nil
	}

	topLevel := map[string]bool{
		"threadId": true, "type": true, "stepId": true,
		"actor": true, "actorService": true, "hash": true,
		"prevHash": true, "status": true, "contentHash": true,
		"timestamp": true, "startedAt": true, "finishedAt": true,
		"metadata": true,
	}

	const cols = 14
	placeholderRows := make([]string, 0, len(deduped))
	values := make([]interface{}, 0, len(deduped)*cols)
	for i, event := range deduped {
		ts := event.Data["timestamp"]

		payload := make(map[string]interface{})
		for k, v := range event.Data {
			if !topLevel[k] {
				payload[k] = v
			}
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal activity payload: %w", err)
		}

		base := i * cols
		// Column 7 (recorded_at) is NOT NULL — use COALESCE so the DB fills in NOW() if missing.
		p := make([]string, cols)
		for j := range p {
			p[j] = fmt.Sprintf("$%d", base+j+1)
		}
		p[6] = fmt.Sprintf("COALESCE($%d::timestamptz, NOW())", base+7)
		placeholderRows = append(placeholderRows, "("+strings.Join(p, ", ")+")")

		// Handle metadata (already JSON string from server)
		metadataStr := event.Data["metadata"]
		var metadataVal interface{} = nil
		if metadataStr != "" {
			// Parse JSON string to ensure it's valid
			var metadataJSON map[string]interface{}
			if err := json.Unmarshal([]byte(metadataStr), &metadataJSON); err == nil {
				metadataVal = metadataJSON
			}
		}

		values = append(values,
			event.Data["threadId"],
			event.Data["type"],
			event.Data["stepId"],
			event.Data["actor"],
			event.Data["actorService"],
			string(payloadJSON),
			parseTimestamp(ts),
			event.Data["hash"],
			event.Data["prevHash"],
			event.Data["status"],
			event.Data["contentHash"],
			parseTimestamp(event.Data["startedAt"]),
			parseTimestamp(event.Data["finishedAt"]),
			metadataVal,
		)
	}

	query := `INSERT INTO thread_activities (
		thread_id, activity_type, step_id, actor, actor_service, payload,
		recorded_at, hash, prev_hash, status, content_hash, started_at, finished_at, metadata
	) VALUES ` + strings.Join(placeholderRows, ", ")

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write activity log: %w", err)
	}

	w.logger.Info("wrote activity log events", zap.Int("count", len(deduped)))
	return nil
}

func (w *PostgresWriter) WriteSubSteps(ctx context.Context, subSteps []map[string]interface{}) error {
	if len(subSteps) == 0 {
		return nil
	}

	const cols = 7
	placeholderRows := make([]string, 0, len(subSteps))
	values := make([]interface{}, 0, len(subSteps)*cols)
	for i, subStep := range subSteps {
		str := func(key string) string { s, _ := subStep[key].(string); return s }

		recordedAt := str("recordedAt")

		payloadJSON := []byte("{}")
		if payload, ok := subStep["payload"].(map[string]interface{}); ok && len(payload) > 0 {
			payloadJSON, _ = json.Marshal(payload)
		}

		id := deterministicUUID(str("threadId"), str("stepId"), str("name"))

		base := i * cols
		p := make([]string, cols)
		for j := range p {
			p[j] = fmt.Sprintf("$%d", base+j+1)
		}
		p[6] = fmt.Sprintf("COALESCE($%d::timestamptz, NOW())", base+7)
		placeholderRows = append(placeholderRows, "("+strings.Join(p, ", ")+")")

		values = append(values,
			id,
			str("threadId"),
			str("stepId"),
			str("name"),
			str("status"),
			string(payloadJSON),
			parseTimestamp(recordedAt),
		)
	}

	query := `INSERT INTO step_substeps (
		id, thread_id, step_id, name, status, payload, recorded_at
	) VALUES ` + strings.Join(placeholderRows, ", ") + ` ON CONFLICT (id) DO NOTHING`

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write sub-steps: %w", err)
	}

	w.logger.Info("wrote sub-steps", zap.Int("count", len(subSteps)))
	return nil
}

func (w *PostgresWriter) WriteThreadStepState(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	type dedupKey struct{ threadID, stepName, idempKey string }
	seen := make(map[dedupKey]StreamEvent, len(events))
	for _, event := range events {
		k := dedupKey{
			threadID: event.Data["threadId"],
			stepName: event.Data["stepName"],
			idempKey: event.Data["idempotencyKey"],
		}
		seen[k] = event
	}

	const cols = 14
	values := make([]interface{}, 0, len(seen)*cols)
	for _, event := range seen {
		values = append(values,
			event.Data["stepId"],
			event.Data["threadId"],
			event.Data["stepName"],
			event.Data["idempotencyKey"],
			event.Data["status"],
			event.Data["retryCount"],
			parseTimestamp(event.Data["firstSeenAt"]),
			parseTimestamp(event.Data["lastUpdatedAt"]),
			parseTimestamp(event.Data["startedAt"]),
			parseTimestamp(event.Data["finishedAt"]),
			event.Data["previousStep"],
			event.Data["actor"],
			event.Data["actorService"],
			event.Data["latestContext"],
		)
	}

	query := `INSERT INTO thread_step_states (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, started_at, finished_at,
		previous_step, actor, actor_service, latest_context
	) VALUES ` + buildPlaceholders(len(seen), cols) + `
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

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write step state: %w", err)
	}

	w.logger.Info("wrote step state events",
		zap.Int("total", len(events)),
		zap.Int("unique", len(seen)),
	)
	return nil
}

func (w *PostgresWriter) SyncUsageMeters(ctx context.Context, events []UsageSyncEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := w.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin usage sync tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	type aggregationKey struct {
		CompanyID         string
		Meter             string
		BillingCycleStart time.Time
	}
	aggregated := make(map[aggregationKey]int64)
	var duplicates int

	for _, event := range events {
		var insertedID string
		err := tx.QueryRow(ctx, `
			INSERT INTO usage_sync_events (event_id, company_id, meter, amount, occurred_at, processed_at, status)
			VALUES ($1, $2, $3, $4, $5, NOW(), 'processed')
			ON CONFLICT (event_id, occurred_at) DO NOTHING
			RETURNING event_id
		`, event.EventID, event.CompanyID, event.Meter, event.Amount, event.OccurredAt).Scan(&insertedID)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				duplicates++
				continue
			}
			return fmt.Errorf("persist usage sync event %s: %w", event.EventID, err)
		}

		key := aggregationKey{
			CompanyID:         event.CompanyID,
			Meter:             event.Meter,
			BillingCycleStart: event.BillingCycleStart,
		}
		aggregated[key] += event.Amount
	}

	for key, totalAmount := range aggregated {
		query := meterUpdateQueries[key.Meter]
		tag, err := tx.Exec(ctx, query, totalAmount, key.CompanyID, key.BillingCycleStart)
		if err != nil {
			return fmt.Errorf("batch sync usage %s for company %s: %w", key.Meter, key.CompanyID, err)
		}
		if tag.RowsAffected() == 0 {
			w.logger.Error("usage sync aggregation: no matching usage cycle found",
				zap.String("company_id", key.CompanyID),
				zap.String("meter", key.Meter),
				zap.Time("billing_cycle_start", key.BillingCycleStart),
			)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit usage sync aggregation tx: %w", err)
	}

	w.logger.Info("synced usage meters to postgres (aggregated)",
		zap.Int("input_events", len(events)),
		zap.Int("duplicates", duplicates),
		zap.Int("batch_updates", len(aggregated)),
	)
	return nil
}
