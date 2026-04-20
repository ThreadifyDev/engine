package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"threadify-go/shared/billing"
	sharedconfig "threadify-go/shared/config"
	"threadify-go/shared/database"
	billingmodels "threadify-go/shared/models"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/service/tests/common"
)

func newBillingOrchestratorForTest(deps *common.MockedDependencies) *service.BillingOrchestrator {
	return service.NewBillingOrchestrator(
		billing.NewNoOpBillingProvider(),
		deps.PlanRepo,
		deps.BillingRepo,
		&sharedconfig.SubscriptionConfig{},
		&sharedconfig.BillingConfig{},
		deps.Valkey,
		deps.PlanSvc,
		deps.Logger,
	)
}

const (
	creditTopupAppliedKeyTTL = 90 * 24 * time.Hour

	fieldEventID           = billingmodels.FieldEventID
	fieldCompanyID         = billingmodels.FieldCompanyID
	fieldMeter             = billingmodels.FieldMeter
	fieldAmount            = billingmodels.FieldAmount
	fieldBillingCycleStart = billingmodels.FieldBillingCycleStart
	fieldTimestamp         = billingmodels.FieldTimestamp
)

var cycleStart = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

func newTopupSnapshot(id, companyID string, totalCents int64) *billingmodels.BillingSnapshot {
	return &billingmodels.BillingSnapshot{
		ID:          id,
		CompanyID:   companyID,
		PeriodStart: cycleStart,
		TotalCents:  totalCents,
	}
}

func TestBillingOrchestrator_ChargeCreditTopup_SkipInvoicingCreatesPaidSnapshot(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-1"
	const eventID = "evt-1"
	svc := newBillingOrchestratorForTest(deps)

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&billingmodels.CreditAccount{
		CompanyID:                        companyID,
		ExternalCustomerID:               "cus_123",
		CreditAutoTopupMillicents:        2000,
		CreditMaxMonthlyChargeMillicents: 10000,
	}, nil)

	deps.BillingRepo.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, snap *billingmodels.BillingSnapshot) error {
			require.Equal(t, eventID, snap.ID)
			require.Equal(t, companyID, snap.CompanyID)
			require.Equal(t, cycleStart, snap.PeriodStart)
			require.Equal(t, int64(5), snap.TotalCents)
			require.Equal(t, billingmodels.SnapshotReasonCreditTopup, snap.Reason)
			require.Equal(t, "noop", snap.ProviderName)
			require.Equal(t, billingmodels.PaymentStatusPaid, snap.PaymentStatus)
			require.Equal(t, "cus_123", snap.ExternalCustomerID)
			require.False(t, snap.PeriodEnd.IsZero())
			require.False(t, snap.CreatedAt.IsZero())
			return nil
		},
	)

	err := svc.ChargeCreditTopup(context.Background(), companyID, eventID, cycleStart, 5000)
	require.NoError(t, err)
}

func TestBillingOrchestrator_ChargeCreditTopup_AutoTopupDisabledReturnsError(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-2"
	svc := newBillingOrchestratorForTest(deps)

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&billingmodels.CreditAccount{
		CompanyID:                        companyID,
		ExternalCustomerID:               "cus_disabled",
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}, nil)

	err := svc.ChargeCreditTopup(context.Background(), companyID, "evt-disabled", cycleStart, 8000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auto-topup disabled")
}

func TestBillingOrchestrator_ChargeCreditTopup_ZeroAmountIsNoOp(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	err := newBillingOrchestratorForTest(deps).
		ChargeCreditTopup(context.Background(), "comp-3", "evt-zero", cycleStart, 0)
	require.NoError(t, err)
}

func TestBillingOrchestrator_ChargeCreditTopup_GetCreditAccountError(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-lookup-error"
	svc := newBillingOrchestratorForTest(deps)

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).
		Return(nil, errors.New("plan repo unavailable"))

	err := svc.ChargeCreditTopup(context.Background(), companyID, "evt-lookup-error", cycleStart, 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "find credit account for credit topup")
}

func TestBillingOrchestrator_ChargeCreditTopup_CreateSnapshotError(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-snapshot-error"
	svc := newBillingOrchestratorForTest(deps)

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&billingmodels.CreditAccount{
		CompanyID:                        companyID,
		ExternalCustomerID:               "cus_snapshot",
		CreditAutoTopupMillicents:        1000,
		CreditMaxMonthlyChargeMillicents: 10000,
	}, nil)
	deps.BillingRepo.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any()).
		Return(errors.New("insert failed"))

	err := svc.ChargeCreditTopup(context.Background(), companyID, "evt-snapshot-error", cycleStart, 3000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist credit topup snapshot")
}

func TestBillingOrchestrator_ApplyCreditTopup_ZeroAmountIsNoOp(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	err := newBillingOrchestratorForTest(deps).
		ApplyCreditTopup(context.Background(), newTopupSnapshot("snap-zero", "comp-9", 0))
	require.NoError(t, err)
}

