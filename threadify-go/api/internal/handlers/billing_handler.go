package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
	"threadify-go/shared/config"
)

type BillingHandler struct {
	billingProvider billing.BillingProvider
	subConfig       *config.SubscriptionConfig
	billingConfig   *config.BillingConfig
	logger          *zap.Logger
}

func NewBillingHandler(
	billingProvider billing.BillingProvider,
	subConfig *config.SubscriptionConfig,
	billingConfig *config.BillingConfig,
	logger *zap.Logger,
) *BillingHandler {
	return &BillingHandler{
		billingProvider: billingProvider,
		subConfig:       subConfig,
		billingConfig:   billingConfig,
		logger:          logger,
	}
}

type CreateCheckoutSessionRequest struct {
	Tier         string `json:"tier"         binding:"required"`
	BillingCycle string `json:"billing_cycle" binding:"required"`
}

type CreateCheckoutSessionResponse struct {
	CheckoutURL string `json:"checkoutUrl"`
}

func (h *BillingHandler) CreateCheckoutSession(c *gin.Context) {
	var req CreateCheckoutSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		h.logger.Error("companyID missing from context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tierConfig := h.subConfig.GetTierLimits(req.Tier)
	if tierConfig == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription tier"})
		return
	}

	providerParams, err := h.billingConfig.GetProviderParams(req.Tier)
	if err != nil {
		h.logger.Error("missing provider params for tier",
			zap.String("tier", req.Tier),
			zap.String("billing_cycle", req.BillingCycle),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "billing configuration error"})
		return
	}

	params := &billing.CheckoutSessionParams{
		CompanyID:      companyID.(string),
		Tier:           req.Tier,
		BillingCycle:   req.BillingCycle,
		SuccessURL:     h.billingConfig.SuccessURL,
		CancelURL:      h.billingConfig.CancelURL,
		ProviderParams: providerParams,
	}

	checkoutURL, err := h.billingProvider.CreateCheckoutSession(params)
	if err != nil {
		h.logger.Error("failed to create checkout session",
			zap.String("company_id", companyID.(string)),
			zap.String("tier", req.Tier),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize checkout"})
		return
	}

	h.logger.Info("checkout session created",
		zap.String("company_id", companyID.(string)),
		zap.String("tier", req.Tier),
		zap.String("billing_cycle", req.BillingCycle),
	)

	c.JSON(http.StatusOK, CreateCheckoutSessionResponse{CheckoutURL: checkoutURL})
}
