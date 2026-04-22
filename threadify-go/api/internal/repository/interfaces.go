package repository

import (
	"context"
	"time"

	"threadify-go/api/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Common interfaces for transaction support
type DBExecer interface {
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/user/mock.go -source=interfaces.go UserRepository
type UserRepository interface {
	Pool() *pgxpool.Pool
	CreateTx(ctx context.Context, execer DBExecer, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	ListByCompanyID(ctx context.Context, companyID string) ([]*models.User, error)
	FindByID(ctx context.Context, id string) (*models.User, error)
	FindByAuthUserID(ctx context.Context, authUserID string) (*models.User, error)
	UpdateAuthUserID(ctx context.Context, id, authUserID string) error
	UpdateEmailVerified(ctx context.Context, id string, verified bool) error
	UpdateLastLogin(ctx context.Context, id string) error
	UpdateProfile(ctx context.Context, id string, fullName, jobRole *string, onboardingCompleted bool) error
	MarkFirstInstrumentationDone(ctx context.Context, id string) error
	GetPasswordChangedAt(ctx context.Context, id string) (*time.Time, error)
	GetPasswordHash(ctx context.Context, email string) (string, error)
	ClearPasswordHash(ctx context.Context, userID string) error
	Delete(ctx context.Context, id string) error
	DeleteTx(ctx context.Context, execer DBExecer, id string) error
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/company/mock.go -source=interfaces.go CompanyRepository
type CompanyRepository interface {
	CreateTx(ctx context.Context, execer DBExecer, company *models.Company) error
	FindByID(ctx context.Context, id string) (*models.Company, error)
	UpdateDetails(ctx context.Context, id string, industry, size, useCase *string) error
	Delete(ctx context.Context, id string) error
	DeleteTx(ctx context.Context, execer DBExecer, id string) error
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/apikey/mock.go -source=interfaces.go APIKeyRepository
type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *models.APIKey) error
	FindByCompanyID(ctx context.Context, companyID string) ([]*models.APIKey, error)
	FindByID(ctx context.Context, id string) (*models.APIKey, error)
	FindByHash(ctx context.Context, keyHash string) (*models.APIKey, error)
	Revoke(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/invitation/mock.go -source=interfaces.go TeamInvitationRepository
type TeamInvitationRepository interface {
	Create(ctx context.Context, invitation *models.TeamInvitation) error
	CreateTx(ctx context.Context, execer DBExecer, invitation *models.TeamInvitation) error
	GetByToken(ctx context.Context, token string) (*models.TeamInvitation, error)
	GetByID(ctx context.Context, id string) (*models.TeamInvitation, error)
	MarkAccepted(ctx context.Context, invitationID, userID string) error
	MarkAcceptedTx(ctx context.Context, execer DBExecer, invitationID, userID string) error
	UpdateStatus(ctx context.Context, invitationID, status string) error
	Delete(ctx context.Context, invitationID string) error
	DeleteTx(ctx context.Context, execer DBExecer, invitationID string) error
	RefreshInvitation(ctx context.Context, invitationID, newToken string, expiresAt time.Time) error
	GetPendingByCompanyAndEmail(ctx context.Context, companyID, email string) (*models.TeamInvitation, error)
	ListByCompany(ctx context.Context, companyID string) ([]*models.TeamInvitation, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/outbox/mock.go -source=interfaces.go OutboxRepository
type OutboxRepository interface {
	Create(ctx context.Context, event *models.OutboxEvent) error
	CreateTx(ctx context.Context, execer DBExecer, event *models.OutboxEvent) error
	FetchPendingDue(ctx context.Context, limit int) ([]*models.OutboxEvent, error)
	ExistsByReference(ctx context.Context, eventType, referenceID string) (bool, error)
	ExistsPendingByReference(ctx context.Context, eventType, referenceID string) (bool, error)
	MarkDone(ctx context.Context, id string) error
	MarkFailedWithRetry(ctx context.Context, id, lastErr string, nextRunAt time.Time) error
	PruneProcessed(ctx context.Context, before time.Time) (int64, error)
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/userrole/mock.go -source=interfaces.go UserRoleRepository
type UserRoleRepository interface {
	AssignRoleToUser(ctx context.Context, userID, roleName, assignedBy string) error
	AssignRoleToUserTx(ctx context.Context, execer DBExecer, userID, roleName, assignedBy string) error
	GetUserRoles(ctx context.Context, userID string) ([]string, error)
	RemoveRoleFromUser(ctx context.Context, userID, roleName string) error
	AssignRoleToServiceAccount(ctx context.Context, serviceAccountID, roleName, assignedBy string) error
	GetServiceAccountRoles(ctx context.Context, serviceAccountID string) ([]string, error)
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/serviceaccount/mock.go -source=interfaces.go ServiceAccountRepository
type ServiceAccountRepository interface {
	Create(ctx context.Context, sa *models.ServiceAccount) error
	FindByID(ctx context.Context, id string) (*models.ServiceAccount, error)
	FindByCompanyID(ctx context.Context, companyID string) ([]*models.ServiceAccount, error)
	Update(ctx context.Context, sa *models.ServiceAccount) error
	Delete(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string) error
	IsServiceAccountActive(ctx context.Context, id string) (bool, error)
}

//go:generate mockgen -package=repomocks -destination=../service/mocks/repository/agent/mock.go -source=interfaces.go AgentRepository
type AgentRepository interface {
	CreateConversation(ctx context.Context, conv *models.AgentConversation) error
	CreateConversationWithParent(ctx context.Context, conv *models.AgentConversation, parentConvID string) error
	GetConversations(ctx context.Context, companyID string) ([]models.AgentConversation, error)
	UpdateConversationStats(ctx context.Context, convID string, messageCount, tokenCount int) error
	GetConversationStats(ctx context.Context, convID string) (messageCount, tokenCount int, err error)
	AddMessage(ctx context.Context, msg *models.AgentMessage) error
	GetMessages(ctx context.Context, convID string) ([]*models.AgentMessage, error)
	DeleteConversation(ctx context.Context, convID string, userID string) error
	SaveContext(ctx context.Context, agentCtx *models.AgentContext) error
	GetContext(ctx context.Context, convID string) ([]*models.AgentContext, error)
}
