package service_test

import (
	"context"
	"testing"

	billingmodels "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"github.com/threadify/engine/internal/service/tests/common"
)

func TestProvisionSubscription_CreatesNewAccount(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	ctx := context.Background()
	companyID := "comp_123"
	externalID := "cust_999"
	initialAmount := int64(5000)
	const expectedMinBalanceFraction = 5

	deps.PlanRepo.EXPECT().GetCreditAccount(ctx, companyID).Return(nil, nil)
	deps.PlanRepo.EXPECT().SetExternalCustomerID(ctx, companyID, externalID).Return(nil)
	deps.PlanRepo.EXPECT().
		CreateCreditAccount(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, acc *billingmodels.CreditAccount) error {
			assert.Equal(t, companyID, acc.CompanyID)
			assert.Equal(t, initialAmount, acc.CreditBalanceMillicents)
			assert.Equal(t, initialAmount/expectedMinBalanceFraction, acc.CreditMinBalanceMillicents)
			return nil
		})

	mockPipe := enginemocks.NewMockValkeyPipeline(deps.Ctrl)
	deps.Valkey.EXPECT().Pipeline().Return(mockPipe)
	mockPipe.EXPECT().Set(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(mockPipe).AnyTimes()
	mockPipe.EXPECT().Del(ctx, gomock.Any()).Return(mockPipe).AnyTimes()
	mockPipe.EXPECT().Exec(ctx).Return(nil, nil)

	svc := deps.NewPlanService(&config.SubscriptionConfig{
		Credit: config.CreditConfig{
			RateLimitTPS:      100,
			PayloadLimitBytes: 1024 * 1024,
		},
	})

	err := svc.ProvisionSubscription(ctx, companyID, externalID, initialAmount)
	require.NoError(t, err)
}
