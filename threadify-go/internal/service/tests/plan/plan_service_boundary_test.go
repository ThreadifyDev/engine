package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"threadify-go/shared/billing"
	"threadify-go/shared/database"
	billingmodels "threadify-go/shared/models"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/service/tests/common"
	"go.uber.org/zap"
)

func TestEvaluateCreditAvailability_BoundaryMatrix(t *testing.T) {
	svc := service.NewPlanService(nil, nil, nil, &config.SubscriptionConfig{}, nil, nil, zap.NewNop(), 0)

	topupDisabled := func() *billingmodels.CreditAccount {
		return &billingmodels.CreditAccount{
			CompanyID:                        "c-1",
			CreditAutoTopupMillicents:        0,
			CreditMaxMonthlyChargeMillicents: 0,
		}
	}

	tests := []struct {
		name    string
		account *billingmodels.CreditAccount
		balance int64
		charged int64
		pending int64
		cost    int64
		wantErr bool
	}{
		{
			name:    "balance covers cost without topup",
			account: topupDisabled(),
			balance: 500,
			cost:    500,
			wantErr: false,
		},
		{
			name:    "disabled topup and deficit fails",
			account: topupDisabled(),
			balance: 100,
			cost:    200,
			wantErr: true,
		},
		{
			name: "pending cushion covers deficit",
			account: &billingmodels.CreditAccount{
				CompanyID:                        "c-1",
				CreditAutoTopupMillicents:        0,
				CreditMaxMonthlyChargeMillicents: 2000,
			},
			balance: 100,
			pending: 200,
			cost:    250,
			wantErr: false,
		},
		{
			name: "auto topup covers deficit under cap",
			account: &billingmodels.CreditAccount{
				CompanyID:                        "c-1",
				CreditAutoTopupMillicents:        1000,
				CreditMaxMonthlyChargeMillicents: 1500,
			},
			balance: 100,
			charged: 200,
			cost:    1000,
			wantErr: false,
		},
		{
			name: "auto topup blocked when monthly cap exceeded",
			account: &billingmodels.CreditAccount{
				CompanyID:                        "c-1",
				CreditAutoTopupMillicents:        1000,
				CreditMaxMonthlyChargeMillicents: 1200,
			},
			balance: 100,
			charged: 600,
			cost:    1000,
			wantErr: true,
		},
		{
			name: "topup available but insufficient cushion still fails",
			account: &billingmodels.CreditAccount{
				CompanyID:                        "c-1",
				CreditAutoTopupMillicents:        300,
				CreditMaxMonthlyChargeMillicents: 1000,
			},
			balance: 100,
			charged: 0,
			cost:    900,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.EvaluateCreditAvailability(tc.account, tc.balance, tc.charged, tc.pending, tc.cost)
			if tc.wantErr {
				require.ErrorIs(t, err, service.ErrInsufficientCredit)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestReadCreditState_BoundaryAndParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		valsForKeys func(keys billing.CompanyKeys) map[string]string
		mgetErr     error
		want        service.CreditState
		wantErrText string
	}{
		{
			name:        "valkey error bubbles up",
			mgetErr:     errors.New("backend down"),
			wantErrText: "backend down",
		},
		{
			name: "missing balance returns unpopulated state",
			valsForKeys: func(keys billing.CompanyKeys) map[string]string {
				return map[string]string{
					"some_other_key": "1",
				}
			},
			want: service.CreditState{},
		},
		{
			name: "missing charged returns unpopulated state",
			valsForKeys: func(keys billing.CompanyKeys) map[string]string {
				return map[string]string{
					keys.Balance: "10",
				}
			},
			want: service.CreditState{},
		},
		{
			name: "invalid balance fails parse",
			valsForKeys: func(keys billing.CompanyKeys) map[string]string {
				return map[string]string{
					keys.Balance: "bad",
					keys.Charged: "10",
				}
			},
			wantErrText: "invalid syntax",
		},
		{
			name: "invalid pending fails parse",
			valsForKeys: func(keys billing.CompanyKeys) map[string]string {
				return map[string]string{
					keys.Balance: "100",
					keys.Charged: "20",
					keys.Pending: "not-a-number",
				}
			},
			wantErrText: "invalid syntax",
		},
		{
			name: "fully populated state parses successfully",
			valsForKeys: func(keys billing.CompanyKeys) map[string]string {
				return map[string]string{
					keys.Balance: "500",
					keys.Charged: "100",
					keys.Pending: "42",
				}
			},
			want: service.CreditState{
				Balance:   500,
				Charged:   100,
				Pending:   42,
				Populated: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			defer deps.Ctrl.Finish()

			companyID := "c-1"
			keys := billing.KeysFor(companyID)

			call := deps.Valkey.EXPECT().MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending)
			if tc.mgetErr != nil {
				call.Return(nil, tc.mgetErr)
			} else {
				call.Return(tc.valsForKeys(keys), nil)
			}

			svc := deps.NewPlanService(&config.SubscriptionConfig{})
			got, err := svc.ReadCreditState(context.Background(), companyID)

			if tc.wantErrText != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.wantErrText), "expected %q to contain %q", err.Error(), tc.wantErrText)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckCreditAvailable_FailsClosedWhenValkeyIsUnavailable(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	companyID := "c-valkey-down"
	subCfg := &config.SubscriptionConfig{
		Credit: config.CreditConfig{
			ContractCostMillicents: 100,
		},
	}
	account := &billingmodels.CreditAccount{
		CompanyID:                        companyID,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	cacheKey := database.PlanCachePrefix + companyID
	deps.Valkey.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)
	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(account, nil)
	deps.Valkey.EXPECT().Set(gomock.Any(), cacheKey, gomock.Any(), gomock.Any()).Return(nil)

	keys := billing.KeysFor(companyID)
	deps.Valkey.EXPECT().
		MGet(gomock.Any(), keys.Balance, keys.Charged, keys.Pending).
		Return(nil, errors.New("valkey unavailable"))

	svc := deps.NewPlanService(subCfg)
	err := svc.CheckCreditAvailable(context.Background(), companyID, service.MeterContractCreate, 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "credit system temporarily unavailable")
}
