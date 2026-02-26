package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

type PostgresWriter struct {
	db     *database.PostgresDB
	logger *zap.Logger
}

func NewPostgresWriter(db *database.PostgresDB, logger *zap.Logger) *PostgresWriter {
	return &PostgresWriter{db: db, logger: logger}
}

// buildPlaceholders returns a ($1,$2,...),($N+1,...) placeholder string for
// a batch INSERT with rowCount rows and colCount columns per row.
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

// nullIfEmpty returns nil when s is empty, otherwise s.
// Use for optional fields that map to nullable DB columns.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (w *PostgresWriter) WriteStepEvents(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	const cols = 12
	values := make([]interface{}, 0, len(events)*cols)
	for _, event := range events {
		contextJSON, err := json.Marshal(event.Data["context"])
		if err != nil {
			w.logger.Error("failed to marshal context", zap.String("step_id", event.Data["stepId"]), zap.Error(err))
			return fmt.Errorf("marshal context: %w", err)
		}
		values = append(values,
			event.Data["stepId"], event.Data["threadId"], event.Data["stepName"],
			event.Data["serviceName"], event.Data["type"], event.Data["status"],
			string(contextJSON), event.Data["startedAt"], event.Data["finishedAt"],
			event.Data["timestamp"], event.Data["hash"], event.Data["prevHash"],
		)
	}

	query := `INSERT INTO thread_activities (
		step_id, thread_id, step_name, service_name, type, status,
		context, started_at, finished_at, timestamp, hash, prev_hash
	) VALUES ` + buildPlaceholders(len(events), cols) + ` ON CONFLICT (step_id) DO NOTHING`

	start := time.Now()
	_, err := w.db.Pool.Exec(ctx, query, values...)
	duration := time.Since(start)
	table := "thread_activities"
	if err != nil {
		DatabaseWritesTotal.WithLabelValues(table, "error").Inc()
		DatabaseWriteDuration.WithLabelValues(table).Observe(duration.Seconds())
		return fmt.Errorf("write step events: %w", err)
	}
	DatabaseWritesTotal.WithLabelValues(table, "success").Inc()
	DatabaseWriteDuration.WithLabelValues(table).Observe(duration.Seconds())
	w.logger.Info("wrote step events", zap.Int("count", len(events)), zap.Duration("duration", duration))
	return nil
}

func (w *PostgresWriter) WriteThreadMetadata(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	refEvents := make([]StreamEvent, 0)
	metadataEvents := make([]StreamEvent, 0)
	for _, event := range events {
		if event.Data["action"] == "ref_added" {
			refEvents = append(refEvents, event)
		} else {
			metadataEvents = append(metadataEvents, event)
		}
	}

	if len(refEvents) > 0 {
		if err := w.WriteThreadRefs(ctx, refEvents); err != nil {
			return err
		}
	}

	for _, event := range metadataEvents {
		if status := event.Data["status"]; status == "completed" || status == "cancelled" {
			completedAt := event.Data["completedAt"]
			if completedAt == "" {
				completedAt = event.Data["startedAt"]
			}
			_, err := w.db.Pool.Exec(ctx, `
				UPDATE threads
				SET status = $1, completed_at = $2, updated_at = $2,
				    company_id = COALESCE($4, company_id)
				WHERE id = $3`,
				status, completedAt, event.Data["thread_id"], nullIfEmpty(event.Data["companyId"]),
			)
			if err != nil {
				w.logger.Warn("failed to update thread status",
					zap.String("thread_id", event.Data["thread_id"]), zap.Error(err))
			}
			continue
		}

		companyID := event.Data["companyId"]
		if companyID == "" {
			w.logger.Error("missing company_id for thread", zap.String("thread_id", event.Data["threadId"]))
			return fmt.Errorf("missing company_id for thread %s", event.Data["threadId"])
		}

		var companyExists bool
		if err := w.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1)`, companyID,
		).Scan(&companyExists); err != nil {
			return fmt.Errorf("validate company_id: %w", err)
		}
		if !companyExists {
			return fmt.Errorf("company_id %q not found", companyID)
		}

		var ownerIDParam interface{} = nullIfEmpty(event.Data["ownerId"])
		if ownerID := event.Data["ownerId"]; ownerID != "" {
			var exists bool
			if err := w.db.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM service_accounts WHERE id = $1)`, ownerID,
			).Scan(&exists); err != nil || !exists {
				ownerIDParam = nil
			}
		}

		_, err := w.db.Pool.Exec(ctx, `
			INSERT INTO threads (
				id, company_id, contract_id, contract_name, contract_version,
				owner_id, error, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				company_id       = EXCLUDED.company_id,
				contract_id      = EXCLUDED.contract_id,
				contract_name    = EXCLUDED.contract_name,
				contract_version = EXCLUDED.contract_version,
				owner_id         = EXCLUDED.owner_id,
				error            = EXCLUDED.error,
				updated_at       = EXCLUDED.updated_at`,
			event.Data["threadId"], companyID,
			event.Data["contractId"], event.Data["contractName"],
			nullIfEmpty(event.Data["contractVersion"]),
			ownerIDParam, event.Data["error"],
			event.Data["startedAt"], event.Data["startedAt"],
		)
		if err != nil {
			return fmt.Errorf("upsert thread %s: %w", event.Data["threadId"], err)
		}
	}

	w.logger.Info("wrote thread metadata", zap.Int("count", len(metadataEvents)))
	return nil
}

