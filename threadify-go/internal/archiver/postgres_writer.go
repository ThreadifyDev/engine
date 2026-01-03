package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/database"
)

// PostgresWriter handles writing events to Postgres
type PostgresWriter struct {
	db *database.PostgresDB
}

// NewPostgresWriter creates a new Postgres writer
func NewPostgresWriter(db *database.PostgresDB) *PostgresWriter {
	return &PostgresWriter{db: db}
}

// WriteStepEvents writes step events to Postgres
func (w *PostgresWriter) WriteStepEvents(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	fmt.Printf("📝 [PostgresWriter] Writing %d step events to Postgres...\n", len(events))
	for i, event := range events {
		fmt.Printf("   Event %d: stepId=%s, threadId=%s, stepName=%s, status=%s\n",
			i+1, event.Data["stepId"], event.Data["threadId"], event.Data["stepName"], event.Data["status"])
	}

	// Use multi-row INSERT for efficiency
	query := `
		INSERT INTO thread_activities (
			step_id, thread_id, step_name, service_name, type, status,
			context, started_at, finished_at, timestamp, hash, prev_hash
		) VALUES `

	values := make([]interface{}, 0, len(events)*12)
	placeholders := ""

	for i, event := range events {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 12
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6,
			offset+7, offset+8, offset+9, offset+10, offset+11, offset+12,
		)

		// Extract values from event data
		contextJSON, err := json.Marshal(event.Data["context"])
		if err != nil {
			fmt.Printf("❌ [PostgresWriter] Failed to marshal context for step %s: %v\n", event.Data["stepId"], err)
			return fmt.Errorf("failed to marshal context: %w", err)
		}

		values = append(values,
			event.Data["stepId"],
			event.Data["threadId"],
			event.Data["stepName"],
			event.Data["serviceName"],
			event.Data["type"],
			event.Data["status"],
			string(contextJSON),
			event.Data["startedAt"],
			event.Data["finishedAt"],
			event.Data["timestamp"],
			event.Data["hash"],
			event.Data["prevHash"],
		)
	}

	query += placeholders + " ON CONFLICT (step_id) DO NOTHING"

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("❌ [PostgresWriter] Failed to write step events: %v\n", err)
		return err
	}

	fmt.Printf("✅ [PostgresWriter] Successfully wrote %d step events to Postgres\n", len(events))
	return nil
}

// WriteThreadMetadata writes thread metadata to Postgres
func (w *PostgresWriter) WriteThreadMetadata(ctx context.Context, events []StreamEvent) error {
	fmt.Printf("🔄 DEBUG: WriteThreadMetadata called with %d events\n", len(events))

	if len(events) == 0 {
		fmt.Printf(" DEBUG: WriteThreadMetadata - no events, returning\n")
		return nil
	}

	fmt.Printf(" DEBUG: Writing thread metadata for %d events\n", len(events))
	for i, event := range events {
		fmt.Printf("   Event %d: threadId=%s, ownerId=%s, contractId=%s\n",
			i+1, event.Data["threadId"], event.Data["ownerId"], event.Data["contractId"])
	}

	// Upsert thread metadata
	query := `
		INSERT INTO threads (
			id, company_id, contract_id, contract_version, owner_id, error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			company_id = EXCLUDED.company_id,
			contract_id = EXCLUDED.contract_id,
			contract_version = EXCLUDED.contract_version,
			error = EXCLUDED.error,
			updated_at = EXCLUDED.updated_at
	`

	for _, event := range events {
		// Handle missing fields with defaults
		error := ""
		if event.Data["error"] != "" {
			error = event.Data["error"]
		}
		fmt.Printf("ContractVersion: %s\n", event.Data["contractVersion"])

		_, err := w.db.Pool.Exec(ctx, query,
			event.Data["threadId"],  // Use threadId instead of id
			event.Data["companyId"], // Add company_id from stream data
			event.Data["contractId"],
			event.Data["contractVersion"],
			event.Data["ownerId"],
			error,                   // Error field (optional)
			event.Data["startedAt"], // Use startedAt as created_at
			event.Data["startedAt"], // Use startedAt as updated_at for new records
		)
		if err != nil {
			fmt.Printf("❌ ERROR: Failed to write thread metadata to Postgres: %v\n", err)
			return err
		}
	}

	fmt.Printf("✅ SUCCESS: Successfully wrote %d thread metadata records to Postgres\n", len(events))
	return nil
}

