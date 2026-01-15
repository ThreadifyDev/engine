package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

// ThreadRepository handles thread persistence in PostgreSQL
type ThreadRepository struct {
	pool     *pgxpool.Pool
	refsRepo *ThreadRefsRepository
}

// NewThreadRepository creates a new Postgres thread repository
func NewThreadRepository(pool *pgxpool.Pool) *ThreadRepository {
	return &ThreadRepository{
		pool:     pool,
		refsRepo: NewThreadRefsRepository(pool),
	}
}

// Save persists a thread to PostgreSQL
func (r *ThreadRepository) Save(ctx context.Context, thread *models.Thread) error {
	query := `
		INSERT INTO threads (
			id, contract_id, contract_version, owner_id, company_id, 
			created_at, updated_at, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			contract_id = EXCLUDED.contract_id,
			contract_version = EXCLUDED.contract_version,
			updated_at = EXCLUDED.updated_at,
			error = EXCLUDED.error
	`

	now := time.Now()
	_, err := r.pool.Exec(ctx, query,
		thread.ID,
		thread.ContractID,
		thread.ContractVersion,
		thread.OwnerID,
		thread.CompanyID,
		thread.StartedAt, // Map StartedAt to created_at column
		now,              // Use current time for updated_at
		thread.Error,
	)

	if err != nil {
		return fmt.Errorf("failed to save thread: %w", err)
	}

	return nil
}

