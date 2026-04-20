package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	billingmodels "threadify-go/shared/models"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	natsmocks "github.com/threadify/engine/internal/service/mocks/nats"
	"github.com/threadify/engine/internal/service/tests/common"
	"go.uber.org/zap"
)

func validTopupPayload(companyID, eventID string) []byte {
	return []byte(fmt.Sprintf(`{
		"company_id":"%s",
		"event_id":"%s",
		"billing_cycle_start":"2026-04-01T00:00:00Z",
		"amount":"5000"
	}`, companyID, eventID))
}

func newCronWithOrchestrator(deps *common.MockedDependencies) *service.BillingCron {
	return service.NewBillingCron(
		newBillingOrchestratorForTest(deps),
		nil,
		nil,
		zap.NewNop(),
	)
}

func TestBillingCron_HandleCreditTopup_MalformedPayloadReturnsPermanent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte("{bad-json"))

	cron := service.NewBillingCron(nil, nil, nil, zap.NewNop())
	err := cron.HandleCreditTopup(msg)

	requirePermanentError(t, err, "unmarshal credit topup")
}

func TestBillingCron_HandleCreditTopup_MissingRequiredFieldsReturnsPermanent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{"event_id":"evt-1","amount":"1000"}`))

	cron := service.NewBillingCron(nil, nil, nil, zap.NewNop())
	err := cron.HandleCreditTopup(msg)

	requirePermanentError(t, err, "invalid credit topup event")
}

func TestBillingCron_HandleCreditTopup_InvalidAmountReturnsPermanent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{
		"company_id":"comp-1",
		"event_id":"evt-1",
		"billing_cycle_start":"2026-04-01T00:00:00Z",
		"amount":12.34
	}`))

	cron := service.NewBillingCron(nil, nil, nil, zap.NewNop())
	err := cron.HandleCreditTopup(msg)

	requirePermanentError(t, err, "parse credit amount")
}

func TestBillingCron_HandleCreditTopup_MissingAmountReturnsPermanent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{
		"company_id":"comp-missing-amount",
		"event_id":"evt-missing-amount",
		"billing_cycle_start":"2026-04-01T00:00:00Z"
	}`))

	cron := service.NewBillingCron(nil, nil, nil, zap.NewNop())
	err := cron.HandleCreditTopup(msg)

	requirePermanentError(t, err, "parse credit amount")
}

func TestBillingCron_HandleCreditTopup_NonPositiveAmountReturnsPermanent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{
		"company_id":"comp-2",
		"event_id":"evt-2",
		"billing_cycle_start":"2026-04-01T00:00:00Z",
		"amount":"0"
	}`))

	cron := service.NewBillingCron(nil, nil, nil, zap.NewNop())
	err := cron.HandleCreditTopup(msg)

	requirePermanentError(t, err, "invalid credit topup amount")
}

func TestBillingCron_HandleCreditTopup_InvalidBillingCycleReturnsPermanent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{
		"company_id":"comp-3",
		"event_id":"evt-3",
		"billing_cycle_start":"not-a-time",
		"amount":"1000"
	}`))

	cron := service.NewBillingCron(nil, nil, nil, zap.NewNop())
	err := cron.HandleCreditTopup(msg)

	requirePermanentError(t, err, "invalid billing_cycle_start")
}

func TestBillingCron_HandleCreditTopup_TransientServiceErrorIsNotPermanent(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), gomock.Any()).Return(nil, errors.New("db unavailable"))

	msg := natsmocks.NewMockMsg(deps.Ctrl)
	msg.EXPECT().Data().Return(validTopupPayload("comp-transient", "evt-transient"))

	err := newCronWithOrchestrator(deps).HandleCreditTopup(msg)

	require.Error(t, err)
	require.False(t, errors.As(err, new(*service.PermanentError)))
	require.Contains(t, err.Error(), "find credit account for credit topup")
}

func TestBillingCron_HandleCreditTopup_AutoTopupDisabledIsNotPermanent(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), gomock.Any()).Return(&billingmodels.CreditAccount{
		CompanyID:                        "comp-disabled",
		ExternalCustomerID:               "cus_disabled",
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}, nil)

	msg := natsmocks.NewMockMsg(deps.Ctrl)
	msg.EXPECT().Data().Return(validTopupPayload("comp-disabled", "evt-disabled"))

	err := newCronWithOrchestrator(deps).HandleCreditTopup(msg)

	require.Error(t, err)
	require.Contains(t, err.Error(), "auto-topup disabled")
	require.False(t, errors.As(err, new(*service.PermanentError)))
}

func TestBillingCron_HandleCreditTopup_ValidEventChargesSuccessfully(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	const companyID = "comp-success"
	const eventID = "evt-success"

	deps.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), gomock.Any()).Return(&billingmodels.CreditAccount{
		CompanyID:                        companyID,
		ExternalCustomerID:               "cus_123",
		CreditAutoTopupMillicents:        5000,
		CreditMaxMonthlyChargeMillicents: 50000,
	}, nil)

	deps.BillingRepo.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, snapshot *billingmodels.BillingSnapshot) error {
			require.Equal(t, eventID, snapshot.ID)
			require.Equal(t, companyID, snapshot.CompanyID)
			require.Equal(t, int64(5), snapshot.TotalCents)
			require.Equal(t, billingmodels.PaymentStatusPaid, snapshot.PaymentStatus)
			return nil
		},
	)

	msg := natsmocks.NewMockMsg(deps.Ctrl)
	msg.EXPECT().Data().Return(validTopupPayload(companyID, eventID))

	err := newCronWithOrchestrator(deps).HandleCreditTopup(msg)
	require.NoError(t, err)
}

func TestBillingCron_RunRollover_DelegatesToPlanService(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	before := time.Now()
	deps.PlanSvc.EXPECT().ProcessRollovers(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, before.Add(5*time.Minute), deadline, 3*time.Second)
		return nil
	})

	newCronWithOrchestrator(deps).RunRollover()
}

func TestBillingCron_RunRollover_ServiceErrorDoesNotPanic(t *testing.T) {
	deps := common.NewMockDeps(t)
	defer deps.Ctrl.Finish()

	deps.PlanSvc.EXPECT().ProcessRollovers(gomock.Any()).Return(errors.New("rollover failed"))

	newCronWithOrchestrator(deps).RunRollover()
}

func requirePermanentError(t *testing.T, err error, substr string) {
	t.Helper()
	require.Error(t, err)
	require.ErrorAs(t, err, new(*service.PermanentError))
	require.Contains(t, err.Error(), substr)
}
