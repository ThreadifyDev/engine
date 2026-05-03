package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	shareddomain "threadify-go/shared/domain"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestBillingHandler_UpdateMaxMonthlyCharge(t *testing.T) {
	const companyID = "comp_billing_1"

	newRouter := func(deps *common.MockedHandlers) *gin.Engine {
		h := handlers.NewBillingHandler(deps.NewBillingService())
		r := common.SetupTestRouter()
		r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: "user_1"}))
		r.PUT("/billing/spending-limit", h.UpdateMaxMonthlyCharge)
		return r
	}

	tests := []struct {
		name       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]int64{"max_monthly_millicents": 5000},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&shareddomain.CreditAccount{CreditAutoTopupMillicents: 1000}, nil)
				d.PlanRepo.EXPECT().UpdateMaxMonthlyCharge(gomock.Any(), companyID, int64(5000)).Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "limit_too_low",
			body: map[string]int64{"max_monthly_millicents": 500},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&shareddomain.CreditAccount{CreditAutoTopupMillicents: 1000}, nil)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "account_not_found",
			body: map[string]int64{"max_monthly_millicents": 5000},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(nil, nil)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "service_error",
			body: map[string]int64{"max_monthly_millicents": 5000},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&shareddomain.CreditAccount{CreditAutoTopupMillicents: 1000}, nil)
				d.PlanRepo.EXPECT().UpdateMaxMonthlyCharge(gomock.Any(), companyID, int64(5000)).
					Return(errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "malformed_json",
			body:       "{bad}",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newRouter(deps), "PUT", "/billing/spending-limit", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBillingHandler_CreateCheckoutSession(t *testing.T) {
	const companyID = "comp_billing_1"

	newRouter := func(deps *common.MockedHandlers) *gin.Engine {
		h := handlers.NewBillingHandler(deps.NewBillingService())
		r := common.SetupTestRouter()
		r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: "user_1"}))
		r.POST("/billing/topup", h.CreateCheckoutSession)
		return r
	}

	tests := []struct {
		name       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]int64{"amount_millicents": 2000},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetExternalCustomerID(gomock.Any(), companyID).Return("ext_123", nil)
				d.BillingProvider.EXPECT().CreateCheckoutSession(shareddomain.CheckoutSessionParams{
					CompanyID:               companyID,
					InitialAmountMillicents: 2000,
					SuccessURL:              "http://ok",
					CancelURL:               "http://cancel",
					ExternalCustomerID:      "ext_123",
				}).Return("https://stripe.com/checkout", nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "plan_error",
			body: map[string]int64{"amount_millicents": 2000},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetExternalCustomerID(gomock.Any(), companyID).Return("", errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "provider_error",
			body: map[string]int64{"amount_millicents": 2000},
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetExternalCustomerID(gomock.Any(), companyID).Return("ext_123", nil)
				d.BillingProvider.EXPECT().CreateCheckoutSession(gomock.Any()).Return("", errors.New("stripe fail"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "malformed_json",
			body:       "{bad}",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newRouter(deps), "POST", "/billing/topup", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBillingHandler_GetCurrentPlan(t *testing.T) {
	const companyID = "comp_billing_1"

	newRouter := func(deps *common.MockedHandlers) *gin.Engine {
		h := handlers.NewBillingHandler(deps.NewBillingService())
		r := common.SetupTestRouter()
		r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: "user_1"}))
		r.GET("/billing/plan", h.GetCurrentPlan)
		return r
	}

	tests := []struct {
		name       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&shareddomain.CreditAccount{CompanyID: companyID}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(nil, serror.NewDomainError("not found", http.StatusNotFound))
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "service_error",
			setupMock: func(d *common.MockedHandlers) {
				d.PlanRepo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(nil, errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newRouter(deps), "GET", "/billing/plan", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
