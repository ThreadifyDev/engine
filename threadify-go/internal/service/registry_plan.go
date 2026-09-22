package service

import (
	"context"
	"github.com/threadify/engine/internal/domain"
	shareddomain "threadify-go/shared/domain"
	"threadify-go/shared/registry"
)

// RegistryPlanService retains the operation checks used by Engine handlers
// without constructing the retired credit ledger, payment provider or workers.
// Request and bandwidth allowances are enforced at the transport boundary.
type RegistryPlanService struct{}

var _ domain.PlanService = (*RegistryPlanService)(nil)

func NewRegistryPlanService() *RegistryPlanService { return &RegistryPlanService{} }

func (s *RegistryPlanService) GetCurrentLimits(ctx context.Context, companyID string) (*shareddomain.CreditAccount, error) {
	return &shareddomain.CreditAccount{CompanyID: companyID}, registry.Default().CheckCompany(companyID)
}
func (s *RegistryPlanService) CheckBalancePositive(ctx context.Context, companyID string) (*shareddomain.CreditAccount, error) {
	return s.GetCurrentLimits(ctx, companyID)
}
func (s *RegistryPlanService) CheckPayloadSize(context.Context, *shareddomain.CreditAccount, int64) error {
	return nil
}
func (s *RegistryPlanService) CheckCreditAvailable(ctx context.Context, companyID, meter string, amount int64) error {
	return registry.Default().CheckCompany(companyID)
}
func (s *RegistryPlanService) ChargeContract(ctx context.Context, companyID string) error {
	return registry.Default().CheckCompany(companyID)
}
func (s *RegistryPlanService) ChargeContractVersion(ctx context.Context, companyID string) error {
	return registry.Default().CheckCompany(companyID)
}
func (s *RegistryPlanService) DecrementEgress(ctx context.Context, companyID string, bytes int64) error {
	return registry.Default().CheckCompany(companyID)
}
func (s *RegistryPlanService) DecrementIngress(ctx context.Context, companyID string, count int64) error {
	return registry.Default().CheckCompany(companyID)
}
func (s *RegistryPlanService) DecrementLLMUsage(ctx context.Context, companyID string, tokens int64) error {
	return registry.Default().CheckCompany(companyID)
}
