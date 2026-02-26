package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	shderrors "threadify-go/shared/errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/perf"
	"go.uber.org/zap"
)

// threadSelectCols is the canonical column list for full thread SELECT queries.
const threadSelectCols = `t.id, t.contract_id, t.contract_name, t.contract_version,
	t.owner_id, t.company_id, t.status, t.error,
	t.created_at, t.updated_at, t.completed_at`

type ThreadRepository struct {
	pool     *pgxpool.Pool
	refsRepo *ThreadRefsRepository
}

func NewThreadRepository(pool *pgxpool.Pool) *ThreadRepository {
	return &ThreadRepository{
		pool:     pool,
		refsRepo: NewThreadRefsRepository(pool),
	}
}

// scanThreadRow scans a full thread row (11 columns matching threadSelectCols) into thread.
func scanThreadRow(row pgx.Row, thread *models.Thread) error {
	var createdAt, updatedAt time.Time
	var completedAt *time.Time
	var contractID, contractName *string
	var contractVersion *int
	var status, errorMsg *string

	if err := row.Scan(
		&thread.ID, &contractID, &contractName, &contractVersion,
		&thread.OwnerID, &thread.CompanyID,
		&status, &errorMsg,
		&createdAt, &updatedAt, &completedAt,
	); err != nil {
		return err
	}

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
	thread.CompletedAt = completedAt
	thread.Refs = make(map[string]string)
	return nil
}

// threadQueryBuilder builds SELECT + COUNT query pairs with shared filter clauses.
type threadQueryBuilder struct {
	where  string
	args   []interface{}
	argIdx int
}

func newThreadQueryBuilder(initialWhere string, initialArgs ...interface{}) *threadQueryBuilder {
	return &threadQueryBuilder{
		where:  initialWhere,
		args:   initialArgs,
		argIdx: len(initialArgs) + 1,
	}
}

func (b *threadQueryBuilder) addOptionalFilter(condition string, val *string) {
	if val != nil && *val != "" {
		b.where += fmt.Sprintf(" AND "+condition, b.argIdx)
		b.args = append(b.args, *val)
		b.argIdx++
	}
}

func (b *threadQueryBuilder) addOptionalIntFilter(condition string, val *int) {
	if val != nil {
		b.where += fmt.Sprintf(" AND "+condition, b.argIdx)
		b.args = append(b.args, *val)
		b.argIdx++
	}
}

func (b *threadQueryBuilder) buildSelect(cols, from, orderLimit string) string {
	return "SELECT " + cols + " FROM " + from + " WHERE " + b.where + orderLimit
}

func (b *threadQueryBuilder) buildCount(from string) string {
	return "SELECT COUNT(DISTINCT t.id) FROM " + from + " WHERE " + b.where
}

func (b *threadQueryBuilder) paginatedArgs(limit, offset int) []interface{} {
	return append(b.args, limit, offset)
}

// --- Repository methods ---

func (r *ThreadRepository) Save(ctx context.Context, thread *models.Thread) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO threads (
			id, contract_id, contract_version, owner_id, company_id,
			created_at, updated_at, error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			contract_id      = EXCLUDED.contract_id,
			contract_version = EXCLUDED.contract_version,
			updated_at       = EXCLUDED.updated_at,
			error            = EXCLUDED.error`,
		thread.ID, thread.ContractID, thread.ContractVersion,
		thread.OwnerID, thread.CompanyID,
		thread.StartedAt, time.Now(), thread.Error,
	)
	if err != nil {
		return fmt.Errorf("save thread: %w", err)
	}
	return nil
}

func (r *ThreadRepository) Get(ctx context.Context, threadID string) (*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, company_id,
		       created_at, updated_at, error, status, contract_name
		FROM threads WHERE id = $1`

	var thread models.Thread
	var createdAt, updatedAt time.Time
	var contractID, contractName, errorMsg sql.NullString
	var contractVersion sql.NullInt32
	var status string

	err := r.pool.QueryRow(ctx, query, threadID).Scan(
		&thread.ID, &contractID, &contractVersion,
		&thread.OwnerID, &thread.CompanyID,
		&createdAt, &updatedAt, &errorMsg, &status, &contractName,
	)
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}

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
	return &thread, nil
}

func (r *ThreadRepository) GetWithRefs(ctx context.Context, threadID string) (*models.Thread, error) {
	thread, err := r.Get(ctx, threadID)
	if err != nil {
		return nil, err
	}
	refs, err := r.refsRepo.GetRefs(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("load refs: %w", err)
	}
	thread.Refs = refs
	return thread, nil
}

func (r *ThreadRepository) GetThreadsByRef(ctx context.Context, refKey, refValue string) ([]string, error) {
	return r.refsRepo.GetThreadsByRef(ctx, refKey, refValue)
}

