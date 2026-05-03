package common

import (
	"testing"

	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/service"
	agentrepomocks "threadify-go/api/internal/service/mocks/repository/agent"
	apikeyrepomocks "threadify-go/api/internal/service/mocks/repository/apikey"
	companyrepomocks "threadify-go/api/internal/service/mocks/repository/company"
	invitationrepomocks "threadify-go/api/internal/service/mocks/repository/invitation"
	outboxrepomocks "threadify-go/api/internal/service/mocks/repository/outbox"
	sarepomocks "threadify-go/api/internal/service/mocks/repository/serviceaccount"
	userrepomocks "threadify-go/api/internal/service/mocks/repository/user"
	userrolerepomocks "threadify-go/api/internal/service/mocks/repository/userrole"
	agentmocks "threadify-go/api/internal/service/mocks/service/agent"
	apikeymocks "threadify-go/api/internal/service/mocks/service/api_key"
	authmocks "threadify-go/api/internal/service/mocks/service/auth"
	billingmocks "threadify-go/api/internal/service/mocks/service/billing"
	emailmocks "threadify-go/api/internal/service/mocks/service/email"
	entityprofilemocks "threadify-go/api/internal/service/mocks/service/entity_profile_type"
	serviceaccountmocks "threadify-go/api/internal/service/mocks/service/service_account"
	invitationmocks "threadify-go/api/internal/service/mocks/service/team_invitation"
	usermocks "threadify-go/api/internal/service/mocks/service/user"
	sharedauth "threadify-go/shared/auth"
	sharedmocks "threadify-go/shared/mocks"
	"threadify-go/shared/rbac"

	"github.com/golang/mock/gomock"
	"go.uber.org/zap"
)

type MockedDeps struct {
	Ctrl *gomock.Controller

	UserRepo              *userrepomocks.MockUserRepository
	CompanyRepo           *companyrepomocks.MockCompanyRepository
	APIKeyRepoInternal    *apikeyrepomocks.MockAPIKeyRepository
	InvitationRepo        *invitationrepomocks.MockTeamInvitationRepository
	OutboxRepo            *outboxrepomocks.MockOutboxRepository
	UserRoleRepo          *userrolerepomocks.MockUserRoleRepository
	ServiceAccountRepo    *sarepomocks.MockServiceAccountRepository
	AgentRepo             *agentrepomocks.MockAgentRepository
	EntityProfileTypeRepo *sharedmocks.MockEntityProfileTypeRepository
	PlanRepo              *sharedmocks.MockPlanRepository

	AuthSvc           *authmocks.MockAuthService
	APIKeySvc         *apikeymocks.MockAPIKeyService
	InvitationSvc     *invitationmocks.MockTeamInvitationService
	AgentSvc          *agentmocks.MockAgentService
	EmailSvc          *emailmocks.MockEmailService
	ServiceAccountSvc *serviceaccountmocks.MockServiceAccountService
	EntityProfileSvc  *entityprofilemocks.MockEntityProfileTypeService
	UserSvc           *usermocks.MockUserService
	BillingSvc        *billingmocks.MockBillingService

	Logger *zap.Logger
}

func NewMockDeps(t *testing.T) *MockedDeps {
	ctrl := gomock.NewController(t)
	return &MockedDeps{
		Ctrl: ctrl,

		UserRepo:              userrepomocks.NewMockUserRepository(ctrl),
		CompanyRepo:           companyrepomocks.NewMockCompanyRepository(ctrl),
		APIKeyRepoInternal:    apikeyrepomocks.NewMockAPIKeyRepository(ctrl),
		InvitationRepo:        invitationrepomocks.NewMockTeamInvitationRepository(ctrl),
		OutboxRepo:            outboxrepomocks.NewMockOutboxRepository(ctrl),
		UserRoleRepo:          userrolerepomocks.NewMockUserRoleRepository(ctrl),
		ServiceAccountRepo:    sarepomocks.NewMockServiceAccountRepository(ctrl),
		AgentRepo:             agentrepomocks.NewMockAgentRepository(ctrl),
		EntityProfileTypeRepo: sharedmocks.NewMockEntityProfileTypeRepository(ctrl),
		PlanRepo:              sharedmocks.NewMockPlanRepository(ctrl),

		AuthSvc:           authmocks.NewMockAuthService(ctrl),
		APIKeySvc:         apikeymocks.NewMockAPIKeyService(ctrl),
		InvitationSvc:     invitationmocks.NewMockTeamInvitationService(ctrl),
		AgentSvc:          agentmocks.NewMockAgentService(ctrl),
		EmailSvc:          emailmocks.NewMockEmailService(ctrl),
		ServiceAccountSvc: serviceaccountmocks.NewMockServiceAccountService(ctrl),
		EntityProfileSvc:  entityprofilemocks.NewMockEntityProfileTypeService(ctrl),
		BillingSvc:        billingmocks.NewMockBillingService(ctrl),

		Logger: zap.NewNop(),
	}
}

func (d *MockedDeps) NewAuthService(
	pool ports.DBPool,
	authClient sharedauth.AuthClient,
	jwksVerifier sharedauth.TokenVerifier,
	outboxWorker service.OutboxWorkerTrigger,
	encryptionKey []byte,
) *service.AuthService {
	return service.NewAuthService(
		ports.WrapAsTxManager(pool),
		d.UserRepo,
		d.CompanyRepo,
		d.UserRoleRepo,
		d.EmailSvc,
		authClient,
		jwksVerifier,
		d.BillingSvc,
		d.OutboxRepo,
		d.InvitationRepo,
		outboxWorker,
		encryptionKey,
		d.Logger,
	)
}

func (d *MockedDeps) NewAPIKeyService(rbacLoader ports.RBACRoleLoader) *service.APIKeyService {
	return service.NewAPIKeyService(
		d.APIKeyRepoInternal,
		d.ServiceAccountRepo,
		d.UserRoleRepo,
		rbacLoader,
		d.Logger,
	)
}

func (d *MockedDeps) NewTeamInvitationService(
	outboxWorker service.OutboxWorkerTrigger,
	encryptionKey []byte,
	frontendURL string,
) *service.TeamInvitationService {
	return service.NewTeamInvitationService(
		d.InvitationRepo,
		d.OutboxRepo,
		outboxWorker,
		d.UserRepo,
		d.CompanyRepo,
		encryptionKey,
		frontendURL,
		d.Logger,
	)
}

func (d *MockedDeps) NewServiceAccountService() *service.ServiceAccountService {
	return service.NewServiceAccountService(
		d.Logger,
		d.ServiceAccountRepo,
		d.UserRoleRepo,
	)
}

func (d *MockedDeps) NewEntityProfileTypeService() *service.EntityProfileTypeService {
	return service.NewEntityProfileTypeService(
		d.EntityProfileTypeRepo,
		d.Logger,
	)
}

func (d *MockedDeps) NewAgentService(engineURL, openaiKey string, maxMsg, maxTok, sumTok int) *service.AgentService {
	return service.NewAgentService(
		engineURL,
		openaiKey,
		d.AgentRepo,
		maxMsg,
		maxTok,
		sumTok,
		d.Logger,
	)
}

type RBACRoleLoaderWrapper struct {
	Loader *rbac.Loader
}

func (w *RBACRoleLoaderWrapper) GetRolesByLevel(level string) map[string]struct{} {
	if w.Loader == nil {
		return nil
	}
	roles := w.Loader.GetRolesByLevel(level)
	result := make(map[string]struct{}, len(roles))
	for roleName := range roles {
		result[roleName] = struct{}{}
	}
	return result
}
