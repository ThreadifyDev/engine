package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"threadify-go/api/internal/domain"
	serror "threadify-go/shared/errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type userRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) domain.UserRepository {
	return &userRepository{pool: pool}
}

func (r *userRepository) CreateTx(ctx context.Context, tx domain.Execer, user *domain.User) error {
	execer, ok := tx.(DBExecer)
	if !ok {
		return fmt.Errorf("invalid execer type: expected DBExecer, got %T", tx)
	}
	const query = `
        INSERT INTO users (id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
    `
	_, err := execer.Exec(ctx, query,
		user.ID, user.CompanyID, user.Email, user.AuthUserID,
		user.FullName, user.JobRole, user.EmailVerified,
		user.OnboardingCompleted, user.FirstInstrumentationDone,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user := &domain.User{}
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE email = $1
    `
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.CompanyID, &user.Email, &user.AuthUserID,
		&user.FullName, &user.JobRole, &user.EmailVerified,
		&user.OnboardingCompleted, &user.FirstInstrumentationDone,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, serror.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return user, nil
}

func (r *userRepository) ListByCompanyID(ctx context.Context, companyID string) ([]*domain.User, error) {
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE company_id = $1
        ORDER BY created_at ASC
    `
	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list users by company: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		if err := rows.Scan(
			&user.ID, &user.CompanyID, &user.Email, &user.AuthUserID,
			&user.FullName, &user.JobRole, &user.EmailVerified,
			&user.OnboardingCompleted, &user.FirstInstrumentationDone,
			&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
		); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	user := &domain.User{}
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE id = $1
    `
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.CompanyID, &user.Email, &user.AuthUserID,
		&user.FullName, &user.JobRole, &user.EmailVerified,
		&user.OnboardingCompleted, &user.FirstInstrumentationDone,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, serror.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func (r *userRepository) FindByAuthUserID(ctx context.Context, authUserID string) (*domain.User, error) {
	user := &domain.User{}
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE auth_user_id = $1
    `
	err := r.pool.QueryRow(ctx, query, authUserID).Scan(
		&user.ID, &user.CompanyID, &user.Email, &user.AuthUserID,
		&user.FullName, &user.JobRole, &user.EmailVerified,
		&user.OnboardingCompleted, &user.FirstInstrumentationDone,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, serror.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by auth user id: %w", err)
	}
	return user, nil
}

func (r *userRepository) UpdateAuthUserID(ctx context.Context, id, authUserID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET auth_user_id = $1, updated_at = NOW() WHERE id = $2`, authUserID, id)
	if err != nil {
		return fmt.Errorf("update auth user id: %w", err)
	}
	return nil
}

func (r *userRepository) UpdateEmailVerified(ctx context.Context, id string, verified bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET email_verified = $1, updated_at = NOW() WHERE id = $2`, verified, id)
	if err != nil {
		return fmt.Errorf("update email verified: %w", err)
	}
	return nil
}

func (r *userRepository) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

func (r *userRepository) UpdateProfile(ctx context.Context, id string, fullName, jobRole *string, onboardingCompleted bool) error {
	const query = `
        UPDATE users
        SET full_name = COALESCE($1, full_name),
            job_role = COALESCE($2, job_role),
            onboarding_completed = $3,
            updated_at = NOW()
        WHERE id = $4
    `
	_, err := r.pool.Exec(ctx, query, fullName, jobRole, onboardingCompleted, id)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *userRepository) MarkFirstInstrumentationDone(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET first_instrumentation_done = true, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark first instrumentation done: %w", err)
	}
	return nil
}

func (r *userRepository) GetPasswordChangedAt(ctx context.Context, id string) (*time.Time, error) {
	var changedAt *time.Time
	err := r.pool.QueryRow(ctx, `SELECT password_changed_at FROM users WHERE id = $1`, id).Scan(&changedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, serror.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get password changed at: %w", err)
	}
	return changedAt, nil
}

func (r *userRepository) GetPasswordHash(ctx context.Context, email string) (string, error) {
	var passwordHash string
	err := r.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email = $1`, email).Scan(&passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", serror.ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get password hash: %w", err)
	}
	return passwordHash, nil
}

func (r *userRepository) ClearPasswordHash(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET password_hash = NULL WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("clear password hash: %w", err)
	}
	return nil
}

func (r *userRepository) ArchiveUser(ctx context.Context, userID, archivedEmail string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err = tx.Exec(ctx,
		`DELETE FROM user_roles WHERE principal_id = $1 AND principal_type = 'user'`,
		userID,
	); err != nil {
		return fmt.Errorf("remove user roles: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE users
		    SET email = $1, company_id = NULL, password_hash = NULL,
		        auth_user_id = NULL, updated_at = NOW()
		  WHERE id = $2`,
		archivedEmail, userID,
	); err != nil {
		return fmt.Errorf("archive user record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit archive transaction: %w", err)
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id string) error {
	return r.DeleteTx(ctx, r.pool, id)
}

func (r *userRepository) DeleteTx(ctx context.Context, tx domain.Execer, id string) error {
	execer, ok := tx.(DBExecer)
	if !ok {
		return fmt.Errorf("invalid execer type: expected DBExecer, got %T", tx)
	}
	_, err := execer.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
