package domain

import (
	"context"
	"time"
)

//go:generate mockgen -destination=../service/mocks/repository/user/mock.go -package=repomocks . UserRepository
type UserRepository interface {
	CreateTx(ctx context.Context, tx Execer, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	ListByCompanyID(ctx context.Context, companyID string) ([]*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
	FindByAuthUserID(ctx context.Context, authUserID string) (*User, error)
	UpdateAuthUserID(ctx context.Context, id, authUserID string) error
	UpdateEmailVerified(ctx context.Context, id string, verified bool) error
	UpdateLastLogin(ctx context.Context, id string) error
	UpdateProfile(ctx context.Context, id string, fullName, jobRole *string, onboardingCompleted bool) error
	MarkFirstInstrumentationDone(ctx context.Context, id string) error
	GetPasswordChangedAt(ctx context.Context, id string) (*time.Time, error)
	GetPasswordHash(ctx context.Context, email string) (string, error)
	ClearPasswordHash(ctx context.Context, userID string) error
	ArchiveUser(ctx context.Context, userID, archivedEmail string) error
	Delete(ctx context.Context, id string) error
	DeleteTx(ctx context.Context, tx Execer, id string) error
}

//go:generate mockgen -destination=../service/mocks/repository/company/mock.go -package=repomocks . CompanyRepository
type CompanyRepository interface {
	CreateTx(ctx context.Context, tx Execer, company *Company) error
	FindByID(ctx context.Context, id string) (*Company, error)
	UpdateDetails(ctx context.Context, id string, industry, size, useCase *string) error
	Delete(ctx context.Context, id string) error
	DeleteTx(ctx context.Context, tx Execer, id string) error
}

//go:generate mockgen -destination=../service/mocks/repository/apikey/mock.go -package=repomocks . APIKeyRepository
type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *APIKey) error
	FindByCompanyID(ctx context.Context, companyID string) ([]*APIKey, error)
	FindByID(ctx context.Context, id string) (*APIKey, error)
	FindByHash(ctx context.Context, keyHash string) (*APIKey, error)
	Revoke(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
}

//go:generate mockgen -destination=../service/mocks/repository/invitation/mock.go -package=repomocks . TeamInvitationRepository
type TeamInvitationRepository interface {
	Create(ctx context.Context, invitation *TeamInvitation) error
	CreateTx(ctx context.Context, tx Execer, invitation *TeamInvitation) error
	GetByToken(ctx context.Context, token string) (*TeamInvitation, error)
	GetByID(ctx context.Context, id string) (*TeamInvitation, error)
	MarkAccepted(ctx context.Context, invitationID, userID string) error
	MarkAcceptedTx(ctx context.Context, tx Execer, invitationID, userID string) error
	UpdateStatus(ctx context.Context, invitationID, status string) error
	Delete(ctx context.Context, invitationID string) error
	DeleteTx(ctx context.Context, tx Execer, invitationID string) error
	RefreshInvitation(ctx context.Context, invitationID, newToken string, expiresAt time.Time) error
	GetPendingByCompanyAndEmail(ctx context.Context, companyID, email string) (*TeamInvitation, error)
	ListByCompany(ctx context.Context, companyID string) ([]*TeamInvitation, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

//go:generate mockgen -destination=../service/mocks/repository/outbox/mock.go -package=repomocks . OutboxRepository
type OutboxRepository interface {
	Create(ctx context.Context, event *OutboxEvent) error
	CreateTx(ctx context.Context, tx Execer, event *OutboxEvent) error
	FetchPendingDue(ctx context.Context, limit int) ([]*OutboxEvent, error)
	ExistsByReference(ctx context.Context, eventType, referenceID string) (bool, error)
	ExistsPendingByReference(ctx context.Context, eventType, referenceID string) (bool, error)
	MarkDone(ctx context.Context, id string) error
	MarkFailedWithRetry(ctx context.Context, id, lastErr string, nextRunAt time.Time) error
	PruneProcessed(ctx context.Context, before time.Time) (int64, error)
}

//go:generate mockgen -destination=../service/mocks/repository/userrole/mock.go -package=repomocks . UserRoleRepository
type UserRoleRepository interface {
	AssignRoleToUser(ctx context.Context, userID, roleName, assignedBy string) error
	AssignRoleToUserTx(ctx context.Context, tx Execer, userID, roleName, assignedBy string) error
	GetUserRoles(ctx context.Context, userID string) ([]string, error)
	RemoveRoleFromUser(ctx context.Context, userID, roleName string) error
	AssignRoleToServiceAccount(ctx context.Context, serviceAccountID, roleName, assignedBy string) error
	GetServiceAccountRoles(ctx context.Context, serviceAccountID string) ([]string, error)
}

//go:generate mockgen -destination=../service/mocks/repository/serviceaccount/mock.go -package=repomocks . ServiceAccountRepository
type ServiceAccountRepository interface {
	Create(ctx context.Context, sa *ServiceAccount) error
	FindByID(ctx context.Context, id string) (*ServiceAccount, error)
	FindByCompanyID(ctx context.Context, companyID string) ([]*ServiceAccount, error)
	Update(ctx context.Context, sa *ServiceAccount) error
	Delete(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string) error
	IsServiceAccountActive(ctx context.Context, id string) (bool, error)
}

//go:generate mockgen -destination=../service/mocks/repository/agent/mock.go -package=repomocks . AgentRepository
type AgentRepository interface {
	CreateConversation(ctx context.Context, conv *AgentConversation) error
	CreateConversationWithParent(ctx context.Context, conv *AgentConversation, parentConvID string) error
	GetConversations(ctx context.Context, companyID string) ([]AgentConversation, error)
	UpdateConversationStats(ctx context.Context, convID string, messageCount, tokenCount int) error
	GetConversationStats(ctx context.Context, convID string) (messageCount, tokenCount int, err error)
	AddMessage(ctx context.Context, msg *AgentMessage) error
	GetMessages(ctx context.Context, convID string) ([]*AgentMessage, error)
	DeleteConversation(ctx context.Context, convID string, userID string) error
	SaveContext(ctx context.Context, agentCtx *AgentContext) error
	GetContext(ctx context.Context, convID string) ([]*AgentContext, error)
}
