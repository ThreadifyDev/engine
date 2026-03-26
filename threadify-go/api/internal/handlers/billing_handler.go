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
	DisableAutoTopup(ctx context.Context, companyID string) error
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
	ID                       string    `json:"id"`
	CompanyID                string    `json:"company_id"`
	BillingCycleStart        time.Time `json:"billing_cycle_start"`
	BalanceMillicents        int64     `json:"balance_millicents"`
	MinBalanceMillicents     int64     `json:"min_balance_millicents"`
	MaxMonthlyChargeMillicents int64   `json:"max_monthly_charge_millicents"`
	AutoTopupMillicents      int64     `json:"auto_topup_millicents"`
	MonthlyChargedMillicents int64     `json:"monthly_charged_millicents"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type GetCurrentPlanResponse struct {
	CreditAccount *CreditAccountDTO `json:"credit_account"`
}

func mapAccountToDTO(m *billing.CreditAccount) *CreditAccountDTO {
	if m == nil {
		return nil
	}
	return &CreditAccountDTO{
		ID:                       m.ID,
		CompanyID:                m.CompanyID,
		BillingCycleStart:        m.BillingCycleStart,
		BalanceMillicents:        m.CreditBalanceMillicents,
		MinBalanceMillicents:     m.CreditMinBalanceMillicents,
		MaxMonthlyChargeMillicents: m.CreditMaxMonthlyChargeMillicents,
		AutoTopupMillicents:      m.CreditAutoTopupMillicents,
		MonthlyChargedMillicents: m.CreditMonthlyChargedMillicents,
		CreatedAt:                m.CreatedAt,
		UpdatedAt:                m.UpdatedAt,
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

func (h *BillingHandler) DisableAutoTopup(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	err := h.billingService.DisableAutoTopup(c.Request.Context(), compID)
	if err != nil {
		h.logger.Error("failed to disable auto-topup", zap.Error(err), zap.String("companyID", compID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable auto-topup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Auto-topup disabled"})
}
