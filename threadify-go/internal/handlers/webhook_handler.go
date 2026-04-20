package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	billingmodels "threadify-go/shared/models"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

const (
	maxWebhookBodyBytes = 65536

	eventInvoicePaid          = "invoice.paid"
	eventInvoicePaymentFailed = "invoice.payment_failed"
	eventCheckoutCompleted    = "checkout.session.completed"

	maxPaymentAttempts = int64(4)
)

type WebhookHandler struct {
	provider   interfaces.WebhookProvider
	billingSvc interfaces.BillingWebhookService
	logger     *zap.Logger
}

func NewWebhookHandler(
	provider interfaces.WebhookProvider,
	billingSvc interfaces.BillingWebhookService,
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

	if !json.Valid(body) {
		h.logger.Warn("webhook: malformed json payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
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
		h.logger.Debug("webhook: verified but no event returned",
			zap.String("provider", h.provider.Name()),
		)
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	ctx := c.Request.Context()
	var handlerErr error

	switch event.Type {
	case eventInvoicePaid:
		handlerErr = h.handleInvoicePaid(ctx, event)
	case eventInvoicePaymentFailed:
		handlerErr = h.handleInvoicePaymentFailed(ctx, event)
	case eventCheckoutCompleted:
		handlerErr = h.handleCheckoutSessionCompleted(ctx, event)
	default:
		h.logger.Debug("webhook: unhandled event type",
			zap.String("provider", h.provider.Name()),
			zap.String("type", event.Type),
		)
	}

	if handlerErr != nil {
		h.logger.Error("webhook: processing failed",
			zap.String("type", event.Type),
			zap.Error(handlerErr),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal processing error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *WebhookHandler) handleInvoicePaid(ctx context.Context, event *billingmodels.WebhookEvent) error {
	if event.ExternalInvoiceID == "" {
		h.logger.Warn("webhook: invoice.paid missing invoice ID")
		return nil
	}

	snapshot, err := h.billingSvc.FindSnapshotByInvoiceID(ctx, event.ExternalInvoiceID)
	if err != nil {
		return fmt.Errorf("find snapshot by invoice: %w", err)
	}

	if snapshot == nil {
		h.logger.Warn("webhook: invoice.paid has no matching snapshot, skipping",
			zap.String("invoice_id", event.ExternalInvoiceID),
		)
		return nil
	}

	if snapshot.ExternalInvoiceID == "" {
		h.logger.Info("webhook: linking orphan snapshot to paid invoice",
			zap.String("snapshot_id", snapshot.ID),
			zap.String("invoice_id", event.ExternalInvoiceID),
		)
		if err := h.billingSvc.LinkAndMarkSnapshotPaid(ctx, snapshot.ID, event.ExternalInvoiceID); err != nil {
			return fmt.Errorf("link and mark snapshot paid: %w", err)
		}
	} else {
		if err := h.billingSvc.MarkSnapshotPaid(ctx, event.ExternalInvoiceID); err != nil {
			return fmt.Errorf("mark snapshot paid: %w", err)
		}
	}

	if strings.HasPrefix(string(snapshot.Reason), string(billingmodels.SnapshotReasonCreditTopup)) {
		if err := h.billingSvc.ApplyCreditTopup(ctx, snapshot); err != nil {
			return fmt.Errorf("apply credit topup: %w", err)
		}
	}

	h.logger.Info("webhook: invoice paid processed",
		zap.String("provider", h.provider.Name()),
		zap.String("invoice_id", event.ExternalInvoiceID),
		zap.String("company_id", snapshot.CompanyID),
	)

	return nil
}

func (h *WebhookHandler) handleInvoicePaymentFailed(ctx context.Context, event *billingmodels.WebhookEvent) error {
	if event.ExternalInvoiceID == "" {
		h.logger.Warn("webhook: invoice.payment_failed missing invoice ID")
		return nil
	}

	if err := h.billingSvc.MarkSnapshotFailed(ctx, event.ExternalInvoiceID); err != nil {
		return fmt.Errorf("mark snapshot failed: %w", err)
	}

	snapshot, err := h.billingSvc.FindSnapshotByInvoiceID(ctx, event.ExternalInvoiceID)
	if err != nil {
		return fmt.Errorf("find snapshot for failed invoice: %w", err)
	}
	if snapshot == nil {
		// No snapshot -> fail-safe: clear pending to avoid permanent block on auto-topup
		companyID := event.Metadata["company_id"]
		if companyID != "" {
			if clearErr := h.billingSvc.ClearCreditTopupPending(ctx, companyID); clearErr != nil {
				h.logger.Error("webhook: failed to clear credit topup pending for missing snapshot (fail-open)",
					zap.String("company_id", companyID),
					zap.String("invoice_id", event.ExternalInvoiceID),
					zap.Error(clearErr),
				)
			}
		}
		h.logger.Warn("webhook: invoice.payment_failed has no matching snapshot",
			zap.String("invoice_id", event.ExternalInvoiceID),
		)
		return nil
	}

	companyID := snapshot.CompanyID

	if clearErr := h.billingSvc.ClearCreditTopupPending(ctx, companyID); clearErr != nil {
		h.logger.Error("webhook: failed to clear credit topup pending (fail-open)",
			zap.String("company_id", companyID),
			zap.String("invoice_id", event.ExternalInvoiceID),
			zap.Error(clearErr),
		)
	}

	if event.AttemptCount >= maxPaymentAttempts {
		if err := h.billingSvc.UpdateMaxMonthlyCharge(ctx, companyID, billingmodels.CreditDisabled); err != nil {
			h.logger.Error("webhook: failed to disable auto-topup after final failure", zap.Error(err), zap.String("company_id", companyID))
		}
		h.logger.Error("webhook: final payment attempt failed — auto-topup disabled",
			zap.String("provider", h.provider.Name()),
			zap.String("company_id", companyID),
			zap.String("invoice_id", event.ExternalInvoiceID),
			zap.Int64("attempt_count", event.AttemptCount),
		)
	} else {
		h.logger.Warn("webhook: payment failed, will retry",
			zap.String("provider", h.provider.Name()),
			zap.String("company_id", companyID),
			zap.String("invoice_id", event.ExternalInvoiceID),
			zap.Int64("attempt_count", event.AttemptCount),
			zap.Int64("max_attempts", maxPaymentAttempts),
		)
	}

	return nil
}

func (h *WebhookHandler) handleCheckoutSessionCompleted(ctx context.Context, event *billingmodels.WebhookEvent) error {
	companyID := event.Metadata["company_id"]
	if companyID == "" {
		h.logger.Warn("webhook: checkout.session.completed missing company_id in metadata")
		return nil
	}

	if event.ExternalCustomerID == "" {
		h.logger.Warn("webhook: checkout.session.completed missing customer ID",
			zap.String("company_id", companyID),
		)
		return nil
	}

	if event.AmountMillicents <= 0 {
		h.logger.Warn("webhook: checkout.session.completed has no usable amount",
			zap.String("company_id", companyID),
		)
		return nil
	}

	h.logger.Info("webhook: provisioning from checkout",
		zap.String("company_id", companyID),
		zap.String("customer_id", event.ExternalCustomerID),
		zap.Int64("initial_amount_millicents", event.AmountMillicents),
	)

	if err := h.billingSvc.ProvisionSubscription(
		ctx,
		companyID,
		event.ExternalCustomerID,
		event.AmountMillicents,
	); err != nil {
		h.logger.Error("webhook: failed to provision from checkout",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return err
	}

	return nil
}
