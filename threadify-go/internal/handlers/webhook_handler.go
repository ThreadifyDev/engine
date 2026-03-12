package handlers

import (
	"context"
	"io"
	"net/http"

	"threadify-go/shared/billing"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

const (
	maxWebhookBodyBytes = 65536

	eventInvoicePaid              = "invoice.paid"
	eventInvoicePaymentFailed     = "invoice.payment_failed"
	eventSubscriptionDeleted      = "subscription.deleted"
	eventCheckoutSessionCompleted = "checkout.session.completed"
)

type BillingWebhookService interface {
	MarkSnapshotPaid(ctx context.Context, externalInvoiceID string) error
	MarkSnapshotFailed(ctx context.Context, externalInvoiceID string) error
	FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*billing.BillingSnapshot, error)
	RenewAndReset(ctx context.Context, companyID string, snapshot *billing.BillingSnapshot) error
	LiftSuspension(ctx context.Context, companyID string) error
	SuspendCompany(ctx context.Context, companyID string) error
	CancelPlan(ctx context.Context, companyID string) error
	GetCompanyIDByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error)
	ProvisionSubscription(ctx context.Context, companyID string, tier billing.PlanTier, billingCycle billing.BillingCycle, externalCustomerID, externalSubscriptionID string) error
}

type WebhookHandler struct {
	provider   interfaces.WebhookProvider
	billingSvc BillingWebhookService
	logger     *zap.Logger
}

func NewWebhookHandler(
	provider interfaces.WebhookProvider,
	billingSvc BillingWebhookService,
	logger *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		provider:   provider,
		billingSvc: billingSvc,
		logger:     logger,
	}
}