// WriteThreadAccess writes thread access grants to Postgres
func (w *PostgresWriter) WriteThreadAccess(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Upsert thread access records
	// Use COALESCE to only update non-empty fields (allows separate role and permission updates)
	query := `
		INSERT INTO thread_access (
			thread_id, user_id, roles, permissions, granted_by, granted_at, status
		) VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7)
		ON CONFLICT (thread_id, user_id) DO UPDATE SET
			roles = COALESCE(NULLIF(EXCLUDED.roles, '[]'::jsonb), thread_access.roles),
			permissions = COALESCE(NULLIF(EXCLUDED.permissions, ''::text), thread_access.permissions),
			granted_by = COALESCE(NULLIF(EXCLUDED.granted_by, ''::text), thread_access.granted_by),
			granted_at = EXCLUDED.granted_at,
			status = EXCLUDED.status
	`

	for _, event := range events {
		fmt.Printf("🔍 [PostgresWriter] Processing thread_access event: threadId=%s, userId=%s, roles=%s\n",
			event.Data["threadId"], event.Data["userId"], event.Data["roles"])

		_, err := w.db.Pool.Exec(ctx, query,
			event.Data["threadId"],
			event.Data["userId"],
			event.Data["roles"],
			event.Data["permissions"],
			event.Data["grantedBy"],
			event.Data["grantedAt"],
			event.Data["status"],
		)
		if err != nil {
			fmt.Printf("❌ [PostgresWriter] Failed to write thread_access event: %v\n", err)
			return err
		}
	}

	fmt.Printf("✅ [PostgresWriter] Successfully wrote %d thread access events\n", len(events))
	return nil
}

// WriteValidationResults writes validation results to Postgres
func (w *PostgresWriter) WriteValidationResults(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	fmt.Printf("📝 [PostgresWriter] Writing %d validation results to Postgres...\n", len(events))

	// Use multi-row INSERT for efficiency
	query := `
		INSERT INTO thread_validations (
			validation_id, thread_id, step_id, step_name, idempotency_key,
			timestamp, validations, overall_status, has_critical_violation,
			critical_count, warning_count, minor_count, info_count, total_validations
		) VALUES `

	values := make([]interface{}, 0, len(events)*14)
	placeholders := ""

	for i, event := range events {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 14
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7,
			offset+8, offset+9, offset+10, offset+11, offset+12, offset+13, offset+14,
		)

		values = append(values,
			event.Data["validationID"],
			event.Data["threadID"],
			event.Data["stepID"],
			event.Data["stepName"],
			event.Data["idempotencyKey"],
			event.Data["timestamp"],
			event.Data["validations"],
			event.Data["overallStatus"],
			event.Data["hasCriticalViolation"],
			event.Data["criticalCount"],
			event.Data["warningCount"],
			event.Data["minorCount"],
			event.Data["infoCount"],
			event.Data["totalValidations"],
		)

		fmt.Printf("   Validation %d: validationID=%s, threadID=%s, stepID=%s, status=%s, critical=%v\n",
			i+1, event.Data["validationID"], event.Data["threadID"], event.Data["stepID"],
			event.Data["overallStatus"], event.Data["hasCriticalViolation"])
	}

	query += placeholders + " ON CONFLICT (validation_id) DO NOTHING"

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("❌ [PostgresWriter] Failed to write validation results: %v\n", err)
		return err
	}

	fmt.Printf("✅ [PostgresWriter] Successfully wrote %d validation results to Postgres\n", len(events))
	return nil
}

