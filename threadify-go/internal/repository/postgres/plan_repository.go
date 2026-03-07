package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

type PlanRepository struct {
	pool *pgxpool.Pool
}

func NewPlanRepository(pool *pgxpool.Pool) *PlanRepository {
	return &PlanRepository{pool: pool}
}

func (r *PlanRepository) CreatePlan(ctx context.Context, plan *models.CompanyPlan) error {
	const query = `
		INSERT INTO company_plans (id, company_id, subscription_tier, billing_cycle, external_customer_id, external_subscription_id, billing_start, billing_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err := r.pool.Exec(ctx, query, plan.ID, plan.CompanyID, plan.SubscriptionTier, plan.BillingCycle, plan.ExternalCustomerID, plan.ExternalSubscriptionID, plan.BillingStart, plan.BillingEnd)
	if err != nil {
		return fmt.Errorf("create company plan: %w", err)
	}
	return nil
}

func (r *PlanRepository) FindPlanByCompanyID(ctx context.Context, companyID string) (*models.CompanyPlan, error) {
	plan := &models.CompanyPlan{}
	const query = `
		SELECT id, company_id, subscription_tier, billing_cycle, external_customer_id, external_subscription_id, billing_start, billing_end, created_at, updated_at
		FROM company_plans WHERE company_id = $1
	`
	err := r.pool.QueryRow(ctx, query, companyID).Scan(
		&plan.ID, &plan.CompanyID, &plan.SubscriptionTier, &plan.BillingCycle,
		&plan.ExternalCustomerID, &plan.ExternalSubscriptionID,
		&plan.BillingStart, &plan.BillingEnd, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find plan by company id: %w", err)
	}
	return plan, nil
}

func (r *PlanRepository) UpdatePlanTier(ctx context.Context, companyID string, tier models.PlanTier, billingCycle models.BillingCycle, externalCustID, externalSubID string, billingStart, billingEnd time.Time) error {
	const query = `
		UPDATE company_plans
		SET subscription_tier = $1, billing_cycle = $2, external_customer_id = $3, external_subscription_id = $4, billing_start = $5, billing_end = $6, updated_at = NOW()
		WHERE company_id = $7
	`
	_, err := r.pool.Exec(ctx, query, tier, billingCycle, externalCustID, externalSubID, billingStart, billingEnd, companyID)
	if err != nil {
		return fmt.Errorf("update plan tier: %w", err)
	}
	return nil
}

func (r *PlanRepository) CreateUsageMeter(ctx context.Context, meter *models.UsageMeter) error {
	const query = `
		INSERT INTO usage_meters (
			id, company_id, subscription_tier, billing_cycle_start, billing_end,
			bandwidth_ingress_balance, bandwidth_egress_balance,
			max_bandwidth_ingress, max_bandwidth_egress,
			max_team_seats, max_contract_limit, max_rate_limit,
			max_payload_bytes,
			hot_storage_days, cold_storage_days, support,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW())
	`
	_, err := r.pool.Exec(ctx, query,
		meter.ID, meter.CompanyID, string(meter.SubscriptionTier), meter.BillingCycleStart, meter.BillingEnd,
		meter.BandwidthIngressBalance, meter.BandwidthEgressBalance,
		meter.MaxBandwidthIngress, meter.MaxBandwidthEgress,
		meter.MaxTeamSeats, meter.MaxContractLimit, meter.MaxRateLimit,
		meter.MaxPayloadBytes,
		meter.HotStorageDays, meter.ColdStorageDays, meter.Support,
	)
	if err != nil {
		return fmt.Errorf("create usage meter: %w", err)
	}
	return nil
}

func (r *PlanRepository) FindCurrentUsageMeter(ctx context.Context, companyID string) (*models.UsageMeter, error) {
	meter := &models.UsageMeter{}
	const query = `
		SELECT m.id, m.company_id, m.subscription_tier, m.billing_cycle_start,
			m.bandwidth_ingress_balance, m.bandwidth_egress_balance,
			m.max_bandwidth_ingress, m.max_bandwidth_egress,
			m.max_team_seats, m.max_contract_limit, m.max_rate_limit,
			m.max_payload_bytes,
			m.hot_storage_days, m.cold_storage_days, m.support,
			m.billing_end,
			m.created_at, m.updated_at
		FROM usage_meters m
		WHERE m.company_id = $1
		ORDER BY m.billing_cycle_start DESC
		LIMIT 1
	`
	err := r.pool.QueryRow(ctx, query, companyID).Scan(
		&meter.ID, &meter.CompanyID, &meter.SubscriptionTier, &meter.BillingCycleStart,
		&meter.BandwidthIngressBalance, &meter.BandwidthEgressBalance,
		&meter.MaxBandwidthIngress, &meter.MaxBandwidthEgress,
		&meter.MaxTeamSeats, &meter.MaxContractLimit, &meter.MaxRateLimit,
		&meter.MaxPayloadBytes,
		&meter.HotStorageDays, &meter.ColdStorageDays, &meter.Support,
		&meter.BillingEnd,
		&meter.CreatedAt, &meter.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find current usage meter: %w", err)
	}
	return meter, nil
}

// DecrementBandwidthIngress always decrements (allows negative for overage tiers).
func (r *PlanRepository) DecrementBandwidthIngress(ctx context.Context, companyID string, count int64) (int64, error) {
	var newBalance int64
	const query = `
		UPDATE usage_meters
		SET bandwidth_ingress_balance = bandwidth_ingress_balance - $1, updated_at = NOW()
		WHERE company_id = $2
		  AND billing_cycle_start = (SELECT MAX(billing_cycle_start) FROM usage_meters WHERE company_id = $2)
		RETURNING bandwidth_ingress_balance
	`
	err := r.pool.QueryRow(ctx, query, count, companyID).Scan(&newBalance)
	if err != nil {
		return 0, fmt.Errorf("decrement bandwidth ingress: %w", err)
	}
	return newBalance, nil
}

// DecrementBandwidthIngressStrict fails if balance would go below 0 (hard cap tiers).
func (r *PlanRepository) DecrementBandwidthIngressStrict(ctx context.Context, companyID string, count int64) (int64, error) {
	var newBalance int64
	const query = `
		UPDATE usage_meters
		SET bandwidth_ingress_balance = bandwidth_ingress_balance - $1, updated_at = NOW()
		WHERE company_id = $2
		  AND billing_cycle_start = (SELECT MAX(billing_cycle_start) FROM usage_meters WHERE company_id = $2)
		  AND bandwidth_ingress_balance >= $1
		RETURNING bandwidth_ingress_balance
	`
	err := r.pool.QueryRow(ctx, query, count, companyID).Scan(&newBalance)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("bandwidth ingress limit exceeded")
	}
	if err != nil {
		return 0, fmt.Errorf("decrement bandwidth ingress strict: %w", err)
	}
	return newBalance, nil
}

// DecrementBandwidthEgress always decrements (both tiers have egress overage).
func (r *PlanRepository) DecrementBandwidthEgress(ctx context.Context, companyID string, bytes int64) (int64, error) {
	var newBalance int64
	const query = `
		UPDATE usage_meters
		SET bandwidth_egress_balance = bandwidth_egress_balance - $1, updated_at = NOW()
		WHERE company_id = $2
		  AND billing_cycle_start = (SELECT MAX(billing_cycle_start) FROM usage_meters WHERE company_id = $2)
		RETURNING bandwidth_egress_balance
	`
	err := r.pool.QueryRow(ctx, query, bytes, companyID).Scan(&newBalance)
	if err != nil {
		return 0, fmt.Errorf("decrement bandwidth egress: %w", err)
	}
	return newBalance, nil
}

func (r *PlanRepository) ListActivePlans(ctx context.Context) ([]models.CompanyPlan, error) {
	const query = `
		SELECT id, company_id, subscription_tier, billing_cycle, billing_start, billing_end, created_at, updated_at
		FROM company_plans
		ORDER BY billing_end ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active plans: %w", err)
	}
	defer rows.Close()

	var plans []models.CompanyPlan
	for rows.Next() {
		var p models.CompanyPlan
		if err := rows.Scan(&p.ID, &p.CompanyID, &p.SubscriptionTier, &p.BillingCycle,
			&p.BillingStart, &p.BillingEnd, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan company plan: %w", err)
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}
