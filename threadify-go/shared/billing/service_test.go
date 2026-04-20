package billing

import (
	"context"
	"errors"
	"testing"

	billingmocks "threadify-go/shared/billing/mocks"
	"threadify-go/shared/config"
	"threadify-go/shared/models"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBillingService_GetCreditAccount(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name      string
		setupMock func(repo *billingmocks.MockPlanRepository)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(repo *billingmocks.MockPlanRepository) {
				repo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(&models.CreditAccount{ID: "acc_123"}, nil)
			},
		},
		{
			name: "repo_error",
			setupMock: func(repo *billingmocks.MockPlanRepository) {
				repo.EXPECT().GetCreditAccount(gomock.Any(), companyID).Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := billingmocks.NewMockPlanRepository(ctrl)
			tt.setupMock(repo)

			svc := NewBillingService(nil, repo, nil, nil, zap.NewNop())
			res, err := svc.GetCreditAccount(context.Background(), companyID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, res)
				assert.Equal(t, "acc_123", res.ID)
			}
		})
	}
}

func TestBillingService_CreateCheckoutSession(t *testing.T) {
	const (
		companyID = "comp_123"
		extCustID = "cus_123"
		amount    = 5000000
	)

	cfg := &config.BillingConfig{
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	}

	tests := []struct {
		name      string
		amount    int64
		setupMock func(repo *billingmocks.MockPlanRepository, provider *billingmocks.MockBillingProvider)
		wantErr   bool
		wantURL   string
	}{
		{
			name:   "success",
			amount: amount,
			setupMock: func(repo *billingmocks.MockPlanRepository, provider *billingmocks.MockBillingProvider) {
				repo.EXPECT().GetExternalCustomerID(gomock.Any(), companyID).Return(extCustID, nil)
				provider.EXPECT().CreateCheckoutSession(gomock.Any()).Return("https://stripe.com/checkout", nil)
			},
			wantURL: "https://stripe.com/checkout",
		},
		{
			name:      "zero_amount",
			amount:    0,
			setupMock: func(repo *billingmocks.MockPlanRepository, provider *billingmocks.MockBillingProvider) {},
			wantErr:   true,
		},
		{
			name:      "negative_amount",
			amount:    -1000,
			setupMock: func(repo *billingmocks.MockPlanRepository, provider *billingmocks.MockBillingProvider) {},
			wantErr:   true,
		},
		{
			name:   "plan_repo_error",
			amount: amount,
			setupMock: func(repo *billingmocks.MockPlanRepository, provider *billingmocks.MockBillingProvider) {
				repo.EXPECT().GetExternalCustomerID(gomock.Any(), companyID).Return("", errors.New("not found"))
			},
			wantErr: true,
		},
		{
			name:   "provider_error",
			amount: amount,
			setupMock: func(repo *billingmocks.MockPlanRepository, provider *billingmocks.MockBillingProvider) {
				repo.EXPECT().GetExternalCustomerID(gomock.Any(), companyID).Return(extCustID, nil)
				provider.EXPECT().CreateCheckoutSession(gomock.Any()).Return("", errors.New("stripe error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := billingmocks.NewMockPlanRepository(ctrl)
			provider := billingmocks.NewMockBillingProvider(ctrl)
			tt.setupMock(repo, provider)

			svc := NewBillingService(provider, repo, nil, cfg, zap.NewNop())
			url, err := svc.CreateCheckoutSession(context.Background(), companyID, tt.amount)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, url)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantURL, url)
			}
		})
	}
}

func TestBillingService_UpdateMaxMonthlyCharge(t *testing.T) {
	const (
		companyID = "comp_123"
		limit     = 10000000
	)

	tests := []struct {
		name      string
		setupMock func(repo *billingmocks.MockPlanRepository)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(repo *billingmocks.MockPlanRepository) {
				repo.EXPECT().UpdateMaxMonthlyCharge(gomock.Any(), companyID, int64(limit)).Return(nil)
			},
		},
		{
			name: "repo_error",
			setupMock: func(repo *billingmocks.MockPlanRepository) {
				repo.EXPECT().UpdateMaxMonthlyCharge(gomock.Any(), companyID, int64(limit)).Return(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := billingmocks.NewMockPlanRepository(ctrl)
			tt.setupMock(repo)

			svc := NewBillingService(nil, repo, nil, nil, zap.NewNop())
			err := svc.UpdateMaxMonthlyCharge(context.Background(), companyID, limit)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
