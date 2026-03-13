package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
)

type BillingHandler struct {
	billingService *billing.BillingService
	logger         *zap.Logger
}

func NewBillingHandler(
	billingService *billing.BillingService,
	logger *zap.Logger,
) *BillingHandler {
	return &BillingHandler{
		billingService: billingService,
		logger:         logger,
	}
}

type CreateCheckoutSessionRequest struct {
	Tier         string `json:"tier"         binding:"required"`
	BillingCycle string `json:"billing_cycle" binding:"required"`
}

type CreateCheckoutSessionResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

type TierLimitsResponse struct {
	BandwidthIngress                       int64  `json:"bandwidth_ingress"`
	BandwidthIngressHardCap                int64  `json:"bandwidth_ingress_hard_cap"`
	BandwidthIngressOverageCentsPerMillion int    `json:"bandwidth_ingress_overage_cents_per_million"`
	BandwidthEgress                        int64  `json:"bandwidth_egress"`
	BandwidthEgressOverageCentsPerGB       int    `json:"bandwidth_egress_overage_cents_per_gb"`
	TeamSeats                              int    `json:"team_seats"`
	SeatOverageCentsPerMonth               int    `json:"seat_overage_cents_per_month"`
	ContractLimit                          int    `json:"contract_limit"`
	RateLimit                              int    `json:"rate_limit"`
	MaxPayloadBytes                        int64  `json:"max_payload_bytes"`
	HotStorageDays                         int    `json:"hot_storage_days"`
	ColdStorageDays                        int    `json:"cold_storage_days"`
	ColdStorageOverageCentsPerGB           int    `json:"cold_storage_overage_cents_per_gb"`
	Support                                string `json:"support"`
	OverageAllowed                         bool   `json:"overage_allowed"`
}

type TierResponse struct {
	Name              string             `json:"name"`
	Limits            TierLimitsResponse `json:"limits"`
	MonthlyPriceID    string             `json:"monthly_price_id"`
	YearlyPriceID     string             `json:"yearly_price_id"`
	MonthlyPriceCents int                `json:"monthly_price_cents"`
	YearlyPriceCents  int                `json:"yearly_price_cents"`
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
	compID := companyID.(string)

	checkoutURL, err := h.billingService.CreateCheckoutSession(c.Request.Context(), compID, req.Tier, req.BillingCycle)
	if err != nil {
		h.logger.Error("failed to create checkout session",
			zap.String("company_id", compID),
			zap.String("tier", req.Tier),
			zap.Error(err),
		)
		if errors.Is(err, billing.ErrInvalidTier) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize checkout"})
		}
		return
	}

	h.logger.Info("checkout session created",
		zap.String("company_id", compID),
		zap.String("tier", req.Tier),
		zap.String("billing_cycle", req.BillingCycle),
	)

	c.JSON(http.StatusOK, CreateCheckoutSessionResponse{CheckoutURL: checkoutURL})
}

type PlanDTO struct {
	ID               string    `json:"id"`
	CompanyID        string    `json:"company_id"`
	SubscriptionTier string    `json:"subscription_tier"`
	BillingCycle     string    `json:"billing_cycle"`
	Status           string    `json:"status"`
	BillingStart     time.Time `json:"billing_start"`
	BillingEnd       time.Time `json:"billing_end"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type UsageMeterDTO struct {
	ID                      string    `json:"id"`
	CompanyID               string    `json:"company_id"`
	SubscriptionTier        string    `json:"subscription_tier"`
	BillingCycleStart       time.Time `json:"billing_cycle_start"`
	BillingEnd              time.Time `json:"billing_end"`
	BandwidthIngressBalance int64     `json:"bandwidth_ingress_balance"`
	BandwidthEgressBalance  int64     `json:"bandwidth_egress_balance"`
	MaxBandwidthIngress     int64     `json:"max_bandwidth_ingress"`
	MaxBandwidthEgress      int64     `json:"max_bandwidth_egress"`
	MaxTeamSeats            int       `json:"max_team_seats,omitempty"`
	MaxContractLimit        int       `json:"max_contract_limit,omitempty"`
	MaxRateLimit            int       `json:"max_rate_limit,omitempty"`
	MaxPayloadBytes         int64     `json:"max_payload_bytes,omitempty"`
	HotStorageDays          int       `json:"hot_storage_days,omitempty"`
	ColdStorageDays         int       `json:"cold_storage_days,omitempty"`
	Support                 string    `json:"support,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type GetCurrentPlanResponse struct {
	Plan       *PlanDTO       `json:"plan"`
	UsageMeter *UsageMeterDTO `json:"usage_meter"`
}

