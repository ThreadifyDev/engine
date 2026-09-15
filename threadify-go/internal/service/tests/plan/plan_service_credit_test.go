package service_test

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/service/tests/common"
	"math"
	"testing"
	shareddomain "threadify-go/shared/domain"
)

// Existing credit rows are historical data. Runtime operations must not read,
// debit, or require them; these strict mocks have no database/cache expectations.
func TestRuntimeOperationsDoNotRequireCredits(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()
	svc := deps.NewPlanService(&config.SubscriptionConfig{})
	ctx := context.Background()
	account, err := svc.CheckBalancePositive(ctx, "company")
	require.NoError(t, err)
	require.Equal(t, "company", account.CompanyID)
	require.NoError(t, svc.CheckCreditAvailable(ctx, "company", "llm_token_usage", math.MaxInt64))
	require.NoError(t, svc.DecrementLLMUsage(ctx, "company", math.MaxInt64))
	require.NoError(t, svc.DecrementIngress(ctx, "company", math.MaxInt64))
	require.NoError(t, svc.DecrementEgress(ctx, "company", math.MaxInt64))
	require.NoError(t, svc.ChargeContract(ctx, "company"))
	require.NoError(t, svc.ChargeContractVersion(ctx, "company"))
	require.NoError(t, svc.CheckPayloadSize(ctx, &shareddomain.CreditAccount{PayloadLimitBytes: 1}, math.MaxInt64))
}