// WriteActivityLog writes activity log events to Postgres (batched)
func (w *PostgresWriter) WriteActivityLog(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	fmt.Printf("📝 [PostgresWriter] Writing %d activity events to thread_activities (batched)...\n", len(events))

	query := `
		INSERT INTO thread_activities (
			thread_id, activity_type, step_id, actor, actor_service, payload, recorded_at, hash
		) VALUES `

	values := make([]interface{}, 0, len(events)*8)
	placeholders := ""

	for i, event := range events {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 8
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d::jsonb, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8,
		)

		// Extract fields from event data
		threadID := event.Data["thread_id"]
		activityType := event.Data["type"]
		stepID := event.Data["step_id"]
		actor := event.Data["actor"]
		actorService := event.Data["actor_service"]
		hash := event.Data["hash"]
		timestamp := event.Data["timestamp"]
		// Handle empty timestamp - use current time as fallback
		if timestamp == "" {
			timestamp = time.Now().Format(time.RFC3339)
		}

		// Create payload with all event data except the top-level fields
		payload := make(map[string]interface{})
		for k, v := range event.Data {
			if k != "thread_id" && k != "type" && k != "step_id" && k != "actor" && k != "actor_service" && k != "hash" && k != "timestamp" {
				payload[k] = v
			}
		}

		// Parse context JSON if present
		if contextJSON, ok := payload["context"]; ok && contextJSON != "" {
			// Try to parse as JSON first
			var contextMap map[string]interface{}
			if err := json.Unmarshal([]byte(contextJSON.(string)), &contextMap); err == nil {
				payload["context"] = contextMap
			}
		}

		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("❌ Failed to marshal payload: %v\n", err)
			continue
		}

		values = append(values, threadID, activityType, stepID, actor, actorService, string(payloadJSON), timestamp, hash)
	}

	query += placeholders

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("❌ [PostgresWriter] Failed to write thread activities: %v\n", err)
		return err
	}

	fmt.Printf("✅ [PostgresWriter] Successfully wrote %d activity log events to thread_activities\n", len(events))
	return nil
}

// WriteThreadStepState writes step state snapshots to Postgres (batched)
func (w *PostgresWriter) WriteThreadStepState(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	fmt.Printf("📝 [PostgresWriter] Writing %d thread step state events (batched)...\n", len(events))

	// Deduplicate by step_id - keep only the latest event for each step_id
	eventMap := make(map[string]StreamEvent)
	for _, event := range events {
		stepID := event.Data["step_id"]
		if stepID == "" {
			// Skip if step_id is empty
			continue
		}
		eventMap[stepID] = event
	}

	// Convert back to slice
	deduped := make([]StreamEvent, 0, len(eventMap))
	for _, event := range eventMap {
		deduped = append(deduped, event)
	}

	fmt.Printf("📝 [PostgresWriter] Deduplicated to %d unique step states\n", len(deduped))

	// If all events were deduplicated away, nothing to do
	if len(deduped) == 0 {
		fmt.Printf("✅ [PostgresWriter] No new step states to write (all were duplicates)\n")
		return nil
	}

	query := `
		INSERT INTO thread_step_state (
			id, thread_id, step_name, idempotency_key, status, 
			retry_count, first_seen_at, last_updated_at, previous_step
		) VALUES `

	values := make([]interface{}, 0, len(deduped)*9)
	placeholders := ""

	for i, event := range deduped {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 9
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8, offset+9,
		)

		values = append(values,
			event.Data["step_id"],
			event.Data["thread_id"],
			event.Data["step_name"],
			event.Data["idempotency_key"],
			event.Data["status"],
			event.Data["retry_count"],
			event.Data["first_seen_at"],
			event.Data["last_updated_at"],
			event.Data["previous_step"],
		)
	}

	query += placeholders + ` 
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			retry_count = EXCLUDED.retry_count,
			last_updated_at = EXCLUDED.last_updated_at,
			previous_step = EXCLUDED.previous_step`

	// Debug: Log the query structure
	if len(deduped) > 0 {
		fmt.Printf("🔍 [PostgresWriter] SQL Query (first 500 chars): %s...\n", query[:min(500, len(query))])
		fmt.Printf("🔍 [PostgresWriter] Values count: %d, Expected: %d\n", len(values), len(deduped)*9)
	}

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("❌ [PostgresWriter] Failed to write thread step state: %v\n", err)
		fmt.Printf("❌ [PostgresWriter] Full query: %s\n", query)
		return err
	}

	fmt.Printf("✅ [PostgresWriter] Successfully wrote %d thread step state events\n", len(events))
	return nil
}
