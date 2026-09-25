package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	shderrors "threadify-go/shared/errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/perf"
	"go.uber.org/zap"
)

// threadSelectCols is the canonical column list for full thread SELECT queries.
const threadSelectCols = `t.id, t.label, t.contract_id, t.contract_name, t.contract_version,
	t.owner_id, t.company_id, t.status, t.error,
	t.created_at, t.updated_at, t.completed_at,
	(SELECT COALESCE(array_agg(tag), '{}') FROM thread_tags WHERE thread_id = t.id) as tags`

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

func scanThreadRow(row pgx.Row, thread *domain.Thread) error {
	var createdAt, updatedAt time.Time
	var completedAt *time.Time
	var contractID, contractName *string
	var contractVersion *int
	var ownerID sql.NullString
	var status, errorMsg, label *string
	var tags []string

	if err := row.Scan(
		&thread.ID, &label, &contractID, &contractName, &contractVersion,
		&ownerID, &thread.CompanyID,
		&status, &errorMsg,
		&createdAt, &updatedAt, &completedAt,
		&tags,
	); err != nil {
		return err
	}

	thread.Tags = tags
	thread.OwnerID = ownerID.String

	if label != nil {
		thread.Label = *label
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
		thread.Status = domain.ThreadStatus(*status)
	} else {
		thread.Status = domain.ThreadStatusActive
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

func (b *threadQueryBuilder) addOptionalTagsFilter(tags []string) {
	if len(tags) > 0 {
		b.where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM thread_tags tt WHERE tt.thread_id = t.id AND tt.tag = ANY($%d))", b.argIdx)
		b.args = append(b.args, tags)
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

func (r *ThreadRepository) Save(ctx context.Context, thread *domain.Thread) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO threads (
			id, label, contract_id, contract_name, contract_version, owner_id, company_id,
			status, created_at, updated_at, error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			label            = EXCLUDED.label,
			contract_id      = EXCLUDED.contract_id,
			contract_name    = EXCLUDED.contract_name,
			contract_version = EXCLUDED.contract_version,
			status           = EXCLUDED.status,
			updated_at       = EXCLUDED.updated_at,
			error            = EXCLUDED.error`,
		thread.ID, thread.Label, thread.ContractID, thread.ContractName, thread.ContractVersion,
		thread.OwnerID, thread.CompanyID,
		thread.Status, thread.StartedAt, time.Now(), thread.Error,
	)
	if err != nil {
		return fmt.Errorf("save thread: %w", err)
	}

	// Bulk write tags into the normalised thread_tags table.
	if len(thread.Tags) > 0 {
		placeholders := make([]string, 0, len(thread.Tags))
		args := make([]interface{}, 0, len(thread.Tags)*3)
		argIdx := 1
		for _, tag := range thread.Tags {
			placeholders = append(placeholders, fmt.Sprintf("($%d,$%d,$%d)", argIdx, argIdx+1, argIdx+2))
			args = append(args, thread.ID, tag, thread.CompanyID)
			argIdx += 3
		}

		query := "INSERT INTO thread_tags (thread_id, tag, company_id) VALUES " +
			strings.Join(placeholders, ", ") +
			" ON CONFLICT (thread_id, tag) DO NOTHING"

		_, err = tx.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("save thread tags: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (r *ThreadRepository) Get(ctx context.Context, threadID string, opts ...domain.ThreadReadOptions) (*domain.Thread, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+threadSelectCols+` FROM threads t WHERE t.id = $1`, threadID)
	var thread domain.Thread
	if err := scanThreadRow(row, &thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shderrors.ErrThreadNotFound
		}
		return nil, fmt.Errorf("get thread: %w", err)
	}
	return &thread, nil
}

func (r *ThreadRepository) GetWithRefs(ctx context.Context, threadID string) (*domain.Thread, error) {
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
	companyID string,
	refKeys []string,
	refValue string,
	status, startedAfter, startedBefore *string,
	limit, offset int,
) ([]*domain.Thread, int, error) {
	var from string
	var b *threadQueryBuilder

	like := "%" + refValue + "%"
	if len(refKeys) == 0 {
		from = "threads t JOIN thread_refs tr ON t.id = tr.thread_id"
		b = newThreadQueryBuilder("t.company_id = $1 AND tr.ref_value ILIKE $2", companyID, like)
	} else if len(refKeys) == 1 {
		from = "threads t JOIN thread_refs tr ON t.id = tr.thread_id"
		b = newThreadQueryBuilder("t.company_id = $1 AND tr.ref_key = $2 AND tr.ref_value ILIKE $3", companyID, refKeys[0], like)
	} else {
		from = "threads t JOIN thread_refs tr ON t.id = tr.thread_id"
		b = newThreadQueryBuilder("t.company_id = $1 AND tr.ref_key = ANY($2) AND tr.ref_value ILIKE $3", companyID, refKeys, like)
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
	status *string,
	tags []string,
	startedAfter, startedBefore, completedAfter, completedBefore *string,
	limit, offset int,
) ([]*domain.Thread, error) {
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
	b.addOptionalTagsFilter(tags)
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
	status *string,
	tags []string,
	startedAfter, startedBefore, completedAfter, completedBefore *string,
	limit, offset int,
) ([]*domain.Thread, int, error) {
	repoStart := perf.Now()
	defer func() {
		perf.LogStructured("QueryThreadsWithAccess.total", zap.Duration("duration", perf.Since(repoStart)))
	}()

	from := "threads t"

	b := newThreadQueryBuilder("t.company_id = $1", companyID)
	if actor != nil && *actor != "" {
		// Filter by owner_id since actor column doesn't exist in threads table
		b.addOptionalFilter("t.owner_id = $%d", actor)
	}
	if contractName != nil && *contractName != "" {
		b.addOptionalFilter("t.contract_name = $%d", contractName)
		b.addOptionalIntFilter("t.contract_version = $%d", contractVersion)
	}
	b.addOptionalFilter("t.status = $%d", status)
	b.addOptionalTagsFilter(tags)
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
	actor, status *string,
	tags []string,
	startedAfter, startedBefore *string,
	limit, offset int,
) ([]*domain.Thread, error) {
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
	b.addOptionalTagsFilter(tags)
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

func (r *ThreadRepository) GetByOwner(ctx context.Context, ownerID string, limit, offset int) ([]*domain.Thread, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, label, contract_id, contract_version, owner_id, company_id, status, created_at, updated_at, error
		FROM threads WHERE owner_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		ownerID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query threads by owner: %w", err)
	}
	defer rows.Close()
	return scanSimpleThreadRows(rows)
}

func (r *ThreadRepository) GetByContract(ctx context.Context, contractID string, limit, offset int) ([]*domain.Thread, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, label, contract_id, contract_version, owner_id, company_id, status, created_at, updated_at, error
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
			return shderrors.ErrThreadNotFound
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
			closed_at    = CASE WHEN $2 THEN $4 ELSE closed_at    END,
			completed_at = CASE WHEN $3 THEN $4 ELSE completed_at END,
			updated_at   = $4
		WHERE id = $5`,
		status, status == "closed", status == "completed", timestamp, threadID,
	)
	if err != nil {
		return fmt.Errorf("update thread status: %w", err)
	}
	return nil
}

func (r *ThreadRepository) GetThreadRefsRepo() *ThreadRefsRepository {
	return r.refsRepo
}

func (r *ThreadRepository) GetStepState(ctx context.Context, threadID, stepName, idempotencyKey string) (*domain.StepStateSnapshot, error) {
	var s domain.StepStateSnapshot
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
			return nil, shderrors.ErrNoActiveThread
		}
		return nil, fmt.Errorf("get step state: %w", err)
	}
	if previousStep.Valid {
		s.PreviousStep = previousStep.String
	}
	return &s, nil
}

