package archiver

import (
	"context"
	"encoding/json"
	"fmt"

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
		INSERT INTO step_events (
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
		contextJSON, _ := json.Marshal(event.Data["context"])

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
	if len(events) == 0 {
		return nil
	}

	// Upsert thread metadata
	query := `
		INSERT INTO threads (
			id, owner_id, company_id, contract_id, contract_version,
			contract_name, status, current_step, last_hash, started_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			current_step = EXCLUDED.current_step,
			last_hash = EXCLUDED.last_hash,
			completed_at = EXCLUDED.completed_at
	`

	for _, event := range events {
		_, err := w.db.Pool.Exec(ctx, query,
			event.Data["id"],
			event.Data["ownerId"],
			event.Data["companyId"],
			event.Data["contractId"],
			event.Data["contractVersion"],
			event.Data["contractName"],
			event.Data["status"],
			event.Data["currentStep"],
			event.Data["lastHash"],
			event.Data["startedAt"],
			event.Data["completedAt"],
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteAuditLogs writes audit logs to Postgres
func (w *PostgresWriter) WriteAuditLogs(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	query := `
		INSERT INTO audit_logs (
			id, thread_id, user_id, action, details, timestamp
		) VALUES `

	values := make([]interface{}, 0, len(events)*6)
	placeholders := ""

	for i, event := range events {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 6
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6,
		)

		values = append(values,
			event.Data["id"],
			event.Data["threadId"],
			event.Data["userId"],
			event.Data["action"],
			event.Data["details"],
			event.Data["timestamp"],
		)
	}

	query += placeholders

	_, err := w.db.Pool.Exec(ctx, query, values...)
	return err
}

// WriteInvitations writes invitations to Postgres
func (w *PostgresWriter) WriteInvitations(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	query := `
		INSERT INTO invitations (
			id, thread_id, inviter_id, invitee_email, role, permissions, status, created_at, expires_at
		) VALUES `

	values := make([]interface{}, 0, len(events)*9)
	placeholders := ""

	for i, event := range events {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 9
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8, offset+9,
		)

		values = append(values,
			event.Data["id"],
			event.Data["threadId"],
			event.Data["inviterId"],
			event.Data["inviteeEmail"],
			event.Data["role"],
			event.Data["permissions"],
			event.Data["status"],
			event.Data["createdAt"],
			event.Data["expiresAt"],
		)
	}

	query += placeholders + " ON CONFLICT (id) DO NOTHING"

	_, err := w.db.Pool.Exec(ctx, query, values...)
	return err
}
