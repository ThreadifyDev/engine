package repository

import (
	"context"
	"errors"
	"fmt"

	serror "threadify-go/shared/errors"
	"threadify-go/shared/models"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanRepo struct {
	pool *pgxpool.Pool
}

func NewPlanRepo(pool *pgxpool.Pool) *PlanRepo {
	return &PlanRepo{pool: pool}
}

func (r *PlanRepo) GetCreditAccount(ctx context.Context, companyID string) (*models.CreditAccount, error) {
	const query = `
		SELECT 
			ca.id, ca.company_id, ca.billing_cycle_start,
			ca.credit_balance_millicents, ca.credit_min_balance_millicents, ca.credit_max_monthly_charge_millicents,
			ca.credit_auto_topup_millicents, ca.credit_monthly_charged_millicents, ca.rate_limit_tps, ca.payload_limit_bytes,
			ca.created_at, ca.updated_at,
			COALESCE(c.external_customer_id, '')
		FROM credit_accounts ca
		JOIN companies c ON c.id = ca.company_id
		WHERE ca.company_id = $1
		ORDER BY ca.billing_cycle_start DESC, ca.created_at DESC
		LIMIT 1
	`
	account := &models.CreditAccount{}
	err := r.pool.QueryRow(ctx, query, companyID).Scan(
		&account.ID, &account.CompanyID, &account.BillingCycleStart,
		&account.CreditBalanceMillicents, &account.CreditMinBalanceMillicents, &account.CreditMaxMonthlyChargeMillicents,
		&account.CreditAutoTopupMillicents, &account.CreditMonthlyChargedMillicents,
		&account.RateLimitTPS, &account.PayloadLimitBytes,
		&account.CreatedAt, &account.UpdatedAt,
		&account.ExternalCustomerID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get credit account: %w", err)
	}
	return account, nil
}

func (r *PlanRepo) GetExternalCustomerID(ctx context.Context, companyID string) (string, error) {
	const query = `SELECT COALESCE(external_customer_id, '') FROM companies WHERE id = $1`
	var externalID string
	err := r.pool.QueryRow(ctx, query, companyID).Scan(&externalID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get external customer id: %w", err)
	}
	return externalID, nil
}

func (r *PlanRepo) SetExternalCustomerID(ctx context.Context, companyID, externalCustomerID string) error {
	const query = `UPDATE companies SET external_customer_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, externalCustomerID, companyID)
	return err
}

func (r *PlanRepo) FindCompanyByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	const query = `SELECT id FROM companies WHERE external_customer_id = $1 LIMIT 1`
	var id string
	err := r.pool.QueryRow(ctx, query, externalCustomerID).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (r *PlanRepo) ListCompaniesForRollover(ctx context.Context) (map[string]string, error) {
	const query = `
		SELECT DISTINCT c.id, COALESCE(c.external_customer_id, '')
		FROM companies c
		WHERE EXISTS (
			SELECT 1 FROM credit_accounts ca WHERE ca.company_id = c.id
		)
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	companies := make(map[string]string)
	for rows.Next() {
		var id, extID string
		if err := rows.Scan(&id, &extID); err != nil {
			return nil, err
		}
		companies[id] = extID
	}
	return companies, rows.Err()
}

func (r *PlanRepo) CreateCreditAccount(ctx context.Context, account *models.CreditAccount) error {
	const query = `
		INSERT INTO credit_accounts (
			id, company_id, billing_cycle_start,
			credit_balance_millicents, credit_min_balance_millicents, credit_max_monthly_charge_millicents,
			credit_auto_topup_millicents, credit_monthly_charged_millicents,
			rate_limit_tps, payload_limit_bytes,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
	`
	_, err := r.pool.Exec(ctx, query,
		account.ID, account.CompanyID, account.BillingCycleStart,
		account.CreditBalanceMillicents, account.CreditMinBalanceMillicents, account.CreditMaxMonthlyChargeMillicents,
		account.CreditAutoTopupMillicents, account.CreditMonthlyChargedMillicents,
		account.RateLimitTPS, account.PayloadLimitBytes,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return serror.ErrDuplicateCreditAccount
		}
		return fmt.Errorf("create credit account: %w", err)
	}
	return nil
}

func (r *PlanRepo) UpdateCumulativeMonthlyCharge(ctx context.Context, id string, amount int64) error {
	const query = `UPDATE credit_accounts SET credit_monthly_charged_millicents = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, amount, id)
	return err
}

func (r *PlanRepo) UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error {
	const query = `
		WITH latest AS (
			SELECT id FROM credit_accounts
			WHERE company_id = $1
			ORDER BY billing_cycle_start DESC
			LIMIT 1
			FOR UPDATE
		)
		UPDATE credit_accounts
		SET credit_max_monthly_charge_millicents = $2,
			updated_at = NOW()
		FROM latest
		WHERE credit_accounts.id = latest.id
	`
	tag, err := r.pool.Exec(ctx, query, companyID, maxMonthlyMillicents)
	if err != nil {
		return fmt.Errorf("update max monthly charge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no credit account found for company %s", companyID)
	}
	return nil
}

func (r *PlanRepo) UpdateTopupSettings(ctx context.Context, companyID string, autoTopupAmount, minBalance int64) error {
	const query = `
        WITH latest AS (
            SELECT id FROM credit_accounts
            WHERE company_id = $1
            ORDER BY billing_cycle_start DESC
            LIMIT 1
            FOR UPDATE
        )
        UPDATE credit_accounts
        SET credit_auto_topup_millicents         = $2,
            credit_min_balance_millicents        = $3,
            updated_at = NOW()
        FROM latest
        WHERE credit_accounts.id = latest.id
    `
	tag, err := r.pool.Exec(ctx, query, companyID, autoTopupAmount, minBalance)
	if err != nil {
		return fmt.Errorf("update topup settings: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no credit account found for company %s", companyID)
	}
	return nil
}
