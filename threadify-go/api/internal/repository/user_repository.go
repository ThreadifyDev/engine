package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"threadify-go/api/internal/models"
	serror "threadify-go/shared/errors"
)

type UserRepository struct {
	db *sql.DB
}

func (r *UserRepository) GetDB() *sql.DB {
	return r.db
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

type userExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (r *UserRepository) CreateTx(execer userExecer, user *models.User) error {
	const query = `
        INSERT INTO users (id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
    `
	_, err := execer.Exec(query,
		user.ID, user.CompanyID, user.Email, user.AuthUserID,
		user.FullName, user.JobRole, user.EmailVerified,
		user.OnboardingCompleted, user.FirstInstrumentationDone,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	user := &models.User{}
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE email = $1
    `
	err := r.db.QueryRow(query, email).Scan(
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

func (r *UserRepository) ListByCompanyID(companyID string) ([]*models.User, error) {
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE company_id = $1
        ORDER BY created_at ASC
    `
	rows, err := r.db.Query(query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list users by company: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
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

func (r *UserRepository) FindByID(id string) (*models.User, error) {
	user := &models.User{}
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE id = $1
    `
	err := r.db.QueryRow(query, id).Scan(
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

func (r *UserRepository) FindByAuthUserID(authUserID string) (*models.User, error) {
	user := &models.User{}
	const query = `
        SELECT id, company_id, email, auth_user_id, full_name, job_role,
            email_verified, onboarding_completed, first_instrumentation_done,
            created_at, updated_at, last_login_at
        FROM users WHERE auth_user_id = $1
    `
	err := r.db.QueryRow(query, authUserID).Scan(
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

func (r *UserRepository) UpdateAuthUserID(id, authUserID string) error {
	_, err := r.db.Exec(`UPDATE users SET auth_user_id = $1, updated_at = NOW() WHERE id = $2`, authUserID, id)
	if err != nil {
		return fmt.Errorf("update auth user id: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateEmailVerified(id string, verified bool) error {
	_, err := r.db.Exec(`UPDATE users SET email_verified = $1, updated_at = NOW() WHERE id = $2`, verified, id)
	if err != nil {
		return fmt.Errorf("update email verified: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateLastLogin(id string) error {
	_, err := r.db.Exec(`UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateProfile(id string, fullName, jobRole *string, onboardingCompleted bool) error {
	const query = `
        UPDATE users
        SET full_name = COALESCE($1, full_name),
            job_role = COALESCE($2, job_role),
            onboarding_completed = $3,
            updated_at = NOW()
        WHERE id = $4
    `
	_, err := r.db.Exec(query, fullName, jobRole, onboardingCompleted, id)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *UserRepository) MarkFirstInstrumentationDone(id string) error {
	_, err := r.db.Exec(`UPDATE users SET first_instrumentation_done = true, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark first instrumentation done: %w", err)
	}
	return nil
}

func (r *UserRepository) GetPasswordChangedAt(id string) (*time.Time, error) {
	var changedAt *time.Time
	err := r.db.QueryRow(`SELECT password_changed_at FROM users WHERE id = $1`, id).Scan(&changedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, serror.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get password changed at: %w", err)
	}
	return changedAt, nil
}

func (r *UserRepository) GetPasswordHash(email string) (string, error) {
	var passwordHash string
	err := r.db.QueryRow(`SELECT password_hash FROM users WHERE email = $1`, email).Scan(&passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", serror.ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get password hash: %w", err)
	}
	return passwordHash, nil
}

func (r *UserRepository) ClearPasswordHash(userID string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash = NULL WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("clear password hash: %w", err)
	}
	return nil
}
func (r *UserRepository) Delete(id string) error {
	return r.DeleteTx(r.db, id)
}

func (r *UserRepository) DeleteTx(execer userExecer, id string) error {
	_, err := execer.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
