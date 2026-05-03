package ports

import (
	"context"
	"time"

	"threadify-go/api/internal/domain"
	sharedauth "threadify-go/shared/auth"
	shareddomain "threadify-go/shared/domain"
)

//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/api_key/api_key_mocks.go threadify-go/api/internal/ports APIKeyService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/auth/auth_mocks.go threadify-go/api/internal/ports AuthService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/agent/agent_mocks.go threadify-go/api/internal/ports AgentService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/service_account/service_account_mocks.go threadify-go/api/internal/ports ServiceAccountService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/team_invitation/team_invitation_mocks.go threadify-go/api/internal/ports TeamInvitationService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/entity_profile_type/entity_profile_type_mocks.go threadify-go/api/internal/ports EntityProfileTypeService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/user/user_mocks.go threadify-go/api/internal/ports UserService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/billing/billing_mocks.go threadify-go/api/internal/ports BillingService
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/permission/permission_mocks.go threadify-go/api/internal/ports PermissionLoader
//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/rbac/rbac_mocks.go threadify-go/api/internal/ports RBACRoleLoader

type AuthService interface {
	Signup(ctx context.Context, req *domain.SignupCmd) error
	Login(ctx context.Context, req *domain.LoginCmd, clientIP string) (*domain.AuthSession, error)
	ForgotPassword(ctx context.Context, req *domain.ForgotPasswordCmd) error
	ResetPassword(ctx context.Context, req *domain.ResetPasswordCmd) error
	VerifyEmail(ctx context.Context, req *domain.VerifyEmailCmd) (*domain.AuthSession, error)
	Logout(ctx context.Context, token string) error
	VerifyToken(ctx context.Context, tokenString string) (*sharedauth.TokenClaims, error)
	GetUserRoles(ctx context.Context, userID, principalType string) ([]string, error)
	ResendVerificationEmail(ctx context.Context, req *domain.ResendVerificationEmailCmd) error
}

type APIKeyService interface {
	CreateAPIKey(ctx context.Context, userID, companyID string, req *domain.CreateAPIKeyCmd) (*domain.APIKeyCredentials, error)
	ListAPIKeys(ctx context.Context, companyID string) ([]*domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, keyID, companyID string) error
	ValidateAPIKey(ctx context.Context, key string) (*domain.APIKey, error)
}

type TeamInvitationService interface {
	SendInvitation(ctx context.Context, companyID, email, role, invitedBy string, expiryDuration time.Duration) (*domain.TeamInvitation, error)
	ValidateToken(ctx context.Context, token string) (*domain.InvitationTokenInfo, error)
	MarkAccepted(ctx context.Context, invitationID, userID string) error
	GetByCompanyAndEmail(ctx context.Context, companyID, email string) (*domain.TeamInvitation, error)
	GetByID(ctx context.Context, invitationID string) (*domain.TeamInvitation, error)
	ListByCompany(ctx context.Context, companyID string) ([]*domain.TeamInvitation, error)
	CancelInvitation(ctx context.Context, invitationID string) error
	RefreshInvitation(ctx context.Context, invitation *domain.TeamInvitation, duration time.Duration) (*domain.TeamInvitation, error)
}

type AgentService interface {
	ChatStream(ctx context.Context, authHeader, userID, companyID, conversationID, message, skill string, onEvent domain.StreamHandler) error
	ChatStreamEino(ctx context.Context, authHeader, userID, companyID, conversationID, message, skill string, onEvent domain.StreamHandler) error
	CheckCredits(ctx context.Context, authHeader string) (bool, error)
	GetConversations(ctx context.Context, companyID string) ([]domain.AgentConversation, error)
	GetMessagesForUser(ctx context.Context, companyID, convID string) ([]*domain.AgentMessage, error)
	DeleteConversation(ctx context.Context, companyID, convID string) error
	ContinueConversation(ctx context.Context, userID, companyID, parentConvID string) (string, string, string, error)
	GetMaxMessages() int
	GetMaxTokens() int
}

type ServiceAccountService interface {
	CreateServiceAccount(ctx context.Context, companyID, createdBy string, req *domain.CreateServiceAccountCmd) (*domain.ServiceAccount, error)
	ListServiceAccounts(ctx context.Context, companyID string) ([]*domain.ServiceAccount, error)
	GetServiceAccount(ctx context.Context, id, companyID string) (*domain.ServiceAccount, error)
	UpdateServiceAccount(ctx context.Context, id, companyID string, req *domain.UpdateServiceAccountCmd) (*domain.ServiceAccount, error)
	DeleteServiceAccount(ctx context.Context, id, companyID string) error
}

type EntityProfileTypeService interface {
	CreateEntityProfileType(ctx context.Context, companyID string, req *domain.CreateEntityProfileTypeCmd) (*domain.EntityProfileType, error)
	ListEntityProfileTypes(ctx context.Context, companyID string) ([]*domain.EntityProfileType, error)
	UpdateEntityProfileType(ctx context.Context, companyID, id string, req *domain.UpdateEntityProfileTypeCmd) (*domain.EntityProfileType, error)
	ArchiveEntityProfileType(ctx context.Context, companyID string, id string) error
	ListMetricsTemplates(ctx context.Context) ([]*domain.MetricsTemplate, error)
}

type UserService interface {
	GetProfile(ctx context.Context, userID, companyID string) (*domain.UserProfile, error)
	UpdateProfile(ctx context.Context, userID, companyID string, req *domain.UpdateProfileCmd) (*domain.User, error)
	MarkInstrumentationDone(ctx context.Context, userID string) (*domain.User, error)
	ListTeamMembers(ctx context.Context, companyID string) ([]*domain.TeamMember, error)
	RemoveTeamMember(ctx context.Context, requesterID, companyID, targetUserID string) error
}

type BillingService interface {
	GetCreditAccount(ctx context.Context, companyID string) (*shareddomain.CreditAccount, error)
	CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents int64) (string, error)
	UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error
	ProvisionSignupCredits(ctx context.Context, companyID string) error
}

type PermissionLoader interface {
	GetPermissionsForRoles(roleNames []string, scopeLevel string) []string
	CheckPermission(userPermissions []string, required string) bool
}

type RBACRoleLoader interface {
	GetRolesByLevel(level string) map[string]struct{}
}
