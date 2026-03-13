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

func (r *PlanRepository) GetCompanyPlan(ctx context.Context, companyID string) (*billing.CompanyPlan, error) {
	query := `
		SELECT id, company_id, subscription_tier, billing_cycle, external_customer_id, external_subscription_id, status, billing_start, billing_end, created_at, updated_at
		FROM company_plans
		WHERE company_id = $1
	`
	plan := &billing.CompanyPlan{}
	err := r.db.QueryRowContext(ctx, query, companyID).Scan(
		&plan.ID, &plan.CompanyID, &plan.SubscriptionTier, &plan.BillingCycle,
		&plan.ExternalCustomerID, &plan.ExternalSubscriptionID,
		&plan.Status, &plan.BillingStart, &plan.BillingEnd,
		&plan.CreatedAt, &plan.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Return nil if no plan is found
	}
	if err != nil {
		return nil, fmt.Errorf("get company plan: %w", err)
	}
	return plan, nil
}

func (r *PlanRepository) GetCurrentUsageMeter(ctx context.Context, companyID string) (*billing.UsageMeter, error) {
	query := `
		SELECT id, company_id, subscription_tier, billing_cycle_start, billing_end,
			bandwidth_ingress_balance, bandwidth_egress_balance,
			max_bandwidth_ingress, max_bandwidth_egress,
			max_team_seats, max_contract_limit, max_rate_limit,
			max_payload_bytes, hot_storage_days, cold_storage_days, support,
			created_at, updated_at
		FROM usage_meters
		WHERE company_id = $1
		ORDER BY billing_cycle_start DESC
		LIMIT 1
	`
	meter := &billing.UsageMeter{}
	err := r.db.QueryRowContext(ctx, query, companyID).Scan(
		&meter.ID, &meter.CompanyID, &meter.SubscriptionTier,
		&meter.BillingCycleStart, &meter.BillingEnd,
		&meter.BandwidthIngressBalance, &meter.BandwidthEgressBalance,
		&meter.MaxBandwidthIngress, &meter.MaxBandwidthEgress,
		&meter.MaxTeamSeats, &meter.MaxContractLimit, &meter.MaxRateLimit,
		&meter.MaxPayloadBytes, &meter.HotStorageDays, &meter.ColdStorageDays, &meter.Support,
		&meter.CreatedAt, &meter.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Return nil if no meter is found
	}
	if err != nil {
		return nil, fmt.Errorf("get current usage meter: %w", err)
	}
	return meter, nil
}

func (r *PlanRepository) MarkPlanCancelled(ctx context.Context, companyID string) error {
	query := `UPDATE company_plans SET status = $1, updated_at = NOW() WHERE company_id = $2`
	_, err := r.db.ExecContext(ctx, query, billing.StatusCancelled, companyID)
	return err
}