func (w *PostgresWriter) WriteThreadRefs(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		_, err := w.db.Pool.Exec(ctx, `
			INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (thread_id, ref_key) DO UPDATE SET
				ref_value  = EXCLUDED.ref_value,
				updated_at = NOW()`,
			event.Data["threadId"], event.Data["refKey"], event.Data["refValue"],
		)
		if err != nil {
			if strings.Contains(err.Error(), "thread_refs_thread_id_fkey") {
				w.logger.Warn("skipping ref for non-existent thread", zap.String("thread_id", event.Data["threadId"]))
				continue
			}
			return fmt.Errorf("write thread ref: %w", err)
		}
	}

	w.logger.Info("wrote thread refs", zap.Int("count", len(events)))
	return nil
}

func (w *PostgresWriter) WriteThreadAccess(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	query := `
		INSERT INTO thread_access (
			thread_id, user_id, roles, runtime_role, permissions, granted_by, granted_at, status
		) VALUES ($1, $2, $3::jsonb, $4, $5::text[], $6, $7, $8)
		ON CONFLICT (thread_id, user_id) DO UPDATE SET
			roles        = COALESCE(NULLIF(EXCLUDED.roles, '[]'::jsonb),    thread_access.roles),
			runtime_role = COALESCE(NULLIF(EXCLUDED.runtime_role, ''),      thread_access.runtime_role),
			permissions  = COALESCE(NULLIF(EXCLUDED.permissions, '{}'),     thread_access.permissions),
			granted_by   = COALESCE(NULLIF(EXCLUDED.granted_by, ''),        thread_access.granted_by),
			granted_at   = EXCLUDED.granted_at,
			status       = EXCLUDED.status`

	var success, skipped int
	for _, event := range events {
		var permissions []string
		if permStr := event.Data["permissions"]; permStr != "" {
			if err := json.Unmarshal([]byte(permStr), &permissions); err != nil {
				w.logger.Error("failed to parse permissions", zap.Error(err))
				permissions = []string{}
			}
		}

		_, err := w.db.Pool.Exec(ctx, query,
			event.Data["threadId"], event.Data["userId"],
			event.Data["roles"], event.Data["runtime_role"],
			permissions, event.Data["grantedBy"],
			event.Data["grantedAt"], event.Data["status"],
		)
		if err != nil {
			if strings.Contains(err.Error(), "fk_thread_access_thread") || strings.Contains(err.Error(), "23503") {
				w.logger.Warn("skipping access for non-existent thread", zap.String("thread_id", event.Data["threadId"]))
				skipped++
				continue
			}
			return fmt.Errorf("write thread access: %w", err)
		}
		success++
	}

	w.logger.Info("wrote thread access events", zap.Int("success", success), zap.Int("skipped", skipped))
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
			event.Data["validationID"], event.Data["threadID"], event.Data["stepID"],
			event.Data["stepName"], event.Data["idempotencyKey"], event.Data["timestamp"],
			event.Data["validations"], event.Data["overallStatus"], event.Data["hasCriticalViolation"],
			event.Data["criticalCount"], event.Data["warningCount"], event.Data["minorCount"],
			event.Data["infoCount"], event.Data["totalValidations"],
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
			event.Data["notificationID"], event.Data["threadID"], event.Data["stepID"],
			event.Data["stepName"], nullIfEmpty(event.Data["idempotencyKey"]),
			event.Data["source"], event.Data["notificationType"],
			nullIfEmpty(event.Data["stepStatus"]), nullIfEmpty(event.Data["validationStatus"]),
			nullIfEmpty(event.Data["violationType"]), nullIfEmpty(event.Data["severity"]),
			event.Data["message"], nullIfEmpty(event.Data["details"]), event.Data["timestamp"],
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

	const cols = 13
	topLevel := map[string]bool{
		"thread_id": true, "type": true, "step_id": true, "actor": true,
		"actor_service": true, "hash": true, "prev_hash": true, "status": true,
		"content_hash": true, "timestamp": true, "started_at": true, "finished_at": true,
	}

	values := make([]interface{}, 0, len(events)*cols)
	for _, event := range events {
		ts := event.Data["timestamp"]
		if ts == "" {
			ts = time.Now().Format(time.RFC3339Nano)
		}

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

		values = append(values,
			event.Data["thread_id"], event.Data["type"], event.Data["step_id"],
			event.Data["actor"], event.Data["actor_service"], string(payloadJSON),
			ts, event.Data["hash"], event.Data["prev_hash"], event.Data["status"],
			event.Data["content_hash"],
			nullIfEmpty(event.Data["started_at"]), nullIfEmpty(event.Data["finished_at"]),
		)
	}

	query := `INSERT INTO thread_activities (
		thread_id, activity_type, step_id, actor, actor_service, payload,
		recorded_at, hash, prev_hash, status, content_hash, started_at, finished_at
	) VALUES ` + buildPlaceholders(len(events), cols)

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write activity log: %w", err)
	}

	w.logger.Info("wrote activity log events", zap.Int("count", len(events)))
	return nil
}