func mapPlanToDTO(p *billing.CompanyPlan) *PlanDTO {
	if p == nil {
		return nil
	}
	return &PlanDTO{
		ID:               p.ID,
		CompanyID:        p.CompanyID,
		SubscriptionTier: string(p.SubscriptionTier),
		BillingCycle:     string(p.BillingCycle),
		Status:           string(p.Status),
		BillingStart:     p.BillingStart,
		BillingEnd:       p.BillingEnd,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func mapMeterToDTO(m *billing.UsageMeter) *UsageMeterDTO {
	if m == nil {
		return nil
	}
	return &UsageMeterDTO{
		ID:                      m.ID,
		CompanyID:               m.CompanyID,
		SubscriptionTier:        string(m.SubscriptionTier),
		BillingCycleStart:       m.BillingCycleStart,
		BillingEnd:              m.BillingEnd,
		BandwidthIngressBalance: m.BandwidthIngressBalance,
		BandwidthEgressBalance:  m.BandwidthEgressBalance,
		MaxBandwidthIngress:     m.MaxBandwidthIngress,
		MaxBandwidthEgress:      m.MaxBandwidthEgress,
		MaxTeamSeats:            m.MaxTeamSeats,
		MaxContractLimit:        m.MaxContractLimit,
		MaxRateLimit:            m.MaxRateLimit,
		MaxPayloadBytes:         m.MaxPayloadBytes,
		HotStorageDays:          m.HotStorageDays,
		ColdStorageDays:         m.ColdStorageDays,
		Support:                 m.Support,
		CreatedAt:               m.CreatedAt,
		UpdatedAt:               m.UpdatedAt,
	}
}

func (h *BillingHandler) GetCurrentPlan(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	plan, meter, _, err := h.billingService.GetCurrentPlan(c.Request.Context(), compID)
	if err != nil {
		h.logger.Error("failed to get current plan info", zap.Error(err), zap.String("companyID", compID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve plan info"})
		return
	}

	c.JSON(http.StatusOK, GetCurrentPlanResponse{
		Plan:       mapPlanToDTO(plan),
		UsageMeter: mapMeterToDTO(meter),
	})
}

func (h *BillingHandler) GetTiers(c *gin.Context) {
	subConfig := h.billingService.GetTiersConfig()
	billingConfig := h.billingService.GetBillingConfig()

	if subConfig == nil || subConfig.Tiers == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tiers not configured"})
		return
	}
	if billingConfig == nil || billingConfig.TierPrices == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tier prices not configured"})
		return
	}

	var response []TierResponse

	for name, tier := range subConfig.Tiers {
		priceInfo := billingConfig.TierPrices[name]
		response = append(response, TierResponse{
			Name: name,
			Limits: TierLimitsResponse{
				BandwidthIngress:                       tier.BandwidthIngress,
				BandwidthIngressHardCap:                tier.BandwidthIngressHardCap,
				BandwidthIngressOverageCentsPerMillion: tier.BandwidthIngressOverageCentsPerMillion,
				BandwidthEgress:                        tier.BandwidthEgress,
				BandwidthEgressOverageCentsPerGB:       tier.BandwidthEgressOverageCentsPerGB,
				TeamSeats:                              tier.TeamSeats,
				SeatOverageCentsPerMonth:               tier.SeatOverageCentsPerMonth,
				ContractLimit:                          tier.ContractLimit,
				RateLimit:                              tier.RateLimit,
				MaxPayloadBytes:                        tier.MaxPayloadBytes,
				HotStorageDays:                         tier.HotStorageDays,
				ColdStorageDays:                        tier.ColdStorageDays,
				ColdStorageOverageCentsPerGB:           tier.ColdStorageOverageCentsPerGB,
				Support:                                tier.Support,
				OverageAllowed:                         tier.OverageAllowed,
			},
			MonthlyPriceID:    priceInfo.MonthlyPriceID,
			YearlyPriceID:     priceInfo.YearlyPriceID,
			MonthlyPriceCents: priceInfo.MonthlyPriceCents,
			YearlyPriceCents:  priceInfo.YearlyPriceCents,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tiers": response})
}

func (h *BillingHandler) CancelSubscription(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	if err := h.billingService.CancelSubscription(c.Request.Context(), compID); err != nil {
		h.logger.Error("failed to cancel subscription", zap.Error(err), zap.String("companyID", compID))
		if errors.Is(err, billing.ErrNoActiveSubscription) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel subscription"})
		}
		return
	}

	h.logger.Info("subscription cancelled", zap.String("company_id", compID))
	c.JSON(http.StatusOK, gin.H{"message": "subscription cancelled successfully"})
}
