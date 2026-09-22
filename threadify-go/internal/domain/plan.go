package domain

import (
	"context"
	shareddomain "threadify-go/shared/domain"
)

// The superset mock also supports tests for the retained legacy billing services.
//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/plan_service_mock.go threadify-go/shared/billing PlanService

// PlanService checks Registry access for Engine operations. Transport middleware
// owns bandwidth and request accounting; local credit billing is not part of it.
type PlanService interface {
	ChargeContract(context.Context, string) error
	ChargeContractVersion(context.Context, string) error
	DecrementEgress(context.Context, string, int64) error
	DecrementIngress(context.Context, string, int64) error
	DecrementLLMUsage(context.Context, string, int64) error
	GetCurrentLimits(context.Context, string) (*shareddomain.CreditAccount, error)
	CheckPayloadSize(context.Context, *shareddomain.CreditAccount, int64) error
	CheckCreditAvailable(context.Context, string, string, int64) error
	CheckBalancePositive(context.Context, string) (*shareddomain.CreditAccount, error)
}
