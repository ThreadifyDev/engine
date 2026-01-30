package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/models"
)

// ActivityRepository handles activity log retrieval from PostgreSQL
type ActivityRepository struct {
	pool   *pgxpool.Pool
	config *config.Config
}

// NewActivityRepository creates a new PostgreSQL activity repository
func NewActivityRepository(pool *pgxpool.Pool, cfg *config.Config) *ActivityRepository {
	return &ActivityRepository{
		pool:   pool,
		config: cfg,
	}
}

// GetActivityLog retrieves activity log from thread_activities table
// Returns activities as map[string]interface{} to match Valkey format
func (r *ActivityRepository) GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			id,
			activity_type,
			thread_id,
			step_id,
			user_id,
			actor,
			actor_service,
			role,
			permissions,
			granted_by,
			granted_at,
			method,
			payload,
			recorded_at
		FROM thread_activities
		WHERE thread_id = $1
		ORDER BY recorded_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query activity log: %w", err)
	}
	defer rows.Close()

	var activities []map[string]interface{}

	for rows.Next() {
		var id, activityType, threadIDVal string
		var stepID, userID, actor, actorService, role, permissions, grantedBy, grantedAt, method sql.NullString
		var payload sql.NullString
		var recordedAt sql.NullTime

		err := rows.Scan(
			&id,
			&activityType,
			&threadIDVal,
			&stepID,
			&userID,
			&actor,
			&actorService,
			&role,
			&permissions,
			&grantedBy,
			&grantedAt,
			&method,
			&payload,
			&recordedAt,
		)

		if err != nil {
			continue // Skip invalid entries
		}

		// Build activity map
		activity := map[string]interface{}{
			"id":        id,
			"type":      activityType,
			"thread_id": threadIDVal,
		}

		// Add optional fields
		if stepID.Valid {
			activity["step_id"] = stepID.String
		}
		if userID.Valid {
			activity["user_id"] = userID.String
		}
		if actor.Valid {
			activity["actor"] = actor.String
		}
		if actorService.Valid {
			activity["actor_service"] = actorService.String
		}
		if role.Valid {
			activity["role"] = role.String
		}
		if permissions.Valid {
			activity["permissions"] = permissions.String
		}
		if grantedBy.Valid {
			activity["granted_by"] = grantedBy.String
		}
		if grantedAt.Valid {
			activity["granted_at"] = grantedAt.String
		}
		if method.Valid {
			activity["method"] = method.String
		}
		if recordedAt.Valid {
			activity["timestamp"] = recordedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}

		// Parse payload if present
		if payload.Valid && payload.String != "" {
			var payloadData map[string]interface{}
			if err := json.Unmarshal([]byte(payload.String), &payloadData); err == nil {
				// Merge payload fields into activity
				for k, v := range payloadData {
					if _, exists := activity[k]; !exists {
						activity[k] = v
					}
				}
			}
		}

		activities = append(activities, activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return activities, nil
}

// VerifyActivityChain verifies the cryptographic integrity of a thread's activity log hash chain
// Returns status with details about verification, including any tampering detected
func (r *ActivityRepository) VerifyActivityChain(ctx context.Context, threadID string) (*models.HashChainStatus, error) {
	status := &models.HashChainStatus{
		Verified:       true,
		LastVerifiedAt: time.Now(),
	}

	query := `
		SELECT 
			hash, 
			prev_hash, 
			thread_id,
			payload->>'step_uuid' as step_uuid,
			payload->>'idempotency_key' as idempotency_key,
			recorded_at
		FROM thread_activities 
		WHERE thread_id = $1 
		AND activity_type = 'step_recorded'
		ORDER BY recorded_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query activity chain: %w", err)
	}
	defer rows.Close()

	var prevHash string
	eventCount := 0

	for rows.Next() {
		var storedHash, storedPrevHash, tid, stepUUID, idempKey sql.NullString
		var recordedAt time.Time

		if err := rows.Scan(&storedHash, &storedPrevHash, &tid, &stepUUID, &idempKey, &recordedAt); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		eventCount++

		// Handle NULL values
		if !storedHash.Valid || !tid.Valid {
			continue
		}

		// Check hash format
		if !strings.HasPrefix(storedHash.String, "hmac-sha256-") {
			// Old format (sha256:...) - can't verify, mark as unverified
			// No error, no brokenAt - just unverifiable
			status.Verified = false
			status.TotalEvents = eventCount
			return status, nil
		}

		// Parse version from new format (e.g., "hmac-sha256-v1:abc...")
		parts := strings.Split(storedHash.String, ":")
		if len(parts) != 2 {
			status.Verified = false
			status.TotalEvents = eventCount
			return status, nil
		}

		version := strings.TrimPrefix(parts[0], "hmac-sha256-")
		secret := r.config.Security.HashChainSecrets[version]

		if secret == "" {
			// Secret version not found - can't verify
			status.Verified = false
			status.TotalEvents = eventCount
			return status, nil
		}

		// Recalculate HMAC with the secret for this version
		h := hmac.New(sha256.New, []byte(secret))
		hashData := fmt.Sprintf("%s:%s:%s:%s:%s",
			prevHash,
			tid.String,
			stepUUID.String,
			idempKey.String,
			recordedAt.Format(time.RFC3339))
		h.Write([]byte(hashData))
		expectedHash := fmt.Sprintf("hmac-sha256-%s:%x", version, h.Sum(nil))

		// Verify stored hash matches calculated hash
		if storedHash.String != expectedHash {
			// Actual tampering detected
			status.Verified = false
			status.BrokenAt = &recordedAt
			errMsg := "Hash tampering detected"
			status.Error = &errMsg
			status.TotalEvents = eventCount
			return status, nil
		}

		// Verify prev_hash links to previous event
		prevHashStr := ""
		if storedPrevHash.Valid {
			prevHashStr = storedPrevHash.String
		}

		if prevHashStr != prevHash {
			// Chain broken
			status.Verified = false
			status.BrokenAt = &recordedAt
			errMsg := "Chain broken (missing/reordered event)"
			status.Error = &errMsg
			status.TotalEvents = eventCount
			return status, nil
		}

		prevHash = storedHash.String
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	status.TotalEvents = eventCount
	return status, nil
}

// VerifyStepHash verifies the cryptographic integrity of a single step
// Returns true if the step's hash is valid, false otherwise
func (r *ActivityRepository) VerifyStepHash(ctx context.Context, threadID, stepName, idempotencyKey string) (bool, string, error) {
	query := `
		SELECT 
			hash, 
			prev_hash, 
			payload->>'step_uuid' as step_uuid,
			payload->>'idempotency_key' as idempotency_key,
			recorded_at
		FROM thread_activities 
		WHERE thread_id = $1 
		AND activity_type = 'step_recorded'
		AND payload->>'step_name' = $2
		AND payload->>'idempotency_key' = $3
		ORDER BY recorded_at DESC
		LIMIT 1
	`

	var storedHash, storedPrevHash, stepUUID, idempKey sql.NullString
	var recordedAt time.Time

	err := r.pool.QueryRow(ctx, query, threadID, stepName, idempotencyKey).Scan(
		&storedHash, &storedPrevHash, &stepUUID, &idempKey, &recordedAt,
	)
	if err != nil {
		// Step not found in activity log - can't verify
		return false, "Step not found in activity log", nil
	}

	// Handle NULL values
	if !storedHash.Valid {
		return false, "Hash not found", nil
	}

	// Check hash format
	if !strings.HasPrefix(storedHash.String, "hmac-sha256-") {
		// Old format - can't verify
		return false, "", nil // No error, just unverifiable
	}

	// Parse version from hash format
	parts := strings.Split(storedHash.String, ":")
	if len(parts) != 2 {
		return false, "Invalid hash format", nil
	}

	version := strings.TrimPrefix(parts[0], "hmac-sha256-")
	secret := r.config.Security.HashChainSecrets[version]

	if secret == "" {
		// Secret version not found
		return false, "", nil // No error, just unverifiable
	}

	// Get previous hash
	prevHash := ""
	if storedPrevHash.Valid {
		prevHash = storedPrevHash.String
	}

	// Recalculate HMAC
	h := hmac.New(sha256.New, []byte(secret))
	hashData := fmt.Sprintf("%s:%s:%s:%s:%s",
		prevHash,
		threadID,
		stepUUID.String,
		idempKey.String,
		recordedAt.Format(time.RFC3339))
	h.Write([]byte(hashData))
	expectedHash := fmt.Sprintf("hmac-sha256-%s:%x", version, h.Sum(nil))

	// Verify
	if storedHash.String != expectedHash {
		return false, "Hash mismatch - data tampering detected", nil
	}

	return true, "", nil
}

// GetStepHashes retrieves the hash and prevHash for a specific step
// Returns hash, prevHash, and any error
func (r *ActivityRepository) GetStepHashes(ctx context.Context, threadID, stepName, idempotencyKey string) (hash, prevHash string, err error) {
	query := `
		SELECT 
			hash, 
			prev_hash
		FROM thread_activities 
		WHERE thread_id = $1 
		AND activity_type = 'step_recorded'
		AND payload->>'step_name' = $2
		AND payload->>'idempotency_key' = $3
		ORDER BY recorded_at DESC
		LIMIT 1
	`

	var storedHash, storedPrevHash sql.NullString

	err = r.pool.QueryRow(ctx, query, threadID, stepName, idempotencyKey).Scan(
		&storedHash, &storedPrevHash,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil // Step not found, return empty hashes
		}
		return "", "", fmt.Errorf("failed to query step hashes: %w", err)
	}

	hash = ""
	if storedHash.Valid {
		hash = storedHash.String
	}

	prevHash = ""
	if storedPrevHash.Valid {
		prevHash = storedPrevHash.String
	}

	return hash, prevHash, nil
}

// GetStepHashesByID retrieves the hash and prevHash for a step by its step ID
// Returns hash, prevHash, and any error
func (r *ActivityRepository) GetStepHashesByID(ctx context.Context, threadID, stepID string) (hash, prevHash string, err error) {
	query := `
		SELECT 
			hash, 
			prev_hash
		FROM thread_activities 
		WHERE thread_id = $1 
		AND activity_type = 'step_recorded'
		AND payload->>'step_uuid' = $2
		ORDER BY recorded_at DESC
		LIMIT 1
	`

	var storedHash, storedPrevHash sql.NullString

	err = r.pool.QueryRow(ctx, query, threadID, stepID).Scan(
		&storedHash, &storedPrevHash,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil // Step not found, return empty hashes
		}
		return "", "", fmt.Errorf("failed to query step hashes by ID: %w", err)
	}

	hash = ""
	if storedHash.Valid {
		hash = storedHash.String
	}

	prevHash = ""
	if storedPrevHash.Valid {
		prevHash = storedPrevHash.String
	}

	return hash, prevHash, nil
}