func TestBillingOrchestrator_ApplyCreditTopup_AlreadyAppliedIsNoOp(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	snapshot := &billingmodels.BillingSnapshot{
		ID:                "snap-1",
		CompanyID:         "comp-4",
		ExternalInvoiceID: "in_999",
		PeriodStart:       cycleStart,
		TotalCents:        10,
	}
	svc := newBillingOrchestratorForTest(deps)

	deps.Valkey.EXPECT().
		SetNX(gomock.Any(), database.CreditTopupAppliedKeyPrefix+"in_999", "1", creditTopupAppliedKeyTTL).
		Return(false, nil)

	err := svc.ApplyCreditTopup(context.Background(), snapshot)
	require.NoError(t, err)
}

func TestBillingOrchestrator_ApplyCreditTopup_SetNXError(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	snapshot := newTopupSnapshot("snap-nx-error", "comp-7", 10)
	svc := newBillingOrchestratorForTest(deps)

	deps.Valkey.EXPECT().
		SetNX(gomock.Any(), database.CreditTopupAppliedKeyPrefix+snapshot.ID, "1", creditTopupAppliedKeyTTL).
		Return(false, errors.New("valkey unavailable"))

	err := svc.ApplyCreditTopup(context.Background(), snapshot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "apply credit topup: mark applied")
}

func TestBillingOrchestrator_ApplyCreditTopup_SuccessInvalidatesCacheAndWritesOutbox(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-5"
	snapshot := newTopupSnapshot("snap-2", companyID, 25)
	svc := newBillingOrchestratorForTest(deps)

	keys := billing.KeysFor(companyID)
	appliedKey := database.CreditTopupAppliedKeyPrefix + snapshot.ID
	const amountMillicents = int64(25000)

	deps.Valkey.EXPECT().SetNX(gomock.Any(), appliedKey, "1", creditTopupAppliedKeyTTL).Return(true, nil)
	deps.Valkey.EXPECT().
		ApplyCreditTopupAtomic(gomock.Any(), keys.Balance, keys.Pending, amountMillicents).
		Return(int64(50000), nil)
	deps.PlanSvc.EXPECT().InvalidatePlanCache(gomock.Any(), companyID)

	deps.Valkey.EXPECT().
		XAdd(gomock.Any(), database.UsageOutboxStreamKey, "*", gomock.Any()).
		DoAndReturn(func(_ context.Context, stream, id string, payload interface{}) (string, error) {
			data, ok := payload.(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, companyID, data[fieldCompanyID])
			require.Equal(t, billingmodels.MeterCreditTopup, data[fieldMeter])
			require.Equal(t, "25000", data[fieldAmount])
			require.Equal(t, snapshot.PeriodStart.Format(time.RFC3339Nano), data[fieldBillingCycleStart])
			require.NotEmpty(t, data[fieldEventID])
			require.NotEmpty(t, data[fieldTimestamp])
			return "1-0", nil
		})

	err := svc.ApplyCreditTopup(context.Background(), snapshot)
	require.NoError(t, err)
}

func TestBillingOrchestrator_ApplyCreditTopup_AtomicFailureRollsBackIdempotencyKey(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-6"
	snapshot := newTopupSnapshot("snap-3", companyID, 15)
	svc := newBillingOrchestratorForTest(deps)

	keys := billing.KeysFor(companyID)
	appliedKey := database.CreditTopupAppliedKeyPrefix + snapshot.ID

	deps.Valkey.EXPECT().SetNX(gomock.Any(), appliedKey, "1", creditTopupAppliedKeyTTL).Return(true, nil)
	deps.Valkey.EXPECT().
		ApplyCreditTopupAtomic(gomock.Any(), keys.Balance, keys.Pending, int64(15000)).
		Return(int64(0), errors.New("atomic apply failed"))
	deps.Valkey.EXPECT().Delete(gomock.Any(), appliedKey).Return(nil)

	err := svc.ApplyCreditTopup(context.Background(), snapshot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "apply credit topup")
}

func TestBillingOrchestrator_ApplyCreditTopup_AtomicFailureWithRollbackDeleteErrorStillReturnsApplyError(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-8"
	snapshot := newTopupSnapshot("snap-rollback-delete-error", companyID, 20)
	svc := newBillingOrchestratorForTest(deps)

	keys := billing.KeysFor(companyID)
	appliedKey := database.CreditTopupAppliedKeyPrefix + snapshot.ID

	deps.Valkey.EXPECT().SetNX(gomock.Any(), appliedKey, "1", creditTopupAppliedKeyTTL).Return(true, nil)
	deps.Valkey.EXPECT().
		ApplyCreditTopupAtomic(gomock.Any(), keys.Balance, keys.Pending, int64(20000)).
		Return(int64(0), errors.New("atomic apply failed"))
	deps.Valkey.EXPECT().Delete(gomock.Any(), appliedKey).Return(errors.New("delete failed"))

	err := svc.ApplyCreditTopup(context.Background(), snapshot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "apply credit topup")
}

func TestBillingOrchestrator_ClearCreditTopupPending_DeletesPendingKey(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-clear"
	keys := billing.KeysFor(companyID)
	svc := newBillingOrchestratorForTest(deps)

	deps.Valkey.EXPECT().Delete(gomock.Any(), keys.Pending).Return(nil)

	err := svc.ClearCreditTopupPending(context.Background(), companyID)
	require.NoError(t, err)
}
