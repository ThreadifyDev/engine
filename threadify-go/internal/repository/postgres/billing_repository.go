package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

var ErrSnapshotAlreadyExists = fmt.Errorf("billing snapshot already exists for this period")

type BillingRepository struct {
	pool *pgxpool.Pool
}

func NewBillingRepository(pool *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{pool: pool}
}

func (r *BillingRepository) CreateSnapshot(ctx context.Context, snapshot *models.BillingSnapshot) error {
	lineItemsJSON, err := json.Marshal(snapshot.LineItems)
	if err != nil {
		return fmt.Errorf("marshal line items: %w", err)
	}

	const query = `
		INSERT INTO billing_snapshots (
			id, company_id, tier, reason, period_start, period_end, is_cycle_end,
			ingress_balance_final, egress_balance_final,
			max_ingress, max_egress,
			line_items_json, total_cents,
			provider_name, external_invoice_id,
			external_customer_id, external_subscription_id, payment_status,
			consecutive_overage_count,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
		ON CONFLICT (company_id, period_end) DO NOTHING
	`
	tag, err := r.pool.Exec(ctx, query,
		snapshot.ID, snapshot.CompanyID, snapshot.Tier, snapshot.Reason,
		snapshot.PeriodStart, snapshot.PeriodEnd, snapshot.IsCycleEnd,
		snapshot.IngressBalanceFinal, snapshot.EgressBalanceFinal,
		snapshot.MaxIngress, snapshot.MaxEgress,
		lineItemsJSON, snapshot.TotalCents,
		snapshot.ProviderName, snapshot.ExternalInvoiceID,
		snapshot.ExternalCustomerID, snapshot.ExternalSubscriptionID, snapshot.PaymentStatus,
		snapshot.ConsecutiveOverageCount,
	)
	if err != nil {
		return fmt.Errorf("create billing snapshot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSnapshotAlreadyExists
	}
	return nil
}

func (r *BillingRepository) FindLatestSnapshot(ctx context.Context, companyID string) (*models.BillingSnapshot, error) {
	const query = `
		SELECT id, company_id, tier, reason, period_start, period_end, is_cycle_end,
			ingress_balance_final, egress_balance_final,
			max_ingress, max_egress,
			line_items_json, total_cents,
			provider_name, external_invoice_id,
			external_customer_id, external_subscription_id, payment_status,
			consecutive_overage_count, created_at
		FROM billing_snapshots
		WHERE company_id = $1
		ORDER BY period_end DESC
		LIMIT 1
	`
	return r.scanSnapshot(r.pool.QueryRow(ctx, query, companyID))
}

func (r *BillingRepository) FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*models.BillingSnapshot, error) {
	const query = `
		SELECT id, company_id, tier, reason, period_start, period_end, is_cycle_end,
			ingress_balance_final, egress_balance_final,
			max_ingress, max_egress,
			line_items_json, total_cents,
			provider_name, external_invoice_id,
			external_customer_id, external_subscription_id, payment_status,
			consecutive_overage_count, created_at
		FROM billing_snapshots
		WHERE external_invoice_id = $1
		LIMIT 1
	`
	return r.scanSnapshot(r.pool.QueryRow(ctx, query, externalInvoiceID))
}

func (r *BillingRepository) ListSnapshots(ctx context.Context, companyID string, limit int) ([]models.BillingSnapshot, error) {
	const query = `
		SELECT id, company_id, tier, reason, period_start, period_end, is_cycle_end,
			ingress_balance_final, egress_balance_final,
			max_ingress, max_egress,
			line_items_json, total_cents,
			provider_name, external_invoice_id,
			external_customer_id, external_subscription_id, payment_status,
			consecutive_overage_count, created_at
		FROM billing_snapshots
		WHERE company_id = $1
		ORDER BY period_end DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, companyID, limit)
	if err != nil {
		return nil, fmt.Errorf("list billing snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []models.BillingSnapshot
	for rows.Next() {
		s, err := r.scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, *s)
	}
	return snapshots, rows.Err()
}

func (r *BillingRepository) UpdateSnapshotInvoiceID(ctx context.Context, snapshotID string, invoiceID string) error {
	const query = `
		UPDATE billing_snapshots
		SET external_invoice_id = $1
		WHERE id = $2
	`
	tag, err := r.pool.Exec(ctx, query, invoiceID, snapshotID)
	if err != nil {
		return fmt.Errorf("update snapshot invoice id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("snapshot %s not found", snapshotID)
	}
	return nil
}

func (r *BillingRepository) UpdateSnapshotPaymentStatus(ctx context.Context, snapshotID string, status models.PaymentStatus) error {
	const query = `
		UPDATE billing_snapshots
		SET payment_status = $1
		WHERE id = $2
	`
	tag, err := r.pool.Exec(ctx, query, status, snapshotID)
	if err != nil {
		return fmt.Errorf("update snapshot payment status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("snapshot %s not found", snapshotID)
	}
	return nil
}

func (r *BillingRepository) MarkSnapshotPaidByInvoiceID(ctx context.Context, externalInvoiceID string) error {
	const query = `
		UPDATE billing_snapshots SET payment_status = $1 WHERE external_invoice_id = $2
	`
	_, err := r.pool.Exec(ctx, query, string(models.PaymentStatusPaid), externalInvoiceID)
	return err
}

func (r *BillingRepository) MarkSnapshotFailedByInvoiceID(ctx context.Context, externalInvoiceID string) error {
	const query = `
		UPDATE billing_snapshots SET payment_status = $1 WHERE external_invoice_id = $2
	`
	_, err := r.pool.Exec(ctx, query, string(models.PaymentStatusFailed), externalInvoiceID)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func (r *BillingRepository) scanSnapshot(row scannable) (*models.BillingSnapshot, error) {
	var s models.BillingSnapshot
	var lineItemsJSON []byte

	err := row.Scan(
		&s.ID, &s.CompanyID, &s.Tier, &s.Reason, &s.PeriodStart, &s.PeriodEnd, &s.IsCycleEnd,
		&s.IngressBalanceFinal, &s.EgressBalanceFinal,
		&s.MaxIngress, &s.MaxEgress,
		&lineItemsJSON, &s.TotalCents,
		&s.ProviderName, &s.ExternalInvoiceID,
		&s.ExternalCustomerID, &s.ExternalSubscriptionID, &s.PaymentStatus,
		&s.ConsecutiveOverageCount, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan billing snapshot: %w", err)
	}
	if err := json.Unmarshal(lineItemsJSON, &s.LineItems); err != nil {
		return nil, fmt.Errorf("unmarshal line items: %w", err)
	}
	return &s, nil
}