func (w *PostgresWriter) WriteSubSteps(ctx context.Context, subSteps []map[string]interface{}) error {
	if len(subSteps) == 0 {
		return nil
	}

	const cols = 7
	values := make([]interface{}, 0, len(subSteps)*cols)
	for _, subStep := range subSteps {
		str := func(key string) string { s, _ := subStep[key].(string); return s }

		recordedAt := str("recorded_at")
		if recordedAt == "" {
			recordedAt = time.Now().Format(time.RFC3339Nano)
		}

		payloadJSON := []byte("{}")
		if payload, ok := subStep["payload"].(map[string]interface{}); ok && len(payload) > 0 {
			payloadJSON, _ = json.Marshal(payload)
		}

		values = append(values,
			uuid.New().String(), str("thread_id"), str("step_id"),
			str("name"), str("status"), string(payloadJSON), recordedAt,
		)
	}

	query := `INSERT INTO step_substeps (
		id, thread_id, step_id, name, status, payload, recorded_at
	) VALUES ` + buildPlaceholders(len(subSteps), cols) + ` ON CONFLICT (id) DO NOTHING`

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

	seen := make(map[string]StreamEvent, len(events))
	for _, event := range events {
		if id := event.Data["step_id"]; id != "" {
			seen[id] = event
		}
	}
	if len(seen) == 0 {
		return nil
	}

	const cols = 9
	values := make([]interface{}, 0, len(seen)*cols)
	for _, event := range seen {
		values = append(values,
			event.Data["step_id"], event.Data["thread_id"], event.Data["step_name"],
			event.Data["idempotency_key"], event.Data["status"], event.Data["retry_count"],
			event.Data["first_seen_at"], event.Data["last_updated_at"], event.Data["previous_step"],
		)
	}

	query := `INSERT INTO thread_step_state (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, previous_step
	) VALUES ` + buildPlaceholders(len(seen), cols) + `
	ON CONFLICT (id) DO UPDATE SET
		status          = EXCLUDED.status,
		retry_count     = EXCLUDED.retry_count,
		last_updated_at = EXCLUDED.last_updated_at,
		previous_step   = EXCLUDED.previous_step`

	if _, err := w.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("write step state: %w", err)
	}

	w.logger.Info("wrote step state events", zap.Int("total", len(events)), zap.Int("unique", len(seen)))
	return nil
}
