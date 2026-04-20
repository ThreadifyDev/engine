package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"
	billingmodels "threadify-go/shared/models"
)

type billingAPI interface {
	GetCreditAccount(ctx context.Context, companyID string) (*billingmodels.CreditAccount, error)
	CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents int64) (string, error)
	UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error
}

type BillingHandler struct {
	billingService billingAPI
	logger         *zap.Logger
}

type UpdateMaxMonthlyRequest struct {
	MaxMonthlyChargeMillicents int64 `json:"max_monthly_millicents" binding:"min=0"`
}

func (h *BillingHandler) UpdateMaxMonthlyCharge(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	var req UpdateMaxMonthlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	account, err := h.billingService.GetCreditAccount(c.Request.Context(), compID)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		h.logger.Error("failed to fetch credit account for validation", zap.Error(err), zap.String("companyID", compID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify topup settings"})
		return
	}
	if account == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "billing account not found"})
		return
	}

	if req.MaxMonthlyChargeMillicents > 0 && req.MaxMonthlyChargeMillicents < account.CreditAutoTopupMillicents {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_LIMIT",
			"message": fmt.Sprintf("spending limit (%d) must be at least as high as the auto-topup amount (%d)", req.MaxMonthlyChargeMillicents, account.CreditAutoTopupMillicents),
		})
		return
	}

	err = h.billingService.UpdateMaxMonthlyCharge(
		c.Request.Context(),
		compID,
		req.MaxMonthlyChargeMillicents,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		h.logger.Error("failed to update max monthly charge",
			zap.Error(err),
			zap.String("companyID", compID),
			zap.Int64("max_monthly", req.MaxMonthlyChargeMillicents),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update max monthly charge"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
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
	AmountMillicents int64 `json:"amount_millicents" binding:"required"`
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
	if req.AmountMillicents <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount_millicents must be greater than zero"})
		return
	}

	url, err := h.billingService.CreateCheckoutSession(
		c.Request.Context(),
		compID,
		req.AmountMillicents,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
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
	ID                         string    `json:"id"`
	CompanyID                  string    `json:"company_id"`
	BillingCycleStart          time.Time `json:"billing_cycle_start"`
	BalanceMillicents          int64     `json:"balance_millicents"`
	MinBalanceMillicents       int64     `json:"min_balance_millicents"`
	MaxMonthlyChargeMillicents int64     `json:"max_monthly_charge_millicents"`
	AutoTopupMillicents        int64     `json:"auto_topup_millicents"`
	MonthlyChargedMillicents   int64     `json:"monthly_charged_millicents"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type GetCurrentPlanResponse struct {
	CreditAccount *CreditAccountDTO `json:"credit_account"`
}

func mapAccountToDTO(m *billingmodels.CreditAccount) *CreditAccountDTO {
	if m == nil {
		return nil
	}
	return &CreditAccountDTO{
		ID:                         m.ID,
		CompanyID:                  m.CompanyID,
		BillingCycleStart:          m.BillingCycleStart,
		BalanceMillicents:          m.CreditBalanceMillicents,
		MinBalanceMillicents:       m.CreditMinBalanceMillicents,
		MaxMonthlyChargeMillicents: m.CreditMaxMonthlyChargeMillicents,
		AutoTopupMillicents:        m.CreditAutoTopupMillicents,
		MonthlyChargedMillicents:   m.CreditMonthlyChargedMillicents,
		CreatedAt:                  m.CreatedAt,
		UpdatedAt:                  m.UpdatedAt,
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
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}
		h.logger.Error("failed to get current billing info", zap.Error(err), zap.String("companyID", compID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve billing info"})
		return
	}

	c.JSON(http.StatusOK, GetCurrentPlanResponse{
		CreditAccount: mapAccountToDTO(account),
	})
}
