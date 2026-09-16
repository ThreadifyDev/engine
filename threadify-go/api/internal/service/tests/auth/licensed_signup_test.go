package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	authmocks "threadify-go/api/internal/service/mocks/service/auth"
	"threadify-go/api/internal/service/tests/common"
	"threadify-go/shared/management/domain"
	sharedmocks "threadify-go/shared/mocks"
)

// Licensed signup attaches the verified owner to the existing Registry company
// without creating another company or granting local credits.
func TestLicensedOwnerSignupUsesProvisionedCompany(t *testing.T) {
	ctrl := gomock.NewController(t)
	deps := common.NewMockDeps(t)
	pool := authmocks.NewMockDBPool(ctrl)
	tx := authmocks.NewMockTx(ctrl)
	client := sharedmocks.NewMockAuthClient(ctrl)
	company := &domain.Company{ID: "licensed-company", Name: "Registry company"}
	deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), "owner@example.com").Return(nil, nil)
	deps.CompanyRepo.EXPECT().FindByID(gomock.Any(), company.ID).Return(company, nil)
	pool.EXPECT().Begin(gomock.Any()).Return(tx, nil)
	tx.EXPECT().Rollback(gomock.Any()).AnyTimes()
	deps.UserRepo.EXPECT().CreateTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ any, user *domain.User) error {
		require.Equal(t, company.ID, user.CompanyID)
		return nil
	})
	deps.UserRoleRepo.EXPECT().AssignRoleToUserTx(gomock.Any(), tx, gomock.Any(), "admin", "system").Return(nil)
	deps.OutboxRepo.EXPECT().CreateTx(gomock.Any(), tx, gomock.Any()).Return(nil)
	tx.EXPECT().Commit(gomock.Any()).Return(nil)
	svc := deps.NewAuthService(pool, client, nil, nil, []byte(testEncryptionKey))
	svc.ConfigureLicensedAccount(company.ID, "owner@example.com")
	require.NoError(t, svc.Signup(context.Background(), &domain.SignupCmd{Email: "owner@example.com", Password: "Password123!", CompanyName: "Ignored"}))
}

// An arbitrary mailbox cannot bootstrap an administrator on the licensed company.
func TestLicensedSignupRequiresOwnerOrInvitation(t *testing.T) {
	deps := common.NewMockDeps(t)
	svc := deps.NewAuthService(nil, nil, nil, nil, []byte(testEncryptionKey))
	svc.ConfigureLicensedAccount("licensed-company", "owner@example.com")
	require.ErrorContains(t, svc.Signup(context.Background(), &domain.SignupCmd{Email: "stranger@example.com"}), "team invitation")
}

// Every negative branch uses strict mocks with no write expectations. Even an
// otherwise valid invitation cannot redirect signup into another licensed company.
func TestLicensedInvitationFailuresDoNotProvisionUsers(t *testing.T) {
	for _, tc := range []struct {
		name, company, status           string
		expired, missing, lookupFailure bool
		want                            string
	}{
		{name: "other company", company: "different-company", status: "pending", want: "licensed account"},
		{name: "expired", company: "licensed-company", status: "pending", expired: true, want: "expired"},
		{name: "already accepted", company: "licensed-company", status: "accepted", want: "already used"},
		{name: "revoked", company: "licensed-company", status: "revoked", want: "already used"},
		{name: "missing", missing: true, want: "not found"},
		{name: "lookup failure", lookupFailure: true, want: "invalid invitation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewAuthService(nil, nil, nil, nil, []byte(testEncryptionKey))
			svc.ConfigureLicensedAccount("licensed-company", "owner@example.com")
			expiry := time.Now().Add(time.Hour)
			if tc.expired {
				expiry = time.Now().Add(-time.Hour)
			}
			var invitation *domain.TeamInvitation
			if !tc.missing && !tc.lookupFailure {
				invitation = &domain.TeamInvitation{ID: "invitation", CompanyID: tc.company, Email: "invited@example.com", Role: "admin", Status: tc.status, ExpiresAt: expiry}
			}
			var lookupErr error
			if tc.lookupFailure {
				lookupErr = errors.New("database unavailable")
			}
			deps.InvitationRepo.EXPECT().GetByToken(gomock.Any(), "invitation-token").Return(invitation, lookupErr)
			token := "invitation-token"
			request := &domain.SignupCmd{Email: "submitted@example.com", InvitationToken: &token}
			require.ErrorContains(t, svc.Signup(context.Background(), request), tc.want)
			require.Equal(t, "submitted@example.com", request.Email, "denial must precede invitation email rebinding")
		})
	}
}

func TestLicensedOwnerCannotReassignExistingForeignCompanyUser(t *testing.T) {
	deps := common.NewMockDeps(t)
	svc := deps.NewAuthService(nil, nil, nil, nil, []byte(testEncryptionKey))
	svc.ConfigureLicensedAccount("licensed-company", "owner@example.com")
	deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), "owner@example.com").Return(&domain.User{ID: "existing", Email: "owner@example.com", CompanyID: "other-company"}, nil)
	require.ErrorContains(t, svc.Signup(context.Background(), &domain.SignupCmd{Email: "owner@example.com"}), "already exists")
}

func TestLicensedSignupWithoutRegistryOwnerCannotBootstrap(t *testing.T) {
	deps := common.NewMockDeps(t)
	svc := deps.NewAuthService(nil, nil, nil, nil, []byte(testEncryptionKey))
	svc.ConfigureLicensedAccount("licensed-company", "")
	require.ErrorContains(t, svc.Signup(context.Background(), &domain.SignupCmd{Email: "owner@example.com"}), "team invitation")
}

// A company can disappear after the invitation is read. Its missing row must
// produce a denial before user provisioning, not a nil dereference.
func TestLicensedInvitationMissingCompanyIsRejectedWithoutMutation(t *testing.T) {
	deps := common.NewMockDeps(t)
	svc := deps.NewAuthService(nil, nil, nil, nil, []byte(testEncryptionKey))
	svc.ConfigureLicensedAccount("licensed-company", "owner@example.com")
	deps.InvitationRepo.EXPECT().GetByToken(gomock.Any(), "token").Return(&domain.TeamInvitation{CompanyID: "licensed-company", Email: "invited@example.com", Role: "member", Status: "pending", ExpiresAt: time.Now().Add(time.Hour)}, nil)
	deps.CompanyRepo.EXPECT().FindByID(gomock.Any(), "licensed-company").Return(nil, nil)
	token := "token"
	require.ErrorContains(t, svc.Signup(context.Background(), &domain.SignupCmd{Email: "submitted@example.com", InvitationToken: &token}), "company not found")
}