func (r *ThreadRepository) GetCompletedSteps(ctx context.Context, threadID string) ([]domain.StepWithTimestamp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT step_name, last_updated_at
		FROM thread_step_states
		WHERE thread_id=$1 AND status IN ('success', 'completed')
		ORDER BY last_updated_at ASC`,
		threadID,
	)
	if err != nil {
		return nil, fmt.Errorf("query completed steps: %w", err)
	}
	defer rows.Close()

	var steps []domain.StepWithTimestamp
	for rows.Next() {
		var step domain.StepWithTimestamp
		if err := rows.Scan(&step.StepName, &step.CompletedAt); err != nil {
			continue
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate completed steps: %w", err)
	}
	return steps, nil
}

func (r *ThreadRepository) GetThreadWithPermissionCheck(ctx context.Context, threadID, companyID string) (*domain.Thread, error) {
	var thread domain.Thread
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

func (r *ThreadRepository) GetThreadChainWithPermissionCheck(ctx context.Context, rootID, companyID string, maxDepth *int) ([]*domain.Thread, error) {
	depth := 10
	if maxDepth != nil && *maxDepth > 0 {
		depth = *maxDepth
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE thread_chain AS (
			SELECT t.id, t.label, t.contract_id, t.contract_name, t.contract_version,
			       t.owner_id, t.company_id, t.status, t.error,
			       t.created_at, t.updated_at, t.completed_at, 1 AS depth
			FROM threads t
			WHERE t.id=$1 AND t.company_id=$2
			
			UNION ALL
			
			SELECT t.id, t.label, t.contract_id, t.contract_name, t.contract_version,
			       t.owner_id, t.company_id, t.status, t.error,
			       t.created_at, t.updated_at, t.completed_at, tc.depth+1
			FROM threads t
			INNER JOIN thread_refs tr ON t.id = tr.ref_value
			INNER JOIN thread_chain tc ON tr.thread_id = tc.id
			WHERE tr.ref_key LIKE 'linkedThread:%'
			  AND tc.depth < $3
			  AND t.company_id = $2
		)
		SELECT tc.id, tc.label, tc.contract_id, tc.contract_name, tc.contract_version,
		       tc.owner_id, tc.company_id, tc.status, tc.error,
		       tc.created_at, tc.updated_at, tc.completed_at,
		       (SELECT COALESCE(array_agg(tag), '{}') FROM thread_tags WHERE thread_id = tc.id) as tags
		FROM thread_chain tc ORDER BY tc.depth ASC`,
		rootID, companyID, depth,
	)
	if err != nil {
		return nil, fmt.Errorf("query thread chain: %w", err)
	}
	defer rows.Close()
	return scanThreadRows(rows)
}

func (r *ThreadRepository) GetPool() *pgxpool.Pool {
	return r.pool
}

// --- row scanning helpers ---

// rowScanner is satisfied by both pgx.Rows (in a loop) and pgx.Row.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanThreadRows(rows pgx.Rows) ([]*domain.Thread, error) {
	var threads []*domain.Thread
	for rows.Next() {
		var t domain.Thread
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

// scanSimpleThreadRows scans the slim rows returned by GetByOwner/GetByContract.
func scanSimpleThreadRows(rows pgx.Rows) ([]*domain.Thread, error) {
	var threads []*domain.Thread
	for rows.Next() {
		var t domain.Thread
		var createdAt, updatedAt time.Time
		var label *string
		var ownerID sql.NullString
		var status *string
		if err := rows.Scan(
			&t.ID, &label, &t.ContractID, &t.ContractVersion,
			&ownerID, &t.CompanyID, &status,
			&createdAt, &updatedAt, &t.Error,
		); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		if label != nil {
			t.Label = *label
		}
		t.OwnerID = ownerID.String
		if status != nil {
			t.Status = domain.ThreadStatus(*status)
		}
		t.StartedAt = createdAt
		threads = append(threads, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate threads: %w", err)
	}
	return threads, nil
}
