package interfaces

import (
	"context"
	"time"

	"threadify-go/api/internal/models"
	apiservice "threadify-go/api/internal/service"
	sharedauth "threadify-go/shared/auth"
	sharedmodels "threadify-go/shared/models"
)

//go:generate mockgen -package=svcmocks -destination=../service/mocks/service/service_mocks.go -source=service.go

type AuthService interface {
	Signup(ctx context.Context, req *models.SignupRequest) error
	Login(ctx context.Context, req *models.LoginRequest, clientIP string) (*models.AuthResponse, error)
	ForgotPassword(ctx context.Context, req *models.ForgotPasswordRequest) error
	ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) error
	VerifyEmail(ctx context.Context, req *models.VerifyEmailRequest) (*models.AuthResponse, error)
	Logout(ctx context.Context, token string) error
	VerifyToken(ctx context.Context, tokenString string) (*sharedauth.TokenClaims, error)
	GetUserRoles(ctx context.Context, userID, principalType string) ([]string, error)
	ResendVerificationEmail(ctx context.Context, req *models.ResendVerificationEmailRequest) error
}

type APIKeyService interface {
	CreateAPIKey(ctx context.Context, userID, companyID string, req *apiservice.CreateAPIKeyRequest) (*apiservice.CreateAPIKeyResponse, error)
	ListAPIKeys(ctx context.Context, companyID string) ([]*models.APIKey, error)
	RevokeAPIKey(ctx context.Context, keyID, companyID string) error
	ValidateAPIKey(ctx context.Context, key string) (*models.APIKey, error)
}

type TeamInvitationService interface {
	SendInvitation(ctx context.Context, companyID, email, role, invitedBy string, expiryDuration time.Duration) (*models.TeamInvitation, error)
	ValidateToken(ctx context.Context, token string) (*models.TeamInvitation, error)
	MarkAccepted(ctx context.Context, invitationID, userID string) error
	GetByCompanyAndEmail(ctx context.Context, companyID, email string) (*models.TeamInvitation, error)
	GetByID(ctx context.Context, invitationID string) (*models.TeamInvitation, error)
	ListByCompany(ctx context.Context, companyID string) ([]*models.TeamInvitation, error)
	CancelInvitation(ctx context.Context, invitationID string) error
	RefreshInvitation(ctx context.Context, invitation *models.TeamInvitation, duration time.Duration) (*models.TeamInvitation, error)
}

type AgentService interface {
	ChatStream(ctx context.Context, authHeader, userID, companyID, conversationID, message, skill string, onEvent apiservice.StreamHandler) error
	CheckCredits(ctx context.Context, authHeader string) (bool, error)
	GetConversations(ctx context.Context, companyID string) ([]models.AgentConversation, error)
	GetMessagesForUser(ctx context.Context, companyID, convID string) ([]*models.AgentMessage, error)
	DeleteConversation(ctx context.Context, companyID, convID string) error
	ContinueConversation(ctx context.Context, userID, companyID, parentConvID string) (string, string, string, error)
}

type ServiceAccountService interface {
	CreateServiceAccount(ctx context.Context, companyID, createdBy string, req *apiservice.CreateServiceAccountRequest) (*models.ServiceAccount, error)
	ListServiceAccounts(ctx context.Context, companyID string) ([]*models.ServiceAccount, error)
	GetServiceAccount(ctx context.Context, id, companyID string) (*models.ServiceAccount, error)
	UpdateServiceAccount(ctx context.Context, id, companyID string, req *apiservice.UpdateServiceAccountRequest) (*models.ServiceAccount, error)
	DeleteServiceAccount(ctx context.Context, id, companyID string) error
}

type EntityProfileTypeService interface {
	CreateEntityProfileType(ctx context.Context, companyID string, req *models.CreateEntityProfileTypeRequest) (*sharedmodels.EntityProfileType, error)
	ListEntityProfileTypes(ctx context.Context, companyID string) ([]*sharedmodels.EntityProfileType, error)
	UpdateEntityProfileType(ctx context.Context, companyID, id string, req *models.UpdateEntityProfileTypeRequest) (*sharedmodels.EntityProfileType, error)
	ArchiveEntityProfileType(ctx context.Context, companyID, id string) error
}
