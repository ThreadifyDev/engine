package handlers

import (
	"context"
	"net/http"

	"threadify-go/api/internal/dto"

	"github.com/gin-gonic/gin"

	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"
	shareddomain "threadify-go/shared/domain"
)

type billingAPI interface {
	GetCreditAccount(ctx context.Context, companyID string) (*shareddomain.CreditAccount, error)
	CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents int64) (string, error)
	UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error
}

type BillingHandler struct {
	billingService billingAPI
}

func NewBillingHandler(
	billingService billingAPI,
) *BillingHandler {
	return &BillingHandler{
		billingService: billingService,
	}
}

func (h *BillingHandler) UpdateMaxMonthlyCharge(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	var req dto.UpdateMaxMonthlyRequest
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify topup settings"})
		return
	}
	if account == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "billing account not found"})
		return
	}

	if err := account.ValidateSpendingLimit(req.MaxMonthlyChargeMillicents); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_LIMIT",
			"message": err.Error(),
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update max monthly charge"})
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse{Status: "success"})
}

func (h *BillingHandler) CreateCheckoutSession(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	var req dto.CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.AmountMillicents == nil || *req.AmountMillicents <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount_millicents must be greater than zero"})
		return
	}

	url, err := h.billingService.CreateCheckoutSession(
		c.Request.Context(),
		compID,
		*req.AmountMillicents,
	)
	if err != nil {
		if de := serror.GetDomainError(err); de != nil {
			c.JSON(de.Code, gin.H{"error": de.Message})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	c.JSON(http.StatusOK, dto.CheckoutResponse{URL: url})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve billing info"})
		return
	}

	c.JSON(http.StatusOK, dto.GetCurrentPlanResponse{
		CreditAccount: mapAccountToDTO(account),
	})
}

func mapAccountToDTO(m *shareddomain.CreditAccount) *dto.CreditAccount {
	if m == nil {
		return nil
	}
	return &dto.CreditAccount{
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