func (r *ThreadRepository) GetThreadsByRefWithFilters(
	ctx context.Context,
	companyID, refKey, refValue string,
	status, startedAfter, startedBefore *string,
	limit, offset int,
) ([]*models.Thread, int, error) {
	var from string
	var b *threadQueryBuilder

	like := "%" + refValue + "%"
	if refKey == "" {
		from = "threads t JOIN thread_refs tr ON t.id = tr.thread_id"
		b = newThreadQueryBuilder("t.company_id = $1 AND tr.ref_value ILIKE $2", companyID, like)
	} else {
		from = "threads t JOIN thread_refs tr ON t.id = tr.thread_id"
		b = newThreadQueryBuilder("t.company_id = $1 AND tr.ref_key = $2 AND tr.ref_value ILIKE $3", companyID, refKey, like)
	}

	b.addOptionalFilter("t.status = $%d", status)
	b.addOptionalFilter("t.created_at >= $%d", startedAfter)
	b.addOptionalFilter("t.created_at <= $%d", startedBefore)

	var totalCount int
	if err := r.pool.QueryRow(ctx, b.buildCount(from), b.args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count threads by ref: %w", err)
	}

	orderLimit := fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", b.argIdx, b.argIdx+1)
	rows, err := r.pool.Query(ctx, b.buildSelect("DISTINCT "+threadSelectCols, from, orderLimit), b.paginatedArgs(limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query threads by ref: %w", err)
	}
	defer rows.Close()

	threads, err := scanThreadRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return threads, totalCount, nil
}

func (r *ThreadRepository) QueryThreads(
	ctx context.Context,
	companyID string,
	actor, contractName *string,
	contractVersion *int,
	status, startedAfter, startedBefore, completedAfter, completedBefore *string,
	limit, offset int,
) ([]*models.Thread, error) {
	repoStart := perf.Now()
	defer func() {
		perf.LogStructured("QueryThreads.total", zap.Duration("duration", perf.Since(repoStart)))
	}()

	from := "threads t"
	if actor != nil && *actor != "" {
		from += " LEFT JOIN thread_activities ta ON t.id = ta.thread_id"
	}

	b := newThreadQueryBuilder("t.company_id = $1", companyID)
	if actor != nil && *actor != "" {
		b.where += fmt.Sprintf(" AND (ta.actor = $%d OR ta.actor_service = $%d)", b.argIdx, b.argIdx)
		b.args = append(b.args, *actor)
		b.argIdx++
	}
	b.addOptionalFilter("t.contract_name = $%d", contractName)
	b.addOptionalIntFilter("t.contract_version = $%d", contractVersion)
	b.addOptionalFilter("t.status = $%d", status)
	b.addOptionalFilter("t.created_at >= $%d", startedAfter)
	b.addOptionalFilter("t.created_at <= $%d", startedBefore)
	b.addOptionalFilter("t.completed_at >= $%d", completedAfter)
	b.addOptionalFilter("t.completed_at <= $%d", completedBefore)

	orderLimit := fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", b.argIdx, b.argIdx+1)
	rows, err := r.pool.Query(ctx, b.buildSelect("DISTINCT "+threadSelectCols, from, orderLimit), b.paginatedArgs(limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("query threads: %w", err)
	}
	defer rows.Close()
	return scanThreadRows(rows)
}

func (r *ThreadRepository) QueryThreadsWithAccess(
	ctx context.Context,
	companyID, userID string,
	actor, contractName *string,
	contractVersion *int,
	status, startedAfter, startedBefore, completedAfter, completedBefore *string,
	limit, offset int,
) ([]*models.Thread, int, error) {
	repoStart := perf.Now()
	defer func() {
		perf.LogStructured("QueryThreadsWithAccess.total", zap.Duration("duration", perf.Since(repoStart)))
	}()

	from := "threads t"
	if actor != nil && *actor != "" {
		from += " LEFT JOIN thread_activities ta ON t.id = ta.thread_id"
	}

	b := newThreadQueryBuilder("t.company_id = $1", companyID)
	if actor != nil && *actor != "" {
		b.where += fmt.Sprintf(" AND (ta.actor = $%d OR ta.actor_service = $%d)", b.argIdx, b.argIdx)
		b.args = append(b.args, *actor)
		b.argIdx++
	}
	if contractName != nil && *contractName != "" {
		b.addOptionalFilter("t.contract_name = $%d", contractName)
		b.addOptionalIntFilter("t.contract_version = $%d", contractVersion)
	}
	b.addOptionalFilter("t.status = $%d", status)
	b.addOptionalFilter("t.created_at >= $%d", startedAfter)
	b.addOptionalFilter("t.created_at <= $%d", startedBefore)
	b.addOptionalFilter("t.completed_at >= $%d", completedAfter)
	b.addOptionalFilter("t.completed_at <= $%d", completedBefore)

	var totalCount int
	countStart := perf.Now()
	if err := r.pool.QueryRow(ctx, b.buildCount(from), b.args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count threads: %w", err)
	}
	perf.LogStructured("QueryThreadsWithAccess.count", zap.Duration("duration", perf.Since(countStart)), zap.Int("total", totalCount))

	orderLimit := fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", b.argIdx, b.argIdx+1)
	execStart := perf.Now()
	rows, err := r.pool.Query(ctx, b.buildSelect("DISTINCT "+threadSelectCols, from, orderLimit), b.paginatedArgs(limit, offset)...)
	perf.LogStructured("QueryThreadsWithAccess.exec", zap.Duration("duration", perf.Since(execStart)))
	if err != nil {
		return nil, 0, fmt.Errorf("query threads with access: %w", err)
	}
	defer rows.Close()

	threads, err := scanThreadRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return threads, totalCount, nil
}

func (r *ThreadRepository) QueryThreadsByContract(
	ctx context.Context,
	companyID, contractName string,
	contractVersion *int,
	actor, status, startedAfter, startedBefore *string,
	limit, offset int,
) ([]*models.Thread, error) {
	from := "threads t"
	if actor != nil && *actor != "" {
		from += " LEFT JOIN thread_activities ta ON t.id = ta.thread_id"
	}

	b := newThreadQueryBuilder("t.company_id = $1 AND t.contract_name = $2", companyID, contractName)
	b.addOptionalIntFilter("t.contract_version = $%d", contractVersion)
	if actor != nil && *actor != "" {
		b.where += fmt.Sprintf(" AND (ta.actor = $%d OR ta.actor_service = $%d)", b.argIdx, b.argIdx)
		b.args = append(b.args, *actor)
		b.argIdx++
	}
	b.addOptionalFilter("t.status = $%d", status)
	b.addOptionalFilter("t.created_at >= $%d", startedAfter)
	b.addOptionalFilter("t.created_at <= $%d", startedBefore)

	orderLimit := fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", b.argIdx, b.argIdx+1)
	rows, err := r.pool.Query(ctx, b.buildSelect("DISTINCT "+threadSelectCols, from, orderLimit), b.paginatedArgs(limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("query threads by contract: %w", err)
	}
	defer rows.Close()
	return scanThreadRows(rows)
}

func (r *ThreadRepository) GetByOwner(ctx context.Context, ownerID string, limit, offset int) ([]*models.Thread, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, contract_id, contract_version, owner_id, company_id, created_at, updated_at, error
		FROM threads WHERE owner_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		ownerID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query threads by owner: %w", err)
	}
	defer rows.Close()
	return scanSimpleThreadRows(rows)
}

func (r *ThreadRepository) GetByContract(ctx context.Context, contractID string, limit, offset int) ([]*models.Thread, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, contract_id, contract_version, owner_id, company_id, created_at, updated_at, error
		FROM threads WHERE contract_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		contractID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query threads by contract: %w", err)
	}
	defer rows.Close()
	return scanSimpleThreadRows(rows)
}

func (r *ThreadRepository) Delete(ctx context.Context, threadID string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM threads WHERE id = $1`, threadID); err != nil {
		return fmt.Errorf("delete thread: %w", err)
	}
	return nil
}

func (r *ThreadRepository) Count(ctx context.Context, ownerID string) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM threads WHERE owner_id = $1`, ownerID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count threads: %w", err)
	}
	return count, nil
}

