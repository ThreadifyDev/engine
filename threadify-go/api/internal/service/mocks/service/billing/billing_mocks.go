package svcmocks

import (
	"context"
	"reflect"
	shareddomain "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
)

type MockBillingService struct {
	ctrl     *gomock.Controller
	recorder *MockBillingServiceMockRecorder
}

type MockBillingServiceMockRecorder struct {
	mock *MockBillingService
}

func NewMockBillingService(ctrl *gomock.Controller) *MockBillingService {
	mock := &MockBillingService{ctrl: ctrl}
	mock.recorder = &MockBillingServiceMockRecorder{mock}
	return mock
}

func (m *MockBillingService) EXPECT() *MockBillingServiceMockRecorder {
	return m.recorder
}

func (m *MockBillingService) GetCreditAccount(ctx context.Context, companyID string) (*shareddomain.CreditAccount, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCreditAccount", ctx, companyID)
	ret0, _ := ret[0].(*shareddomain.CreditAccount)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockBillingServiceMockRecorder) GetCreditAccount(ctx, companyID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCreditAccount", reflect.TypeOf((*MockBillingService)(nil).GetCreditAccount), ctx, companyID)
}

func (m *MockBillingService) CreateCheckoutSession(ctx context.Context, companyID string, amount int64) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateCheckoutSession", ctx, companyID, amount)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockBillingServiceMockRecorder) CreateCheckoutSession(ctx, companyID, amount interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateCheckoutSession", reflect.TypeOf((*MockBillingService)(nil).CreateCheckoutSession), ctx, companyID, amount)
}

func (m *MockBillingService) UpdateMaxMonthlyCharge(ctx context.Context, companyID string, amount int64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateMaxMonthlyCharge", ctx, companyID, amount)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockBillingServiceMockRecorder) UpdateMaxMonthlyCharge(ctx, companyID, amount interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateMaxMonthlyCharge", reflect.TypeOf((*MockBillingService)(nil).UpdateMaxMonthlyCharge), ctx, companyID, amount)
}

func (m *MockBillingService) ProvisionSignupCredits(ctx context.Context, companyID string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProvisionSignupCredits", ctx, companyID)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockBillingServiceMockRecorder) ProvisionSignupCredits(ctx, companyID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProvisionSignupCredits", reflect.TypeOf((*MockBillingService)(nil).ProvisionSignupCredits), ctx, companyID)
}
