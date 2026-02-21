package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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

	start := time.Now()
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
			fmt.Printf("[PostgresWriter] Failed to marshal context for step %s: %v\n", event.Data["stepId"], err)
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
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("[PostgresWriter] Failed to write step events: %v\n", err)
		DatabaseWritesTotal.WithLabelValues("thread_activities", "error").Inc()
		DatabaseWriteDuration.WithLabelValues("thread_activities").Observe(duration.Seconds())
		return err
	}

	DatabaseWritesTotal.WithLabelValues("thread_activities", "success").Inc()
	DatabaseWriteDuration.WithLabelValues("thread_activities").Observe(duration.Seconds())
	fmt.Printf("[PostgresWriter] Successfully wrote %d step events to Postgres in %v\n", len(events), duration)
	return nil
}

// WriteThreadMetadata writes thread metadata to Postgres
func (w *PostgresWriter) WriteThreadMetadata(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Separate ref events from thread metadata events
	refEvents := make([]StreamEvent, 0)
	metadataEvents := make([]StreamEvent, 0)

	for _, event := range events {
		if event.Data["action"] == "ref_added" {
			refEvents = append(refEvents, event)
		} else {
			metadataEvents = append(metadataEvents, event)
		}
	}

	// Process ref events separately
	if len(refEvents) > 0 {
		if err := w.WriteThreadRefs(ctx, refEvents); err != nil {
			return err
		}
	}

	// Process thread metadata events
	if len(metadataEvents) == 0 {
		return nil
	}

	for _, event := range metadataEvents {
		// Handle thread completion/cancellation events separately
		if status := event.Data["status"]; status == "completed" || status == "cancelled" {
			// Update existing thread status and company_id (in case thread was created before company sync)
			updateQuery := `
				UPDATE threads 
				SET status = $1, 
				    completed_at = $2,
				    updated_at = $2,
				    company_id = COALESCE($4, company_id)
				WHERE id = $3
			`
			completedAt := event.Data["completedAt"]
			if completedAt == "" {
				completedAt = event.Data["startedAt"] // Fallback
			}

			// Get company_id if present (may be empty for old events)
			var companyIDParam interface{} = nil
			if companyID := event.Data["companyId"]; companyID != "" {
				companyIDParam = companyID
			}

			_, err := w.db.Pool.Exec(ctx, updateQuery, status, completedAt, event.Data["thread_id"], companyIDParam)
			if err != nil {
				fmt.Printf("[WARN] Failed to update thread status for %s: %v\n", event.Data["thread_id"], err)
			} else {
				fmt.Printf("[ARCHIVER-UPDATE] Updated thread %s status to %s\n", event.Data["thread_id"], status)
			}
			continue
		}

		// Handle missing fields with defaults
		error := ""
		if event.Data["error"] != "" {
			error = event.Data["error"]
		}

		// Validate owner_id exists in service_accounts table, set to NULL if not
		// Note: owner_id is always a service account ID (threads are created via API keys)
		var ownerIDParam interface{} = event.Data["ownerId"]
		if ownerID := event.Data["ownerId"]; ownerID != "" {
			var exists bool
			checkQuery := `SELECT EXISTS(SELECT 1 FROM service_accounts WHERE id = $1)`
			err := w.db.Pool.QueryRow(ctx, checkQuery, ownerID).Scan(&exists)
			if err != nil || !exists {
				ownerIDParam = nil // Set to nil which will be NULL in PostgreSQL
			}
		} else {
			ownerIDParam = nil // Empty string should also be NULL
		}

		// Validate company_id exists in companies table
		companyID := event.Data["companyId"]
		if companyID == "" {
			fmt.Printf("❌ [ARCHIVER-ERROR] Missing company_id for thread %s. Stream data: %+v\n", event.Data["threadId"], event.Data)
			return fmt.Errorf("missing company_id for thread %s", event.Data["threadId"])
		}

		var companyExists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1)`
		err := w.db.Pool.QueryRow(ctx, checkQuery, companyID).Scan(&companyExists)
		if err != nil {
			fmt.Printf("❌ [ARCHIVER-ERROR] Failed to check company existence: %v\n", err)
			return fmt.Errorf("failed to validate company_id: %w", err)
		}
		if !companyExists {
			fmt.Printf("❌ [ARCHIVER-ERROR] Company ID '%s' not found in companies table for thread %s\n", companyID, event.Data["threadId"])
			fmt.Printf("   Thread data: threadId=%s, ownerId=%s, companyId=%s\n", event.Data["threadId"], event.Data["ownerId"], companyID)
			return fmt.Errorf("company_id '%s' does not exist in companies table", companyID)
		}

		// Handle contract_version (integer field) - set to NULL if empty
		var contractVersionParam interface{} = nil
		if contractVersion := event.Data["contractVersion"]; contractVersion != "" {
			contractVersionParam = contractVersion
		}

		// Insert/update thread creation event
		query := `
			INSERT INTO threads (
				id, company_id, contract_id, contract_name, contract_version, owner_id, error, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				company_id = EXCLUDED.company_id,
				contract_id = EXCLUDED.contract_id,
				contract_name = EXCLUDED.contract_name,
				contract_version = EXCLUDED.contract_version,
				owner_id = EXCLUDED.owner_id,
				error = EXCLUDED.error,
				updated_at = EXCLUDED.updated_at
		`

		_, err = w.db.Pool.Exec(ctx, query,
			event.Data["threadId"], // Use threadId instead of id
			companyID,              // Required - validated above
			event.Data["contractId"],
			event.Data["contractName"], // Add contract_name from stream data
			contractVersionParam,       // Use NULL if empty (integer field)
			ownerIDParam,               // Use validated owner_id (NULL if user doesn't exist)
			error,                      // Error field (optional)
			event.Data["startedAt"],    // Use startedAt as created_at
			event.Data["startedAt"],    // Use startedAt as updated_at for new records
		)
		if err != nil {
			return err
		}
	}

	fmt.Printf("SUCCESS: Successfully wrote %d thread metadata records to Postgres\n", len(metadataEvents))
	return nil
}

// WriteThreadRefs writes thread refs to Postgres
func (w *PostgresWriter) WriteThreadRefs(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	fmt.Printf("🔗 Writing %d thread refs to Postgres\n", len(events))

	// Upsert thread refs
	query := `
		INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (thread_id, ref_key) DO UPDATE SET
			ref_value = EXCLUDED.ref_value,
			updated_at = NOW()
	`

	for _, event := range events {
		_, err := w.db.Pool.Exec(ctx, query,
			event.Data["threadId"],
			event.Data["refKey"],
			event.Data["refValue"],
		)
		if err != nil {
			fmt.Printf("ERROR: Failed to write thread ref to Postgres: %v\n", err)
			return err
		}
	}

	fmt.Printf("SUCCESS: Successfully wrote %d thread refs to Postgres\n", len(events))
	return nil
}

// WriteThreadAccess writes thread access grants to Postgres
func (w *PostgresWriter) WriteThreadAccess(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Upsert thread access records
	// Use COALESCE to only update non-empty fields (allows separate role and runtime_role updates)
	query := `
		INSERT INTO thread_access (
			thread_id, user_id, roles, runtime_role, permissions, granted_by, granted_at, status
		) VALUES ($1, $2, $3::jsonb, $4, $5::text[], $6, $7, $8)
		ON CONFLICT (thread_id, user_id) DO UPDATE SET
			roles = COALESCE(NULLIF(EXCLUDED.roles, '[]'::jsonb), thread_access.roles),
			runtime_role = COALESCE(NULLIF(EXCLUDED.runtime_role, ''::text), thread_access.runtime_role),
			permissions = COALESCE(NULLIF(EXCLUDED.permissions, '{}'::text[]), thread_access.permissions),
			granted_by = COALESCE(NULLIF(EXCLUDED.granted_by, ''::text), thread_access.granted_by),
			granted_at = EXCLUDED.granted_at,
			status = EXCLUDED.status
	`

	successCount := 0
	skippedCount := 0

	for _, event := range events {
		fmt.Printf("🔍 [PostgresWriter] Processing thread_access event: threadId=%s, userId=%s, roles=%s, runtime_role=%s, permissions=%s\n",
			event.Data["threadId"], event.Data["userId"], event.Data["roles"], event.Data["runtime_role"], event.Data["permissions"])

		// Parse permissions from JSON string to []string
		var permissions []string
		if permStr := event.Data["permissions"]; permStr != "" {
			if err := json.Unmarshal([]byte(permStr), &permissions); err != nil {
				fmt.Printf("[PostgresWriter] Failed to parse permissions JSON: %v\n", err)
				permissions = []string{} // Use empty array on parse error
			}
		}

		_, err := w.db.Pool.Exec(ctx, query,
			event.Data["threadId"],
			event.Data["userId"],
			event.Data["roles"],
			event.Data["runtime_role"], // Fixed: use snake_case to match NATS publisher
			permissions,                // Use parsed []string instead of JSON string
			event.Data["grantedBy"],
			event.Data["grantedAt"],
			event.Data["status"],
		)
		if err != nil {
			// Check if it's a foreign key violation (thread doesn't exist yet)
			if strings.Contains(err.Error(), "fk_thread_access_thread") || strings.Contains(err.Error(), "23503") {
				fmt.Printf("[PostgresWriter] Skipping thread_access for non-existent thread %s (will retry on next batch)\n", event.Data["threadId"])
				skippedCount++
				// Don't return error - just skip this record, it will be redelivered by NATS
				continue
			}
			fmt.Printf("[PostgresWriter] Failed to write thread_access event: %v\n", err)
			return err
		}
		successCount++
	}

	if skippedCount > 0 {
		fmt.Printf("[PostgresWriter] Wrote %d thread access events (%d skipped due to missing threads)\n", successCount, skippedCount)
	} else {
		fmt.Printf("[PostgresWriter] Successfully wrote %d thread access events\n", successCount)
	}
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
		fmt.Printf("[PostgresWriter] Failed to write validation results: %v\n", err)
		return err
	}

	fmt.Printf("[PostgresWriter] Successfully wrote %d validation results to Postgres\n", len(events))
	return nil
}

// WriteThreadNotifications writes individual thread notifications to Postgres
func (w *PostgresWriter) WriteThreadNotifications(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	fmt.Printf("📝 [PostgresWriter] Writing %d thread notifications to Postgres...\n", len(events))

	// Use multi-row INSERT for efficiency
	query := `
		INSERT INTO thread_notifications (
			notification_id, thread_id, step_id, step_name, idempotency_key,
			source, notification_type, step_status, validation_status,
			violation_type, severity, message, details, timestamp
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

		// Handle optional fields (use NULL if empty)
		var idempotencyKey interface{} = event.Data["idempotencyKey"]
		if idempotencyKey == "" {
			idempotencyKey = nil
		}

		var stepStatus interface{} = event.Data["stepStatus"]
		if stepStatus == "" {
			stepStatus = nil
		}

		var validationStatus interface{} = event.Data["validationStatus"]
		if validationStatus == "" {
			validationStatus = nil
		}

		var violationType interface{} = event.Data["violationType"]
		if violationType == "" {
			violationType = nil
		}

		var severity interface{} = event.Data["severity"]
		if severity == "" {
			severity = nil
		}

		var details interface{} = event.Data["details"]
		if details == "" {
			details = nil
		}

		values = append(values,
			event.Data["notificationID"],
			event.Data["threadID"],
			event.Data["stepID"],
			event.Data["stepName"],
			idempotencyKey,
			event.Data["source"],
			event.Data["notificationType"],
			stepStatus,
			validationStatus,
			violationType,
			severity,
			event.Data["message"],
			details,
			event.Data["timestamp"],
		)

		fmt.Printf("   Notification %d: notificationID=%s, source=%s, type=%s, severity=%s\n",
			i+1, event.Data["notificationID"], event.Data["source"], event.Data["notificationType"], severity)
	}

	query += placeholders + " ON CONFLICT (notification_id) DO NOTHING"

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("[PostgresWriter] Failed to write thread notifications: %v\n", err)
		return err
	}

	fmt.Printf("[PostgresWriter] Successfully wrote %d thread notifications to Postgres\n", len(events))
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
			thread_id, activity_type, step_id, actor, actor_service, payload, recorded_at, hash, prev_hash, status, content_hash, started_at, finished_at
		) VALUES `

	values := make([]interface{}, 0, len(events)*13)
	placeholders := ""

	for i, event := range events {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 13
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d::jsonb, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8, offset+9, offset+10, offset+11, offset+12, offset+13,
		)

		// Extract fields from event data
		threadID := event.Data["thread_id"]
		activityType := event.Data["type"]
		stepID := event.Data["step_id"]
		actor := event.Data["actor"]
		actorService := event.Data["actor_service"]
		hash := event.Data["hash"]
		prevHash := event.Data["prev_hash"]
		status := event.Data["status"]
		contentHash := event.Data["content_hash"]
		timestamp := event.Data["timestamp"]
		startedAt := event.Data["started_at"]
		finishedAt := event.Data["finished_at"]

		// Handle empty timestamp - use current time as fallback
		if timestamp == "" {
			timestamp = time.Now().Format(time.RFC3339Nano)
		}

		// Create payload with all event data except the top-level fields
		payload := make(map[string]interface{})
		for k, v := range event.Data {
			if k != "thread_id" && k != "type" && k != "step_id" && k != "actor" && k != "actor_service" && k != "hash" && k != "prev_hash" && k != "status" && k != "content_hash" && k != "timestamp" && k != "started_at" && k != "finished_at" {
				payload[k] = v
			}
		}

		// Convert payload to JSON
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		// Add values to the slice (use nil for empty timestamps)
		var startedAtVal interface{} = nil
		if startedAt != "" {
			startedAtVal = startedAt
		}
		var finishedAtVal interface{} = nil
		if finishedAt != "" {
			finishedAtVal = finishedAt
		}

		values = append(values, threadID, activityType, stepID, actor, actorService, string(payloadJSON), timestamp, hash, prevHash, status, contentHash, startedAtVal, finishedAtVal)
	}

	query += placeholders

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("[PostgresWriter] Failed to write thread activities: %v\n", err)
		return err
	}

	fmt.Printf("[PostgresWriter] Successfully wrote %d activity log events to thread_activities\n", len(events))
	return nil
}

// WriteSubSteps writes sub-steps to Postgres (batched)
func (w *PostgresWriter) WriteSubSteps(ctx context.Context, subSteps []map[string]interface{}) error {
	if len(subSteps) == 0 {
		return nil
	}

	fmt.Printf("📝 [PostgresWriter] Writing %d sub-steps to step_substeps (batched)...\n", len(subSteps))

	query := `
		INSERT INTO step_substeps (
			id, thread_id, step_id, name, status, payload, recorded_at
		) VALUES `

	values := make([]interface{}, 0, len(subSteps)*7)
	placeholders := ""

	for i, subStep := range subSteps {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 7
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d::jsonb, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7,
		)

		// Extract fields
		threadID, _ := subStep["thread_id"].(string)
		stepID, _ := subStep["step_id"].(string)
		name, _ := subStep["name"].(string)
		status, _ := subStep["status"].(string)
		recordedAt, _ := subStep["recorded_at"].(string)

		// Generate UUID for sub-step
		subStepID := uuid.New().String()

		// Handle payload
		var payloadJSON []byte
		if payload, ok := subStep["payload"].(map[string]interface{}); ok && len(payload) > 0 {
			payloadJSON, _ = json.Marshal(payload)
		} else {
			payloadJSON = []byte("{}")
		}

		// Handle empty timestamp
		if recordedAt == "" {
			recordedAt = time.Now().Format(time.RFC3339Nano)
		}

		values = append(values, subStepID, threadID, stepID, name, status, string(payloadJSON), recordedAt)
	}

	query += placeholders + " ON CONFLICT (id) DO NOTHING"

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("[PostgresWriter] Failed to write sub-steps: %v\n", err)
		return err
	}

	fmt.Printf("[PostgresWriter] Successfully wrote %d sub-steps to step_substeps\n", len(subSteps))
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
		fmt.Printf("[PostgresWriter] No new step states to write (all were duplicates)\n")
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

	if len(deduped) > 0 {
		fmt.Printf("🔍 [PostgresWriter] SQL Query (first 500 chars): %s...\n", query[:min(500, len(query))])
		fmt.Printf("🔍 [PostgresWriter] Values count: %d, Expected: %d\n", len(values), len(deduped)*9)
	}

	_, err := w.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		fmt.Printf("[PostgresWriter] Failed to write thread step state: %v\n", err)
		fmt.Printf("[PostgresWriter] Full query: %s\n", query)
		return err
	}

	fmt.Printf("[PostgresWriter] Successfully wrote %d thread step state events\n", len(events))
	return nil
}