func (r *ThreadRepository) UpdateThreadStatus(ctx context.Context, threadID, status string, timestamp time.Time) error {
	var currentStatus string
	err := r.pool.QueryRow(ctx, `SELECT status FROM threads WHERE id = $1`, threadID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("thread not found")
		}
		return fmt.Errorf("check thread status: %w", err)
	}

	switch {
	case currentStatus == "closed":
		return fmt.Errorf("thread already closed")
	case currentStatus == "completed" && status == "completed":
		return nil // idempotent
	case currentStatus == "completed":
		return fmt.Errorf("thread already completed, cannot change to %s", status)
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE threads SET
			status       = $1,
			closed_at    = CASE WHEN $1 = 'closed'    THEN $2 ELSE closed_at    END,
			completed_at = CASE WHEN $1 = 'completed' THEN $2 ELSE completed_at END,
			updated_at   = $2
		WHERE id = $3`,
		status, timestamp, threadID,
	)
	if err != nil {
		return fmt.Errorf("update thread status: %w", err)
	}
	return nil
}

func (r *ThreadRepository) GetThreadRefsRepo() *ThreadRefsRepository {
	return r.refsRepo
}

func (r *ThreadRepository) GetStepState(ctx context.Context, threadID, stepName, idempotencyKey string) (*interfaces.StepStateSnapshot, error) {
	var s interfaces.StepStateSnapshot
	var previousStep sql.NullString

	err := r.pool.QueryRow(ctx, `
		SELECT id, thread_id, step_name, idempotency_key, status,
		       retry_count, first_seen_at, last_updated_at, previous_step
		FROM thread_step_states
		WHERE thread_id=$1 AND step_name=$2 AND idempotency_key=$3
		LIMIT 1`,
		threadID, stepName, idempotencyKey,
	).Scan(
		&s.ID, &s.ThreadID, &s.StepName, &s.IdempotencyKey, &s.Status,
		&s.RetryCount, &s.FirstSeenAt, &s.LastUpdatedAt, &previousStep,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("step state not found")
		}
		return nil, fmt.Errorf("get step state: %w", err)
	}
	if previousStep.Valid {
		s.PreviousStep = previousStep.String
	}
	return &s, nil
}

func (r *ThreadRepository) GetCompletedSteps(ctx context.Context, threadID string) ([]interfaces.StepWithTimestamp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT step_name, last_updated_at
		FROM thread_step_states
		WHERE thread_id=$1 AND status='completed'
		ORDER BY last_updated_at ASC`,
		threadID,
	)
	if err != nil {
		return nil, fmt.Errorf("query completed steps: %w", err)
	}
	defer rows.Close()

	var steps []interfaces.StepWithTimestamp
	for rows.Next() {
		var step interfaces.StepWithTimestamp
		var lastUpdatedAt string
		if err := rows.Scan(&step.StepName, &lastUpdatedAt); err != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, lastUpdatedAt); err == nil {
			step.CompletedAt = t
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate completed steps: %w", err)
	}
	return steps, nil
}

