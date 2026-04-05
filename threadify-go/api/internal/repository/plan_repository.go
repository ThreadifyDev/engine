package repository

import (
	"context"
	"database/sql"
	"fmt"

	"threadify-go/shared/billing"
)

type PlanRepository struct {
	db *sql.DB
}

func NewPlanRepository(db *sql.DB) *PlanRepository {
	return &PlanRepository{db: db}
}

func (r *PlanRepository) GetCreditAccount(ctx context.Context, companyID string) (*billing.CreditAccount, error) {
	const query = `
		SELECT 
			ca.id, ca.company_id, ca.billing_cycle_start,
			ca.credit_balance_millicents, ca.credit_min_balance_millicents, ca.credit_max_monthly_charge_millicents,
			ca.credit_auto_topup_millicents, ca.credit_monthly_charged_millicents, ca.rate_limit_tps, ca.payload_limit_bytes,
			ca.created_at, ca.updated_at,
			c.external_customer_id
		FROM credit_accounts ca
		JOIN companies c ON c.id = ca.company_id
		WHERE ca.company_id = $1
		ORDER BY ca.billing_cycle_start DESC
		LIMIT 1
	`
	account := &billing.CreditAccount{}
	err := r.db.QueryRowContext(ctx, query, companyID).Scan(
		&account.ID, &account.CompanyID, &account.BillingCycleStart,
		&account.CreditBalanceMillicents, &account.CreditMinBalanceMillicents, &account.CreditMaxMonthlyChargeMillicents,
		&account.CreditAutoTopupMillicents, &account.CreditMonthlyChargedMillicents,
		&account.RateLimitTPS, &account.PayloadLimitBytes,
		&account.CreatedAt, &account.UpdatedAt,
		&account.ExternalCustomerID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get credit account: %w", err)
	}
	return account, nil
}

func (r *PlanRepository) GetExternalCustomerID(ctx context.Context, companyID string) (string, error) {
	const query = `SELECT external_customer_id FROM companies WHERE id = $1`
	var externalID sql.NullString
	err := r.db.QueryRowContext(ctx, query, companyID).Scan(&externalID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return externalID.String, nil
}

func (r *PlanRepository) SetExternalCustomerID(ctx context.Context, companyID, externalCustomerID string) error {
	const query = `UPDATE companies SET external_customer_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, externalCustomerID, companyID)
	return err
}

func (r *PlanRepository) FindCompanyByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	const query = `SELECT id FROM companies WHERE external_customer_id = $1 LIMIT 1`
	var id string
	err := r.db.QueryRowContext(ctx, query, externalCustomerID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (r *PlanRepository) CreateCreditAccount(ctx context.Context, account *billing.CreditAccount) error {
	const query = `
		INSERT INTO credit_accounts (
			id, company_id, billing_cycle_start,
			credit_balance_millicents, credit_min_balance_millicents, credit_max_monthly_charge_millicents,
			credit_auto_topup_millicents, credit_monthly_charged_millicents,
			rate_limit_tps, payload_limit_bytes,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
	`
	_, err := r.db.ExecContext(ctx, query,
		account.ID, account.CompanyID, account.BillingCycleStart,
		account.CreditBalanceMillicents, account.CreditMinBalanceMillicents, account.CreditMaxMonthlyChargeMillicents,
		account.CreditAutoTopupMillicents, account.CreditMonthlyChargedMillicents,
		account.RateLimitTPS, account.PayloadLimitBytes,
	)
	if err != nil {
		return fmt.Errorf("create credit account: %w", err)
	}
	return nil
}

func (r *PlanRepository) UpdateCompanyLimits(ctx context.Context, companyID string, rateLimit *int64, payloadLimit *int64) error {
	const query = `
		UPDATE credit_accounts
		SET rate_limit_tps = $1, payload_limit_bytes = $2, updated_at = NOW()
		WHERE company_id = $3
		AND billing_cycle_start = (
			SELECT MAX(billing_cycle_start) FROM credit_accounts WHERE company_id = $3
		)
	`
	_, err := r.db.ExecContext(ctx, query, rateLimit, payloadLimit, companyID)
	if err != nil {
		return fmt.Errorf("update company limits: %w", err)
	}
	return nil
}
func (r *PlanRepository) UpdateTopupSettings(ctx context.Context, companyID string, autoTopupAmount, minBalance int64) error {
	const query = `
		WITH latest AS (
			SELECT id FROM credit_accounts
			WHERE company_id = $1
			ORDER BY billing_cycle_start DESC
			LIMIT 1
			FOR UPDATE
		)
		UPDATE credit_accounts
		SET credit_auto_topup_millicents = $2,
			credit_min_balance_millicents = $3,
			updated_at = NOW()
		FROM latest
		WHERE credit_accounts.id = latest.id
	`
	_, err := r.db.ExecContext(ctx, query, companyID, autoTopupAmount, minBalance)
	return err
}

func (r *PlanRepository) UpdateCumulativeMonthlyCharge(ctx context.Context, id string, amount int64) error {
	const query = `UPDATE credit_accounts SET credit_monthly_charged_millicents = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, amount, id)
	return err
}

func (r *PlanRepository) UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error {
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
	result, err := r.db.ExecContext(ctx, query, companyID, maxMonthlyMillicents)
	if err != nil {
		return fmt.Errorf("update max monthly charge: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no credit account found for company %s", companyID)
	}
	return nil
}

func (r *PlanRepository) UpdateMonthlyLimit(ctx context.Context, companyID string, maxMonthlyMillicents int64) error {
	const query = `
		UPDATE credit_accounts 
		SET credit_max_monthly_charge_millicents = $1, updated_at = NOW()
		WHERE company_id = $2
		AND billing_cycle_start = (
			SELECT MAX(billing_cycle_start) FROM credit_accounts WHERE company_id = $2
		)
	`
	_, err := r.db.ExecContext(ctx, query, maxMonthlyMillicents, companyID)
	if err != nil {
		return fmt.Errorf("update monthly limit: %w", err)
	}
	return nil
}
