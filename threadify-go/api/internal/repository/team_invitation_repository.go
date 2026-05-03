package repository

import (
	"context"
	"fmt"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/ports"
	serror "threadify-go/shared/errors"
)

type teamInvitationRepository struct {
	pool ports.SQLExecutor
}

func NewTeamInvitationRepository(pool ports.SQLExecutor) domain.TeamInvitationRepository {
	return &teamInvitationRepository{pool: pool}
}

// Create inserts a new team invitation
func (r *teamInvitationRepository) Create(ctx context.Context, invitation *domain.TeamInvitation) error {
	return r.CreateTx(ctx, r.pool, invitation)
}

// CreateTx inserts a new team invitation within a transaction
func (r *teamInvitationRepository) CreateTx(ctx context.Context, tx domain.ExecContext, invitation *domain.TeamInvitation) error {
	execer, ok := tx.(ports.SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid execer type: expected ports.SQLExecutor, got %T", tx)
	}

	const query = `
		INSERT INTO team_invitations
			(id, company_id, email, role, invited_by, status, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err := execer.Exec(ctx, query,
		invitation.ID,
		invitation.CompanyID,
		invitation.Email,
		invitation.Role,
		invitation.InvitedBy,
		invitation.Status,
		invitation.Token,
		invitation.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert team invitation: %w", err)
	}
	return nil
}

// GetByToken retrieves an invitation by token
func (r *teamInvitationRepository) GetByToken(ctx context.Context, token string) (*domain.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE token = $1
	`
	invitation := &domain.TeamInvitation{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&invitation.ID,
		&invitation.CompanyID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.Status,
		&invitation.Token,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
		&invitation.AcceptedAt,
		&invitation.AcceptedByUserID,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get team invitation by token: %w", err)
	}
	return invitation, nil
}

// GetByID retrieves an invitation by ID
func (r *teamInvitationRepository) GetByID(ctx context.Context, id string) (*domain.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE id = $1
	`
	invitation := &domain.TeamInvitation{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&invitation.ID,
		&invitation.CompanyID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.Status,
		&invitation.Token,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
		&invitation.AcceptedAt,
		&invitation.AcceptedByUserID,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get team invitation by id: %w", err)
	}
	return invitation, nil
}

// MarkAccepted marks an invitation as accepted
func (r *teamInvitationRepository) MarkAccepted(ctx context.Context, invitationID, userID string) error {
	return r.MarkAcceptedTx(ctx, r.pool, invitationID, userID)
}

// MarkAcceptedTx marks an invitation as accepted within a transaction
func (r *teamInvitationRepository) MarkAcceptedTx(ctx context.Context, tx domain.ExecContext, invitationID, userID string) error {
	execer, ok := tx.(ports.SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid execer type: expected ports.SQLExecutor, got %T", tx)
	}

	const query = `
		UPDATE team_invitations
		SET status = $1, accepted_at = NOW(), accepted_by_user_id = $2
		WHERE id = $3
	`
	_, err := execer.Exec(ctx, query, "accepted", userID, invitationID)
	if err != nil {
		return fmt.Errorf("mark invitation accepted: %w", err)
	}
	return nil
}

// UpdateStatus updates the status of an invitation
func (r *teamInvitationRepository) UpdateStatus(ctx context.Context, invitationID, status string) error {
	const query = `
		UPDATE team_invitations
		SET status = $1
		WHERE id = $2
	`
	result, err := r.pool.Exec(ctx, query, status, invitationID)
	if err != nil {
		return fmt.Errorf("update invitation status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return serror.ErrInvitationNotFound
	}
	return nil
}

// Delete removes an invitation
func (r *teamInvitationRepository) Delete(ctx context.Context, invitationID string) error {
	return r.DeleteTx(ctx, r.pool, invitationID)
}

// DeleteTx removes an invitation within a transaction
func (r *teamInvitationRepository) DeleteTx(ctx context.Context, tx domain.ExecContext, invitationID string) error {
	execer, ok := tx.(ports.SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid execer type: expected ports.SQLExecutor, got %T", tx)
	}

	const query = `DELETE FROM team_invitations WHERE id = $1`
	_, err := execer.Exec(ctx, query, invitationID)
	if err != nil {
		return fmt.Errorf("delete invitation: %w", err)
	}
	return nil
}

// RefreshInvitation updates the token and expiry date for an existing invitation
func (r *teamInvitationRepository) RefreshInvitation(ctx context.Context, invitationID, newToken string, expiresAt time.Time) error {
	const query = `
		UPDATE team_invitations
		SET token = $1, expires_at = $2
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, newToken, expiresAt, invitationID)
	if err != nil {
		return fmt.Errorf("refresh invitation: %w", err)
	}
	return nil
}

// GetPendingByCompanyAndEmail retrieves pending invitations for a company and email
func (r *teamInvitationRepository) GetPendingByCompanyAndEmail(ctx context.Context, companyID, email string) (*domain.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE company_id = $1 AND email = $2 AND status = 'pending' AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`
	invitation := &domain.TeamInvitation{}
	err := r.pool.QueryRow(ctx, query, companyID, email).Scan(
		&invitation.ID,
		&invitation.CompanyID,
		&invitation.Email,
		&invitation.Role,
		&invitation.InvitedBy,
		&invitation.Status,
		&invitation.Token,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
		&invitation.AcceptedAt,
		&invitation.AcceptedByUserID,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get pending invitation: %w", err)
	}
	return invitation, nil
}

// ListByCompany retrieves all invitations for a company
func (r *teamInvitationRepository) ListByCompany(ctx context.Context, companyID string) ([]*domain.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE company_id = $1 AND status = 'pending'
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	var invitations []*domain.TeamInvitation
	for rows.Next() {
		invitation := &domain.TeamInvitation{}
		if err := rows.Scan(
			&invitation.ID,
			&invitation.CompanyID,
			&invitation.Email,
			&invitation.Role,
			&invitation.InvitedBy,
			&invitation.Status,
			&invitation.Token,
			&invitation.ExpiresAt,
			&invitation.CreatedAt,
			&invitation.AcceptedAt,
			&invitation.AcceptedByUserID,
		); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		invitations = append(invitations, invitation)
	}
	return invitations, rows.Err()
}

// DeleteExpired deletes expired invitations
func (r *teamInvitationRepository) DeleteExpired(ctx context.Context) (int64, error) {
	const query = `DELETE FROM team_invitations WHERE status = 'pending' AND expires_at < NOW()`
	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("delete expired invitations: %w", err)
	}
	return result.RowsAffected(), nil
}
