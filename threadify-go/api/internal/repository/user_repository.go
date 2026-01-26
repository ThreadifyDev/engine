package repository

import (
	"database/sql"
	"threadify-go/api/internal/models"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *models.User) error {
	query := `
		INSERT INTO users (id, company_id, email, password_hash, full_name, job_role, 
			email_verified, onboarding_completed, first_instrumentation_done, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
	`
	_, err := r.db.Exec(query, user.ID, user.CompanyID, user.Email, user.PasswordHash,
		user.FullName, user.JobRole, user.EmailVerified, user.OnboardingCompleted,
		user.FirstInstrumentationDone)
	return err
}

func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, company_id, email, password_hash, full_name, job_role, 
			email_verified, onboarding_completed, first_instrumentation_done, 
			created_at, updated_at, last_login_at
		FROM users WHERE email = $1
	`
	err := r.db.QueryRow(query, email).Scan(
		&user.ID, &user.CompanyID, &user.Email, &user.PasswordHash,
		&user.FullName, &user.JobRole, &user.EmailVerified,
		&user.OnboardingCompleted, &user.FirstInstrumentationDone,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *UserRepository) FindByID(id string) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, company_id, email, password_hash, full_name, job_role, 
			email_verified, onboarding_completed, first_instrumentation_done, 
			created_at, updated_at, last_login_at
		FROM users WHERE id = $1
	`
	err := r.db.QueryRow(query, id).Scan(
		&user.ID, &user.CompanyID, &user.Email, &user.PasswordHash,
		&user.FullName, &user.JobRole, &user.EmailVerified,
		&user.OnboardingCompleted, &user.FirstInstrumentationDone,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *UserRepository) UpdateEmailVerified(id string, verified bool) error {
	query := `UPDATE users SET email_verified = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, verified, id)
	return err
}

func (r *UserRepository) UpdateLastLogin(id string) error {
	query := `UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *UserRepository) UpdateProfile(id string, fullName, jobRole *string, onboardingCompleted bool) error {
	query := `
		UPDATE users 
		SET full_name = COALESCE($1, full_name), 
		    job_role = COALESCE($2, job_role), 
		    onboarding_completed = $3, 
		    updated_at = NOW() 
		WHERE id = $4
	`
	_, err := r.db.Exec(query, fullName, jobRole, onboardingCompleted, id)
	return err
}

func (r *UserRepository) MarkFirstInstrumentationDone(id string) error {
	query := `
		UPDATE users 
		SET first_instrumentation_done = true, 
		    updated_at = NOW() 
		WHERE id = $1
	`
	_, err := r.db.Exec(query, id)
	return err
}