func (r *ThreadRepository) GetThreadWithPermissionCheck(ctx context.Context, threadID, companyID string) (*models.Thread, error) {
	var thread models.Thread
	err := scanThreadRow(r.pool.QueryRow(ctx, `
		SELECT `+threadSelectCols+`
		FROM threads t
		WHERE t.id=$1 AND t.company_id=$2
		LIMIT 1`,
		threadID, companyID,
	), &thread)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shderrors.ErrAccessDenied
		}
		return nil, fmt.Errorf("get thread with permission check: %w", err)
	}
	return &thread, nil
}

func (r *ThreadRepository) GetThreadChainWithPermissionCheck(ctx context.Context, rootID, companyID string, maxDepth *int) ([]*models.Thread, error) {
	depth := 10
	if maxDepth != nil && *maxDepth > 0 {
		depth = *maxDepth
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE thread_chain AS (
			SELECT t.id, t.contract_id, t.contract_name, t.contract_version,
			       t.owner_id, t.company_id, t.status, t.error,
			       t.created_at, t.updated_at, t.completed_at, 1 AS depth
			FROM threads t
			WHERE t.id=$1 AND t.company_id=$2

			UNION ALL

			SELECT t.id, t.contract_id, t.contract_name, t.contract_version,
			       t.owner_id, t.company_id, t.status, t.error,
			       t.created_at, t.updated_at, t.completed_at, tc.depth+1
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
		FROM thread_chain ORDER BY depth ASC`,
		rootID, companyID, depth,
	)
	if err != nil {
		return nil, fmt.Errorf("query thread chain: %w", err)
	}
	defer rows.Close()
	return scanThreadRows(rows)
}

// --- row scanning helpers ---

// rowScanner is satisfied by both pgx.Rows (in a loop) and pgx.Row.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanThreadRows(rows pgx.Rows) ([]*models.Thread, error) {
	var threads []*models.Thread
	for rows.Next() {
		var t models.Thread
		if err := scanThreadRow(rows, &t); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		threads = append(threads, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate threads: %w", err)
	}
	return threads, nil
}

// scanSimpleThreadRows scans the slim 8-column rows returned by GetByOwner/GetByContract.
func scanSimpleThreadRows(rows pgx.Rows) ([]*models.Thread, error) {
	var threads []*models.Thread
	for rows.Next() {
		var t models.Thread
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&t.ID, &t.ContractID, &t.ContractVersion,
			&t.OwnerID, &t.CompanyID,
			&createdAt, &updatedAt, &t.Error,
		); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		t.StartedAt = createdAt
		threads = append(threads, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate threads: %w", err)
	}
	return threads, nil
}
