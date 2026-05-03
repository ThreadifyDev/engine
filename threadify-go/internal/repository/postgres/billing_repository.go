package postgres

import (
	"context"
	"fmt"

	shareddomain "threadify-go/shared/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSnapshotAlreadyExists = fmt.Errorf("billing snapshot already exists for this period")

type BillingRepository struct {
	pool *pgxpool.Pool
}

func NewBillingRepository(pool *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{pool: pool}
}

func (r *BillingRepository) CreateSnapshot(ctx context.Context, snapshot *shareddomain.BillingSnapshot) error {
	const query = `
		INSERT INTO billing_snapshots (
			id, company_id, reason, period_start, period_end,
			total_cents,
			provider_name, external_invoice_id,
			external_customer_id, payment_status,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (id) DO NOTHING
	`
	tag, err := r.pool.Exec(ctx, query,
		snapshot.ID, snapshot.CompanyID, snapshot.Reason,
		snapshot.PeriodStart, snapshot.PeriodEnd,
		snapshot.TotalCents,
		snapshot.ProviderName, snapshot.ExternalInvoiceID,
		snapshot.ExternalCustomerID, snapshot.PaymentStatus,
	)
	if err != nil {
		return fmt.Errorf("create billing snapshot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSnapshotAlreadyExists
	}
	return nil
}

func (r *BillingRepository) FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*shareddomain.BillingSnapshot, error) {
	const query = `
		SELECT id, company_id, reason, period_start, period_end,
			total_cents,
			provider_name, external_invoice_id,
			external_customer_id, payment_status,
			created_at
		FROM billing_snapshots
		WHERE external_invoice_id = $1
		LIMIT 1
	`
	return r.scanSnapshot(r.pool.QueryRow(ctx, query, externalInvoiceID))
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

func (r *BillingRepository) UpdateSnapshotPaymentStatus(ctx context.Context, snapshotID string, status shareddomain.PaymentStatus) error {
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
	_, err := r.pool.Exec(ctx, query, string(shareddomain.PaymentStatusPaid), externalInvoiceID)
	return err
}

func (r *BillingRepository) MarkSnapshotPaidByID(ctx context.Context, snapshotID string, externalInvoiceID string) error {
	const query = `
		UPDATE billing_snapshots 
		SET payment_status = $1, external_invoice_id = $2 
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, string(shareddomain.PaymentStatusPaid), externalInvoiceID, snapshotID)
	return err
}

func (r *BillingRepository) MarkSnapshotFailedByInvoiceID(ctx context.Context, externalInvoiceID string) error {
	const query = `
		UPDATE billing_snapshots SET payment_status = $1 WHERE external_invoice_id = $2
	`
	_, err := r.pool.Exec(ctx, query, string(shareddomain.PaymentStatusFailed), externalInvoiceID)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func (r *BillingRepository) scanSnapshot(row scannable) (*shareddomain.BillingSnapshot, error) {
	var s shareddomain.BillingSnapshot

	err := row.Scan(
		&s.ID, &s.CompanyID, &s.Reason, &s.PeriodStart, &s.PeriodEnd,
		&s.TotalCents,
		&s.ProviderName, &s.ExternalInvoiceID,
		&s.ExternalCustomerID, &s.PaymentStatus,
		&s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan billing snapshot: %w", err)
	}
	return &s, nil
}
