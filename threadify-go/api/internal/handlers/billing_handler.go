package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
)

type billingAPI interface {
	GetCreditAccount(ctx context.Context, companyID string) (*billing.CreditAccount, error)
	CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents, maxMonthlyMillicents int64) (string, error)
}

type BillingHandler struct {
	billingService billingAPI
	logger         *zap.Logger
}

func NewBillingHandler(
	billingService billingAPI,
	logger *zap.Logger,
) *BillingHandler {
	return &BillingHandler{
		billingService: billingService,
		logger:         logger,
	}
}

type CheckoutRequest struct {
	AmountMillicents     int64 `json:"amount_millicents"      binding:"required,gt=0"`
	MaxMonthlyMillicents int64 `json:"max_monthly_millicents"`
}

func (h *BillingHandler) CreateCheckoutSession(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	var req CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	url, err := h.billingService.CreateCheckoutSession(
		c.Request.Context(),
		compID,
		req.AmountMillicents,
		req.MaxMonthlyMillicents,
	)
	if err != nil {
		h.logger.Error("failed to create checkout session",
			zap.Error(err),
			zap.String("companyID", compID),
			zap.Int64("amount_millicents", req.AmountMillicents),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

type CreditAccountDTO struct {
	ID                    string    `json:"id"`
	CompanyID             string    `json:"company_id"`
	BillingCycleStart     time.Time `json:"billing_cycle_start"`
	BalanceCents          int64     `json:"balance_cents"`
	MinBalanceCents       int64     `json:"min_balance_cents"`
	MaxMonthlyChargeCents int64     `json:"max_monthly_charge_cents"`
	AutoTopupCents        int64     `json:"auto_topup_cents"`
	MonthlyChargedCents   int64     `json:"monthly_charged_cents"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type GetCurrentPlanResponse struct {
	CreditAccount *CreditAccountDTO `json:"credit_account"`
}

func mapAccountToDTO(m *billing.CreditAccount) *CreditAccountDTO {
	if m == nil {
		return nil
	}
	return &CreditAccountDTO{
		ID:                    m.ID,
		CompanyID:             m.CompanyID,
		BillingCycleStart:     m.BillingCycleStart,
		BalanceCents:          m.CreditBalanceMillicents / 1000,
		MinBalanceCents:       m.CreditMinBalanceMillicents / 1000,
		MaxMonthlyChargeCents: m.CreditMaxMonthlyChargeMillicents / 1000,
		AutoTopupCents:        m.CreditAutoTopupMillicents / 1000,
		MonthlyChargedCents:   m.CreditMonthlyChargedMillicents / 1000,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}
}
func (h *BillingHandler) GetCurrentPlan(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	account, err := h.billingService.GetCreditAccount(c.Request.Context(), compID)
	if err != nil {
		h.logger.Error("failed to get current billing info", zap.Error(err), zap.String("companyID", compID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve billing info"})
		return
	}

	c.JSON(http.StatusOK, GetCurrentPlanResponse{
		CreditAccount: mapAccountToDTO(account),
	})
}