// Get retrieves a thread from PostgreSQL
func (r *ThreadRepository) Get(ctx context.Context, threadID string) (*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, company_id,
			   created_at, updated_at, error
		FROM threads
		WHERE id = $1
	`

	var thread models.Thread
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, threadID).Scan(
		&thread.ID,
		&thread.ContractID,
		&thread.ContractVersion,
		&thread.OwnerID,
		&thread.CompanyID,
		&createdAt,
		&updatedAt,
		&thread.Error,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	thread.StartedAt = createdAt
	thread.CompletedAt = nil // Will be set based on status logic

	return &thread, nil
}

// GetWithRefs retrieves a thread from PostgreSQL with its refs loaded
func (r *ThreadRepository) GetWithRefs(ctx context.Context, threadID string) (*models.Thread, error) {
	// Get thread metadata
	thread, err := r.Get(ctx, threadID)
	if err != nil {
		return nil, err
	}

	// Load refs from thread_refs table
	refs, err := r.refsRepo.GetRefs(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load refs: %w", err)
	}

	thread.Refs = refs
	return thread, nil
}

// GetThreadsByRef finds thread IDs that have a specific ref key-value pair
func (r *ThreadRepository) GetThreadsByRef(ctx context.Context, refKey, refValue string) ([]string, error) {
	return r.refsRepo.GetThreadsByRef(ctx, refKey, refValue)
}

// GetThreadsByRefWithFilters finds threads by ref with additional filtering (status, dates, pagination)
// SECURITY: Always filters by companyID to enforce company isolation
func (r *ThreadRepository) GetThreadsByRefWithFilters(
	ctx context.Context,
	companyID string,
	refKey string,
	refValue string,
	status *string,
	startedAfter *string,
	startedBefore *string,
	limit int,
	offset int,
) ([]*models.Thread, error) {
	// Build query with dynamic filters
	query := `
		SELECT DISTINCT t.id, t.contract_id, t.contract_name, t.contract_version, 
		       t.owner_id, t.company_id, t.status, t.error, 
		       t.created_at, t.updated_at, t.completed_at
		FROM threads t
		JOIN thread_refs tr ON t.id = tr.thread_id
		WHERE t.company_id = $1
		  AND tr.ref_key = $2
		  AND tr.ref_value = $3
	`

	args := []interface{}{companyID, refKey, refValue}
	argIdx := 4

	// Add optional filters
	if status != nil && *status != "" {
		query += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, *status)
		argIdx++
	}

	if startedAfter != nil && *startedAfter != "" {
		query += fmt.Sprintf(" AND t.created_at >= $%d", argIdx)
		args = append(args, *startedAfter)
		argIdx++
	}

	if startedBefore != nil && *startedBefore != "" {
		query += fmt.Sprintf(" AND t.created_at <= $%d", argIdx)
		args = append(args, *startedBefore)
		argIdx++
	}

	// Add ordering and pagination
	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Execute query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads by ref with filters: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var createdAt, updatedAt time.Time
		var completedAt *time.Time
		var contractID, contractName *string
		var contractVersion *int
		var status, errorMsg *string

		err := rows.Scan(
			&thread.ID,
			&contractID,
			&contractName,
			&contractVersion,
			&thread.OwnerID,
			&thread.CompanyID,
			&status,
			&errorMsg,
			&createdAt,
			&updatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		// Set optional fields
		if contractID != nil {
			thread.ContractID = contractID
		}
		if contractName != nil {
			thread.ContractName = *contractName
		}
		if contractVersion != nil {
			thread.ContractVersion = contractVersion
		}
		if status != nil {
			thread.Status = models.ThreadStatus(*status)
		} else {
			thread.Status = models.ThreadStatusActive // Default
		}
		if errorMsg != nil {
			thread.Error = *errorMsg
		}

		thread.StartedAt = createdAt
		if completedAt != nil {
			thread.CompletedAt = completedAt
		}

		// Load refs for each thread
		refs, err := r.refsRepo.GetRefs(ctx, thread.ID)
		if err != nil {
			// Log error but continue
			fmt.Printf("Warning: failed to load refs for thread %s: %v\n", thread.ID, err)
			refs = make(map[string]string)
		}
		thread.Refs = refs

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, nil
}

// QueryThreads performs a general thread search with flexible filtering
// SECURITY: Always filters by companyID to enforce company isolation
func (r *ThreadRepository) QueryThreads(
	ctx context.Context,
	companyID string,
	actor *string,
	contractName *string,
	contractVersion *int,
	status *string,
	startedAfter *string,
	startedBefore *string,
	completedAfter *string,
	completedBefore *string,
	limit int,
	offset int,
) ([]*models.Thread, error) {
	// Build query with dynamic filters
	query := `
		SELECT DISTINCT t.id, t.contract_id, t.contract_name, t.contract_version,
		       t.owner_id, t.company_id, t.status, t.error,
		       t.created_at, t.updated_at, t.completed_at
		FROM threads t
	`

	// Add LEFT JOIN if actor filter is provided
	if actor != nil && *actor != "" {
		query += ` LEFT JOIN thread_activities ta ON t.id = ta.thread_id`
	}

	query += ` WHERE t.company_id = $1`

	args := []interface{}{companyID}
	argIdx := 2

	// Add actor filter (matches both actor and actor_service)
	if actor != nil && *actor != "" {
		query += fmt.Sprintf(" AND (ta.actor = $%d OR ta.actor_service = $%d)", argIdx, argIdx)
		args = append(args, *actor)
		argIdx++
	}

	// Add contract filters
	if contractName != nil && *contractName != "" {
		query += fmt.Sprintf(" AND t.contract_name = $%d", argIdx)
		args = append(args, *contractName)
		argIdx++
	}

	if contractVersion != nil {
		query += fmt.Sprintf(" AND t.contract_version = $%d", argIdx)
		args = append(args, *contractVersion)
		argIdx++
	}

	// Add status filter
	if status != nil && *status != "" {
		query += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, *status)
		argIdx++
	}

	// Add date filters
	if startedAfter != nil && *startedAfter != "" {
		query += fmt.Sprintf(" AND t.created_at >= $%d", argIdx)
		args = append(args, *startedAfter)
		argIdx++
	}

	if startedBefore != nil && *startedBefore != "" {
		query += fmt.Sprintf(" AND t.created_at <= $%d", argIdx)
		args = append(args, *startedBefore)
		argIdx++
	}

	if completedAfter != nil && *completedAfter != "" {
		query += fmt.Sprintf(" AND t.completed_at >= $%d", argIdx)
		args = append(args, *completedAfter)
		argIdx++
	}

	if completedBefore != nil && *completedBefore != "" {
		query += fmt.Sprintf(" AND t.completed_at <= $%d", argIdx)
		args = append(args, *completedBefore)
		argIdx++
	}

	// Add ordering and pagination
	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Execute query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var createdAt, updatedAt time.Time
		var completedAt *time.Time
		var contractID, contractName *string
		var contractVersion *int
		var status, errorMsg *string

		err := rows.Scan(
			&thread.ID,
			&contractID,
			&contractName,
			&contractVersion,
			&thread.OwnerID,
			&thread.CompanyID,
			&status,
			&errorMsg,
			&createdAt,
			&updatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		// Set optional fields
		if contractID != nil {
			thread.ContractID = contractID
		}
		if contractName != nil {
			thread.ContractName = *contractName
		}
		if contractVersion != nil {
			thread.ContractVersion = contractVersion
		}
		if status != nil {
			thread.Status = models.ThreadStatus(*status)
		} else {
			thread.Status = models.ThreadStatusActive
		}
		if errorMsg != nil {
			thread.Error = *errorMsg
		}

		thread.StartedAt = createdAt
		if completedAt != nil {
			thread.CompletedAt = completedAt
		}

		// Load refs for each thread
		refs, err := r.refsRepo.GetRefs(ctx, thread.ID)
		if err != nil {
			fmt.Printf("Warning: failed to load refs for thread %s: %v\n", thread.ID, err)
			refs = make(map[string]string)
		}
		thread.Refs = refs

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, nil
}

// QueryThreadsByContract performs contract-specific thread search (optimized for contract monitoring)
// SECURITY: Always filters by companyID to enforce company isolation
func (r *ThreadRepository) QueryThreadsByContract(
	ctx context.Context,
	companyID string,
	contractName string,
	contractVersion *int,
	actor *string,
	status *string,
	startedAfter *string,
	startedBefore *string,
	limit int,
	offset int,
) ([]*models.Thread, error) {
	// Build query with dynamic filters
	query := `
		SELECT DISTINCT t.id, t.contract_id, t.contract_name, t.contract_version,
		       t.owner_id, t.company_id, t.status, t.error,
		       t.created_at, t.updated_at, t.completed_at
		FROM threads t
	`

	// Add LEFT JOIN if actor filter is provided
	if actor != nil && *actor != "" {
		query += ` LEFT JOIN thread_activities ta ON t.id = ta.thread_id`
	}

	query += ` WHERE t.company_id = $1 AND t.contract_name = $2`

	args := []interface{}{companyID, contractName}
	argIdx := 3

	// Add contract version filter if provided
	if contractVersion != nil {
		query += fmt.Sprintf(" AND t.contract_version = $%d", argIdx)
		args = append(args, *contractVersion)
		argIdx++
	}

	// Add actor filter (matches both actor and actor_service)
	if actor != nil && *actor != "" {
		query += fmt.Sprintf(" AND (ta.actor = $%d OR ta.actor_service = $%d)", argIdx, argIdx)
		args = append(args, *actor)
		argIdx++
	}

	// Add status filter
	if status != nil && *status != "" {
		query += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, *status)
		argIdx++
	}

	// Add date filters
	if startedAfter != nil && *startedAfter != "" {
		query += fmt.Sprintf(" AND t.created_at >= $%d", argIdx)
		args = append(args, *startedAfter)
		argIdx++
	}

	if startedBefore != nil && *startedBefore != "" {
		query += fmt.Sprintf(" AND t.created_at <= $%d", argIdx)
		args = append(args, *startedBefore)
		argIdx++
	}

	// Add ordering and pagination
	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Execute query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads by contract: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var createdAt, updatedAt time.Time
		var completedAt *time.Time
		var contractID, contractName *string
		var contractVersion *int
		var status, errorMsg *string

		err := rows.Scan(
			&thread.ID,
			&contractID,
			&contractName,
			&contractVersion,
			&thread.OwnerID,
			&thread.CompanyID,
			&status,
			&errorMsg,
			&createdAt,
			&updatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		// Set optional fields
		if contractID != nil {
			thread.ContractID = contractID
		}
		if contractName != nil {
			thread.ContractName = *contractName
		}
		if contractVersion != nil {
			thread.ContractVersion = contractVersion
		}
		if status != nil {
			thread.Status = models.ThreadStatus(*status)
		} else {
			thread.Status = models.ThreadStatusActive
		}
		if errorMsg != nil {
			thread.Error = *errorMsg
		}

		thread.StartedAt = createdAt
		if completedAt != nil {
			thread.CompletedAt = completedAt
		}

		// Load refs for each thread
		refs, err := r.refsRepo.GetRefs(ctx, thread.ID)
		if err != nil {
			fmt.Printf("Warning: failed to load refs for thread %s: %v\n", thread.ID, err)
			refs = make(map[string]string)
		}
		thread.Refs = refs

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, nil
}

// GetByOwner retrieves all threads for a given owner
func (r *ThreadRepository) GetByOwner(ctx context.Context, ownerID string, limit, offset int) ([]*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, company_id,
			   created_at, updated_at, error
		FROM threads
		WHERE owner_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, ownerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&thread.ID,
			&thread.ContractID,
			&thread.ContractVersion,
			&thread.OwnerID,
			&thread.CompanyID,
			&createdAt,
			&updatedAt,
			&thread.Error,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		// Map database columns to model fields
		thread.StartedAt = createdAt
		thread.CompletedAt = nil

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return threads, nil
}

// GetByContract retrieves all threads for a given contract
func (r *ThreadRepository) GetByContract(ctx context.Context, contractID string, limit, offset int) ([]*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, company_id,
			   created_at, updated_at, error
		FROM threads
		WHERE contract_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, contractID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&thread.ID,
			&thread.ContractID,
			&thread.ContractVersion,
			&thread.OwnerID,
			&thread.CompanyID,
			&createdAt,
			&updatedAt,
			&thread.Error,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		// Map database columns to model fields
		thread.StartedAt = createdAt
		thread.CompletedAt = nil

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return threads, nil
}

// Delete removes a thread from PostgreSQL
func (r *ThreadRepository) Delete(ctx context.Context, threadID string) error {
	query := `DELETE FROM threads WHERE id = $1`

	_, err := r.pool.Exec(ctx, query, threadID)
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	return nil
}

// Count returns the total number of threads for an owner
func (r *ThreadRepository) Count(ctx context.Context, ownerID string) (int, error) {
	query := `SELECT COUNT(*) FROM threads WHERE owner_id = $1`

	var count int
	err := r.pool.QueryRow(ctx, query, ownerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count threads: %w", err)
	}

	return count, nil
}

// GetThreadRefsRepo returns the thread refs repository
func (r *ThreadRepository) GetThreadRefsRepo() *ThreadRefsRepository {
	return r.refsRepo
}
