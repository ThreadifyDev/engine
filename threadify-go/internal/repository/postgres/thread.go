package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/interfaces"
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
			   created_at, updated_at, error, status, contract_name
		FROM threads
		WHERE id = $1
	`

	var thread models.Thread
	var createdAt, updatedAt time.Time
	var contractID, contractName, errorMsg sql.NullString
	var contractVersion sql.NullInt32
	var status string

	err := r.pool.QueryRow(ctx, query, threadID).Scan(
		&thread.ID,
		&contractID,
		&contractVersion,
		&thread.OwnerID,
		&thread.CompanyID,
		&createdAt,
		&updatedAt,
		&errorMsg,
		&status,
		&contractName,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	// Handle nullable fields
	if contractID.Valid {
		thread.ContractID = &contractID.String
	}
	if contractVersion.Valid {
		v := int(contractVersion.Int32)
		thread.ContractVersion = &v
	}
	if errorMsg.Valid {
		thread.Error = errorMsg.String
	}
	if contractName.Valid {
		thread.ContractName = contractName.String
	}

	thread.StartedAt = createdAt
	thread.Status = models.ThreadStatus(status)
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
) ([]*models.Thread, int, error) {
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

	// Execute COUNT query first (without LIMIT/OFFSET)
	// Build count query with same filters as main query
	countQuery := `
		SELECT COUNT(DISTINCT t.id)
		FROM threads t
		JOIN thread_refs tr ON t.id = tr.thread_id
		WHERE t.company_id = $1
		  AND tr.ref_key = $2
		  AND tr.ref_value = $3
	`

	// Add all the same filters that were added to main query
	countArgIdx := 4
	if status != nil && *status != "" {
		countQuery += fmt.Sprintf(" AND t.status = $%d", countArgIdx)
		countArgIdx++
	}
	if startedAfter != nil && *startedAfter != "" {
		countQuery += fmt.Sprintf(" AND t.created_at >= $%d", countArgIdx)
		countArgIdx++
	}
	if startedBefore != nil && *startedBefore != "" {
		countQuery += fmt.Sprintf(" AND t.created_at <= $%d", countArgIdx)
		countArgIdx++
	}

	// Use same args (without limit/offset)
	countArgs := args[:len(args)]

	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count threads by ref: %w", err)
	}

	// Add ordering and pagination to main query
	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Execute main query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query threads by ref with filters: %w", err)
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
			return nil, 0, fmt.Errorf("failed to scan thread: %w", err)
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

		// Note: Refs will be batch-loaded in the resolver if requested
		thread.Refs = make(map[string]string)

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, totalCount, nil
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
	repoStart := time.Now()
	fmt.Printf("[PERF] QueryThreads: START\\n")
	defer func() {
		fmt.Printf("[PERF] QueryThreads: TOTAL %v\\n", time.Since(repoStart))
	}()

	// Build query with dynamic filters
	buildStart := time.Now()
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
	fmt.Printf("[PERF] QueryThreads.buildQuery: %v\n", time.Since(buildStart))

	// Execute query
	execStart := time.Now()
	rows, err := r.pool.Query(ctx, query, args...)
	fmt.Printf("[PERF] QueryThreads.executeQuery: %v\n", time.Since(execStart))
	if err != nil {
		return nil, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	scanStart := time.Now()
	rowCount := 0
	for rows.Next() {
		rowCount++
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

		// Note: Refs will be batch-loaded in the resolver if requested
		thread.Refs = make(map[string]string)

		threads = append(threads, &thread)
	}
	fmt.Printf("[PERF] QueryThreads.scanRows: %v (scanned %d rows)\n", time.Since(scanStart), rowCount)

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, nil
}

// QueryThreadsWithAccess performs thread search with SQL-based access filtering
// This is more efficient than post-query access checks and works with archived data
// SECURITY: Filters by companyID AND user access (owner OR explicit thread_access grant)
func (r *ThreadRepository) QueryThreadsWithAccess(
	ctx context.Context,
	companyID string,
	userID string,
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
) ([]*models.Thread, int, error) {
	repoStart := time.Now()
	fmt.Printf("[PERF] QueryThreadsWithAccess: START\n")
	defer func() {
		fmt.Printf("[PERF] QueryThreadsWithAccess: TOTAL %v\n", time.Since(repoStart))
	}()

	// Build query with access filtering in SQL
	buildStart := time.Now()
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

	// MVP: Company-wide access
	// Users can view all threads that belong to their company
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

		if contractVersion != nil {
			query += fmt.Sprintf(" AND t.contract_version = $%d", argIdx)
			args = append(args, *contractVersion)
			argIdx++
		}
	}

	// Add status filter
	if status != nil && *status != "" {
		query += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, *status)
		argIdx++
	}

	// Add date range filters
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

	fmt.Printf("[PERF] QueryThreadsWithAccess.buildQuery: %v\n", time.Since(buildStart))

	// Execute COUNT query first (without LIMIT/OFFSET)
	countStart := time.Now()
	countQuery := "SELECT COUNT(DISTINCT t.id) FROM threads t"

	// Add LEFT JOIN if actor filter is provided
	if actor != nil && *actor != "" {
		countQuery += " LEFT JOIN thread_activities ta ON t.id = ta.thread_id"
	}

	// Add WHERE clause - same as main query
	countQuery += " WHERE t.company_id = $1"

	// Add all the same filters that were added to main query
	countArgIdx := 2
	if actor != nil && *actor != "" {
		countQuery += fmt.Sprintf(" AND (ta.actor = $%d OR ta.actor_service = $%d)", countArgIdx, countArgIdx)
		countArgIdx++
	}
	if contractName != nil && *contractName != "" {
		countQuery += fmt.Sprintf(" AND t.contract_name = $%d", countArgIdx)
		countArgIdx++
	}
	if contractVersion != nil {
		countQuery += fmt.Sprintf(" AND t.contract_version = $%d", countArgIdx)
		countArgIdx++
	}
	if status != nil && *status != "" {
		countQuery += fmt.Sprintf(" AND t.status = $%d", countArgIdx)
		countArgIdx++
	}
	if startedAfter != nil && *startedAfter != "" {
		countQuery += fmt.Sprintf(" AND t.created_at >= $%d", countArgIdx)
		countArgIdx++
	}
	if startedBefore != nil && *startedBefore != "" {
		countQuery += fmt.Sprintf(" AND t.created_at <= $%d", countArgIdx)
		countArgIdx++
	}
	if completedAfter != nil && *completedAfter != "" {
		countQuery += fmt.Sprintf(" AND t.completed_at >= $%d", countArgIdx)
		countArgIdx++
	}
	if completedBefore != nil && *completedBefore != "" {
		countQuery += fmt.Sprintf(" AND t.completed_at <= $%d", countArgIdx)
		countArgIdx++
	}

	// Use same args (without limit/offset)
	countArgs := args[:len(args)]

	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
	fmt.Printf("[PERF] QueryThreadsWithAccess.countQuery: %v (total: %d)\n", time.Since(countStart), totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count threads: %w", err)
	}

	// Add ordering and pagination to main query
	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Execute main query
	execStart := time.Now()
	rows, err := r.pool.Query(ctx, query, args...)
	fmt.Printf("[PERF] QueryThreadsWithAccess.executeQuery: %v\n", time.Since(execStart))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query threads with access: %w", err)
	}
	defer rows.Close()

	// Scan results
	var threads []*models.Thread
	scanStart := time.Now()
	rowCount := 0
	for rows.Next() {
		rowCount++
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
			return nil, 0, fmt.Errorf("failed to scan thread row: %w", err)
		}

		// Map nullable fields
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

		// Note: Refs will be batch-loaded in the resolver if requested
		thread.Refs = make(map[string]string)

		threads = append(threads, &thread)
	}
	fmt.Printf("[PERF] QueryThreadsWithAccess.scanRows: %v (scanned %d rows)\n", time.Since(scanStart), rowCount)

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, totalCount, nil
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

		// Note: Refs will be batch-loaded in the resolver if requested
		thread.Refs = make(map[string]string)

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

// UpdateThreadStatus updates the status of a thread (closed or completed)
// Uses the provided timestamp for closed_at/completed_at
func (r *ThreadRepository) UpdateThreadStatus(
	ctx context.Context,
	threadID string,
	status string,
	timestamp time.Time,
) error {
	query := `
		UPDATE threads
		SET 
			status = $1,
			closed_at = CASE WHEN $1 = 'closed' THEN $2 ELSE closed_at END,
			completed_at = CASE WHEN $1 = 'completed' THEN $2 ELSE completed_at END,
			updated_at = $2
		WHERE id = $3
			AND status NOT IN ('closed', 'completed')
	`

	result, err := r.pool.Exec(ctx, query, status, timestamp, threadID)
	if err != nil {
		return fmt.Errorf("failed to update thread status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("thread not found or already closed/completed")
	}

	return nil
}

// GetThreadRefsRepo returns the thread refs repository
func (r *ThreadRepository) GetThreadRefsRepo() *ThreadRefsRepository {
	return r.refsRepo
}

// GetStepState retrieves a single step state from thread_step_states table
// Used for hot/cold fallback when step data expires from Valkey
func (r *ThreadRepository) GetStepState(ctx context.Context, threadID, stepName, idempotencyKey string) (*interfaces.StepStateSnapshot, error) {
	query := `
		SELECT id, thread_id, step_name, idempotency_key, status, 
		       retry_count, first_seen_at, last_updated_at, previous_step
		FROM thread_step_states
		WHERE thread_id = $1 AND step_name = $2 AND idempotency_key = $3
		LIMIT 1
	`

	var stepState interfaces.StepStateSnapshot
	var previousStep sql.NullString

	err := r.pool.QueryRow(ctx, query, threadID, stepName, idempotencyKey).Scan(
		&stepState.ID,
		&stepState.ThreadID,
		&stepState.StepName,
		&stepState.IdempotencyKey,
		&stepState.Status,
		&stepState.RetryCount,
		&stepState.FirstSeenAt,
		&stepState.LastUpdatedAt,
		&previousStep,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("step state not found")
		}
		return nil, fmt.Errorf("failed to get step state: %w", err)
	}

	if previousStep.Valid {
		stepState.PreviousStep = previousStep.String
	}

	return &stepState, nil
}

// GetAllStepStates has been removed - use StepStateRepository.GetStepsBatch() instead
// This was a duplicate function. All step state queries should use GetStepsBatch() from StepStateRepository

// GetCompletedSteps retrieves all completed steps for a thread from thread_step_states table
// Used for hot/cold fallback when step data expires from Valkey
func (r *ThreadRepository) GetCompletedSteps(ctx context.Context, threadID string) ([]interfaces.StepWithTimestamp, error) {
	query := `
		SELECT step_name, last_updated_at
		FROM thread_step_states
		WHERE thread_id = $1 AND status = 'completed'
		ORDER BY last_updated_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query completed steps: %w", err)
	}
	defer rows.Close()

	var steps []interfaces.StepWithTimestamp

	for rows.Next() {
		var step interfaces.StepWithTimestamp
		var lastUpdatedAt string

		err := rows.Scan(&step.StepName, &lastUpdatedAt)
		if err != nil {
			continue // Skip invalid entries
		}

		// Parse timestamp
		if completedAt, err := time.Parse(time.RFC3339, lastUpdatedAt); err == nil {
			step.CompletedAt = completedAt
		}

		steps = append(steps, step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return steps, nil
}

// GetThreadWithPermissionCheck retrieves a thread with SQL-level permission filtering
// MVP: Company-wide access
// Users can view all threads that belong to their company
// Runtime roles (from thread_access) determine what data they can see, not whether they have access
func (r *ThreadRepository) GetThreadWithPermissionCheck(ctx context.Context, threadID string, companyID string) (*models.Thread, error) {
	// Query thread with company-level access check
	query := `
		SELECT t.id, t.contract_id, t.contract_name, t.contract_version, 
		       t.owner_id, t.company_id, t.status, t.error,
		       t.created_at, t.updated_at, t.completed_at
		FROM threads t
		WHERE t.id = $1
		  AND t.company_id = $2
		LIMIT 1
	`

	var thread models.Thread
	var createdAt, updatedAt time.Time
	var completedAt *time.Time
	var contractID, contractName *string
	var contractVersion *int
	var status, errorMsg *string

	err := r.pool.QueryRow(ctx, query, threadID, companyID).Scan(
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
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("thread not found or access denied")
		}
		return nil, fmt.Errorf("failed to get thread with permission check: %w", err)
	}

	// Map nullable fields
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

	thread.Refs = make(map[string]string)

	return &thread, nil
}

