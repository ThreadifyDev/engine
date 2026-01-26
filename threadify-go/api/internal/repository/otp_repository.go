package repository

import (
	"database/sql"
	"threadify-go/api/internal/models"
	"time"
)

type OTPRepository struct {
	db *sql.DB
}

func NewOTPRepository(db *sql.DB) *OTPRepository {
	return &OTPRepository{db: db}
}

func (r *OTPRepository) Create(otp *models.OTPCode) error {
	query := `
		INSERT INTO otp_codes (id, email, code, expires_at, verified, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err := r.db.Exec(query, otp.ID, otp.Email, otp.Code, otp.ExpiresAt, otp.Verified)
	return err
}

func (r *OTPRepository) FindLatestValidOTP(email string) (*models.OTPCode, error) {
	otp := &models.OTPCode{}
	query := `
		SELECT id, email, code, expires_at, verified, created_at
		FROM otp_codes 
		WHERE email = $1 AND verified = false AND expires_at > $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	err := r.db.QueryRow(query, email, time.Now()).Scan(
		&otp.ID, &otp.Email, &otp.Code, &otp.ExpiresAt, &otp.Verified, &otp.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return otp, err
}

func (r *OTPRepository) MarkAsVerified(id string) error {
	query := `UPDATE otp_codes SET verified = true WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *OTPRepository) DeleteExpired() error {
	query := `DELETE FROM otp_codes WHERE expires_at < NOW()`
	_, err := r.db.Exec(query)
	return err
}