func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodyBytes))
	if err != nil {
		h.logger.Error("webhook: failed to read body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	signature := c.GetHeader(h.provider.SignatureHeader())

	event, err := h.provider.VerifyAndParse(body, signature)
	if err != nil {
		h.logger.Warn("webhook: verification failed",
			zap.String("provider", h.provider.Name()),
			zap.Error(err),
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	if event == nil {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	ctx := c.Request.Context()

	switch event.Type {
	case eventInvoicePaid:
		h.handleInvoicePaid(ctx, event)
	case eventInvoicePaymentFailed:
		h.handleInvoicePaymentFailed(ctx, event)
	case eventSubscriptionDeleted:
		h.handleSubscriptionDeleted(ctx, event)
	case eventCheckoutSessionCompleted:
		h.handleCheckoutSessionCompleted(ctx, event)
	default:
		h.logger.Debug("webhook: unhandled event type",
			zap.String("provider", h.provider.Name()),
			zap.String("type", event.Type),
		)
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *WebhookHandler) handleInvoicePaid(ctx context.Context, event *billing.WebhookEvent) {
	if event.ExternalInvoiceID == "" {
		h.logger.Warn("webhook: invoice.paid missing invoice ID")
		return
	}

	if err := h.billingSvc.MarkSnapshotPaid(ctx, event.ExternalInvoiceID); err != nil {
		h.logger.Error("webhook: failed to mark snapshot paid",
			zap.String("invoice_id", event.ExternalInvoiceID),
			zap.Error(err),
		)
		return
	}

	snapshot, err := h.billingSvc.FindSnapshotByInvoiceID(ctx, event.ExternalInvoiceID)
	if err != nil {
		h.logger.Error("webhook: failed to find snapshot",
			zap.String("invoice_id", event.ExternalInvoiceID),
			zap.Error(err),
		)
		return
	}

	if snapshot == nil {
		h.logger.Debug("webhook: invoice.paid has no matching snapshot, skipping",
			zap.String("invoice_id", event.ExternalInvoiceID),
		)
		return
	}

	if err := h.billingSvc.LiftSuspension(ctx, snapshot.CompanyID); err != nil {
		h.logger.Error("webhook: failed to lift suspension",
			zap.String("company_id", snapshot.CompanyID),
			zap.Error(err),
		)
		return
	}

	if snapshot.IsCycleEnd {
		if err := h.billingSvc.RenewAndReset(ctx, snapshot.CompanyID, snapshot); err != nil {
			h.logger.Error("webhook: failed to renew and reset after cycle-end payment",
				zap.String("company_id", snapshot.CompanyID),
				zap.String("snapshot_id", snapshot.ID),
				zap.Error(err),
			)
			return
		}
		h.logger.Info("webhook: cycle renewed after payment",
			zap.String("company_id", snapshot.CompanyID),
			zap.String("snapshot_id", snapshot.ID),
		)
	}

	h.logger.Info("webhook: invoice paid processed",
		zap.String("provider", h.provider.Name()),
		zap.String("invoice_id", event.ExternalInvoiceID),
		zap.String("company_id", snapshot.CompanyID),
		zap.Bool("cycle_end", snapshot.IsCycleEnd),
	)
}

func (h *WebhookHandler) handleInvoicePaymentFailed(ctx context.Context, event *billing.WebhookEvent) {
	if event.ExternalInvoiceID == "" {
		h.logger.Warn("webhook: invoice.payment_failed missing invoice ID")
		return
	}

	if err := h.billingSvc.MarkSnapshotFailed(ctx, event.ExternalInvoiceID); err != nil {
		h.logger.Error("webhook: failed to mark snapshot failed",
			zap.String("invoice_id", event.ExternalInvoiceID),
			zap.Error(err),
		)
		return
	}

	if event.ExternalCustomerID == "" {
		h.logger.Warn("webhook: invoice.payment_failed missing customer ID, cannot suspend",
			zap.String("invoice_id", event.ExternalInvoiceID),
		)
		return
	}

	companyID, err := h.billingSvc.GetCompanyIDByExternalCustomerID(ctx, event.ExternalCustomerID)
	if err != nil {
		h.logger.Error("webhook: failed to resolve company from customer",
			zap.String("customer_id", event.ExternalCustomerID),
			zap.Error(err),
		)
		return
	}

	if companyID == "" {
		h.logger.Warn("webhook: no company found for customer",
			zap.String("customer_id", event.ExternalCustomerID),
		)
		return
	}

	if err := h.billingSvc.SuspendCompany(ctx, companyID); err != nil {
		h.logger.Error("webhook: failed to suspend company",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return
	}

	h.logger.Warn("webhook: payment failed — company suspended",
		zap.String("provider", h.provider.Name()),
		zap.String("company_id", companyID),
		zap.String("invoice_id", event.ExternalInvoiceID),
		zap.Int64("attempt_count", event.AttemptCount),
	)
}

func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event *billing.WebhookEvent) {
	if event.ExternalCustomerID == "" {
		h.logger.Warn("webhook: subscription.deleted missing customer ID")
		return
	}

	companyID, err := h.billingSvc.GetCompanyIDByExternalCustomerID(ctx, event.ExternalCustomerID)
	if err != nil {
		h.logger.Error("webhook: failed to resolve company from customer",
			zap.String("customer_id", event.ExternalCustomerID),
			zap.Error(err),
		)
		return
	}

	if companyID == "" {
		h.logger.Warn("webhook: no company found for customer",
			zap.String("customer_id", event.ExternalCustomerID),
		)
		return
	}

	if err := h.billingSvc.CancelPlan(ctx, companyID); err != nil {
		h.logger.Error("webhook: failed to cancel plan",
			zap.String("company_id", companyID),
			zap.String("subscription_id", event.ExternalSubscriptionID),
			zap.Error(err),
		)
		return
	}

	h.logger.Warn("webhook: subscription deleted — plan cancelled",
		zap.String("provider", h.provider.Name()),
		zap.String("company_id", companyID),
		zap.String("subscription_id", event.ExternalSubscriptionID),
	)
}

func (h *WebhookHandler) handleCheckoutSessionCompleted(ctx context.Context, event *billing.WebhookEvent) {
	if event.CompanyID == "" || event.Tier == "" {
		h.logger.Error("webhook: checkout.session.completed missing required metadata",
			zap.String("customer_id", event.ExternalCustomerID),
		)
		return
	}

	cycle := billing.BillingCycleMonthly
	if event.BillingCycle == string(billing.BillingCycleYearly) {
		cycle = billing.BillingCycleYearly
	}

	tier := billing.PlanTier(event.Tier)

	err := h.billingSvc.ProvisionSubscription(
		ctx,
		event.CompanyID,
		tier,
		cycle,
		event.ExternalCustomerID,
		event.ExternalSubscriptionID,
	)

	if err != nil {
		h.logger.Error("webhook: failed to provision subscription from checkout",
			zap.String("company_id", event.CompanyID),
			zap.String("customer_id", event.ExternalCustomerID),
			zap.Error(err),
		)
		return
	}

	h.logger.Info("webhook: initial subscription provisioned from checkout",
		zap.String("provider", h.provider.Name()),
		zap.String("company_id", event.CompanyID),
		zap.String("tier", string(tier)),
		zap.String("billing_cycle", string(cycle)),
	)
}