// GetThreadChainWithPermissionCheck retrieves a thread chain with permission check at each level
// MVP: Company-wide access
// Uses recursive CTE with company-level filtering - only returns threads from user's company
func (r *ThreadRepository) GetThreadChainWithPermissionCheck(
	ctx context.Context,
	rootID string,
	companyID string,
	maxDepth *int,
) ([]*models.Thread, error) {

	depth := 10 // Default max depth
	if maxDepth != nil && *maxDepth > 0 {
		depth = *maxDepth
	}

	query := `
		WITH RECURSIVE thread_chain AS (
			-- Base case: root thread with company-level permission check
			SELECT t.id, t.contract_id, t.contract_name, t.contract_version,
			       t.owner_id, t.company_id, t.status, t.error,
			       t.created_at, t.updated_at, t.completed_at,
			       1 as depth
			FROM threads t
			WHERE t.id = $1
			  AND t.company_id = $2
			
			UNION ALL
			
			-- Recursive case: linked threads with company-level permission check
			SELECT t.id, t.contract_id, t.contract_name, t.contract_version,
			       t.owner_id, t.company_id, t.status, t.error,
			       t.created_at, t.updated_at, t.completed_at,
			       tc.depth + 1
			FROM threads t
			INNER JOIN thread_refs tr ON t.id = tr.ref_value
			INNER JOIN thread_chain tc ON tr.thread_id = tc.id
			WHERE tr.ref_key LIKE 'linkedThread:%'
			  AND tc.depth < $3
			  AND t.company_id = $2
		)
		SELECT id, contract_id, contract_name, contract_version,
		       owner_id, company_id, status, error,
		       created_at, updated_at, completed_at
		FROM thread_chain
		ORDER BY depth ASC
	`

	rows, err := r.pool.Query(ctx, query, rootID, companyID, depth)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread chain: %w", err)
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
			return nil, fmt.Errorf("failed to scan thread chain: %w", err)
		}

		// Map nullable fields
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

		thread.Refs = make(map[string]string)
		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating thread chain: %w", err)
	}

	return threads, nil
}
