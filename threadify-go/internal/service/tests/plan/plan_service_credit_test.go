package service_test

import (
	"context"
	"testing"

	"threadify-go/shared/billing"
	shareddomain "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/service/tests/common"
)

func TestCheckBalancePositive_AllowsZeroBalanceWithAutoTopup(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-1"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        100,
		CreditMaxMonthlyChargeMillicents: 500,
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "0",
		keys.Charged: "0",
		keys.Pending: "0",
	}, nil)

	svc := deps.NewPlanService(&config.SubscriptionConfig{})

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCheckBalancePositive_FailsWhenTopupDisabledAndBalanceZero(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-2"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "0",
		keys.Charged: "0",
	}, nil)

	svc := deps.NewPlanService(&config.SubscriptionConfig{})

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.ErrorIs(t, err, service.ErrInsufficientCredit)
	require.Nil(t, got)
}

func TestCheckBalancePositive_UnpopulatedCacheFailsWhenTopupDisabled(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-3"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{}, nil)

	svc := deps.NewPlanService(&config.SubscriptionConfig{})

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.ErrorIs(t, err, service.ErrInsufficientCredit)
	require.Nil(t, got)
}

func TestCheckBalancePositive_AllowsPendingTopup(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-4"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        100,
		CreditMaxMonthlyChargeMillicents: 500,
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "0",
		keys.Charged: "0",
		keys.Pending: "50",
	}, nil)

	svc := deps.NewPlanService(&config.SubscriptionConfig{})

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCheckBalancePositive_FailsWhenLowBalanceAndTopupDisabled(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-low-1"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}
	subCfg := &config.SubscriptionConfig{
		Credit: config.CreditConfig{
			ContractCostMillicents: 1000,
		},
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "500",
		keys.Charged: "0",
	}, nil)

	svc := deps.NewPlanService(subCfg)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.ErrorIs(t, err, service.ErrInsufficientCredit)
	require.Nil(t, got)
}

func TestCheckBalancePositive_SucceedsWhenLowBalanceAndTopupEnabled(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-low-2"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        5000,
		CreditMaxMonthlyChargeMillicents: 10000,
	}
	subCfg := &config.SubscriptionConfig{
		Credit: config.CreditConfig{
			ContractCostMillicents: 1000,
		},
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "500",
		keys.Charged: "0",
	}, nil)

	svc := deps.NewPlanService(subCfg)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCheckCreditAvailable_InsufficientBalanceWithTopupDisabled(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-avail-1"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}
	subCfg := &config.SubscriptionConfig{
		Credit: config.CreditConfig{
			ContractCostMillicents: 10000,
		},
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "5000",
		keys.Charged: "0",
	}, nil)

	svc := deps.NewPlanService(subCfg)

	err := svc.CheckCreditAvailable(context.Background(), companyID, service.MeterContractExecution, 1)
	require.ErrorIs(t, err, service.ErrInsufficientCredit)
}

func TestCheckCreditAvailable_SucceedsAfterTopupEnabled(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-avail-2"
	keys := billing.KeysFor(companyID)
	account := &shareddomain.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        10000,
		CreditMaxMonthlyChargeMillicents: 50000,
	}
	subCfg := &config.SubscriptionConfig{
		Credit: config.CreditConfig{
			ContractCostMillicents: 10000,
		},
	}

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	deps.Valkey.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).Return(map[string]string{
		keys.Balance: "5000",
		keys.Charged: "0",
	}, nil)

	svc := deps.NewPlanService(subCfg)

	err := svc.CheckCreditAvailable(context.Background(), companyID, service.MeterContractExecution, 1)
	require.NoError(t, err)
}
