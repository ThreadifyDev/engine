package repository

import (
	"database/sql"
	"fmt"

	"threadify-go/api/internal/models"
)

type TeamInvitationRepository struct {
	db *sql.DB
}

func NewTeamInvitationRepository(db *sql.DB) *TeamInvitationRepository {
	return &TeamInvitationRepository{db: db}
}

type teamInvitationExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Create inserts a new team invitation
func (r *TeamInvitationRepository) Create(invitation *models.TeamInvitation) error {
	return r.CreateTx(nil, invitation)
}

// CreateTx inserts a new team invitation within a transaction
func (r *TeamInvitationRepository) CreateTx(tx *sql.Tx, invitation *models.TeamInvitation) error {
	const query = `
		INSERT INTO team_invitations
			(id, company_id, email, role, invited_by, status, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	var execer teamInvitationExecer = r.db
	if tx != nil {
		execer = tx
	}

	_, err := execer.Exec(query,
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
func (r *TeamInvitationRepository) GetByToken(token string) (*models.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE token = $1
	`
	invitation := &models.TeamInvitation{}
	err := r.db.QueryRow(query, token).Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get team invitation by token: %w", err)
	}
	return invitation, nil
}

// GetByID retrieves an invitation by ID
func (r *TeamInvitationRepository) GetByID(id string) (*models.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE id = $1
	`
	invitation := &models.TeamInvitation{}
	err := r.db.QueryRow(query, id).Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get team invitation by id: %w", err)
	}
	return invitation, nil
}

// MarkAccepted marks an invitation as accepted
func (r *TeamInvitationRepository) MarkAccepted(invitationID, userID string) error {
	return r.MarkAcceptedTx(nil, invitationID, userID)
}

// MarkAcceptedTx marks an invitation as accepted within a transaction
func (r *TeamInvitationRepository) MarkAcceptedTx(tx *sql.Tx, invitationID, userID string) error {
	const query = `
		UPDATE team_invitations
		SET status = $1, accepted_at = NOW(), accepted_by_user_id = $2
		WHERE id = $3
	`
	var execer teamInvitationExecer = r.db
	if tx != nil {
		execer = tx
	}

	_, err := execer.Exec(query, "accepted", userID, invitationID)
	if err != nil {
		return fmt.Errorf("mark invitation accepted: %w", err)
	}
	return nil
}

// GetPendingByCompanyAndEmail retrieves pending invitations for a company and email
func (r *TeamInvitationRepository) GetPendingByCompanyAndEmail(companyID, email string) (*models.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE company_id = $1 AND email = $2 AND status = 'pending' AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`
	invitation := &models.TeamInvitation{}
	err := r.db.QueryRow(query, companyID, email).Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get pending invitation: %w", err)
	}
	return invitation, nil
}

// ListByCompany retrieves all invitations for a company
func (r *TeamInvitationRepository) ListByCompany(companyID string) ([]*models.TeamInvitation, error) {
	const query = `
		SELECT id, company_id, email, role, invited_by, status, token, expires_at, created_at, accepted_at, accepted_by_user_id
		FROM team_invitations
		WHERE company_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	var invitations []*models.TeamInvitation
	for rows.Next() {
		invitation := &models.TeamInvitation{}
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
func (r *TeamInvitationRepository) DeleteExpired() (int64, error) {
	const query = `DELETE FROM team_invitations WHERE status = 'pending' AND expires_at < NOW()`
	result, err := r.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("delete expired invitations: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
