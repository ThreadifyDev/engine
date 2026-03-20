package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

const (
	creditTopupHandleTimeout   = 30 * time.Second
	creditTopupConsumerBackoff = 5 * time.Second
)

type BillingCron struct {
	billingService *BillingService
	valkeyClient   interfaces.ValkeyClient
	logger         *zap.Logger
	js             jetstream.JetStream
	stopChan       chan struct{}
	stopOnce       sync.Once
}

func NewBillingCron(
	billingService *BillingService,
	valkeyClient interfaces.ValkeyClient,
	js jetstream.JetStream,
	logger *zap.Logger,
) *BillingCron {
	return &BillingCron{
		billingService: billingService,
		valkeyClient:   valkeyClient,
		js:             js,
		logger:         logger,
		stopChan:       make(chan struct{}),
	}
}

func (c *BillingCron) Start() error {
	c.logger.Info("billing consumers starting")

	if c.js != nil {
		go c.startCreditTopupConsumer()
	}
	go c.startRolloverWorker()

	return nil
}

func (c *BillingCron) Stop() error {
	c.stopOnce.Do(func() { close(c.stopChan) })
	return nil
}

func (c *BillingCron) startCreditTopupConsumer() {
	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		if err := c.runCreditTopupConsumer(); err != nil {
			c.logger.Error("credit topup consumer exited with error, will reconnect",
				zap.Error(err),
				zap.Duration("backoff", creditTopupConsumerBackoff),
			)
			select {
			case <-c.stopChan:
				return
			case <-time.After(creditTopupConsumerBackoff):
			}
		}

	}
}

func (c *BillingCron) runCreditTopupConsumer() error {
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	consumer, err := c.js.CreateOrUpdateConsumer(setupCtx, "usage_sync", jetstream.ConsumerConfig{
		Durable:       "billing-credit-topup",
		FilterSubject: "credit.topup",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	setupCancel()
	if err != nil {
		return fmt.Errorf("create credit topup consumer: %w", err)
	}

	iter, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("get credit topup messages: %w", err)
	}
	defer iter.Stop()

	c.logger.Info("credit topup consumer started")

	for {
		msg, err := iter.Next()
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgIteratorClosed) {
				select {
				case <-c.stopChan:
					c.logger.Debug("credit topup iterator closed during shutdown")
					return nil
				default:
					return fmt.Errorf("credit topup iterator closed unexpectedly")
				}
			}
			select {
			case <-c.stopChan:
				return nil
			default:
				return fmt.Errorf("unexpected iterator error: %w", err)
			}
		}

		if err := c.handleCreditTopup(msg); err != nil {
			var permErr *permanentError
			if errors.As(err, &permErr) {
				c.logger.Error("permanent credit topup failure, terminating message",
					zap.Error(err),
					zap.Binary("payload", msg.Data()),
				)
				msg.Term()
			} else {
				c.logger.Error("transient credit topup failure, will retry", zap.Error(err))
				msg.Nak()
			}
		} else {
			msg.Ack()
		}
	}
}

type permanentError struct {
	cause error
}

func (e *permanentError) Error() string { return e.cause.Error() }
func (e *permanentError) Unwrap() error { return e.cause }

func permanent(err error) error { return &permanentError{cause: err} }

type creditTopupEvent struct {
	CompanyID         string      `json:"company_id"`
	EventID           string      `json:"event_id"`
	BillingCycleStart string      `json:"billing_cycle_start"`
	Amount            json.Number `json:"amount"`
}

func (c *BillingCron) handleCreditTopup(msg jetstream.Msg) error {
	dec := json.NewDecoder(bytes.NewReader(msg.Data()))
	dec.UseNumber()
	var event creditTopupEvent
	if err := dec.Decode(&event); err != nil {
		return permanent(fmt.Errorf("unmarshal credit topup: %w", err))
	}

	if event.CompanyID == "" || event.BillingCycleStart == "" {
		return permanent(fmt.Errorf("invalid credit topup event: company_id=%q billing_cycle_start=%q",
			event.CompanyID, event.BillingCycleStart))
	}

	amountMillicents, err := event.Amount.Int64()
	if err != nil {
		return permanent(fmt.Errorf("parse credit amount %q: %w", event.Amount, err))
	}

	if amountMillicents <= 0 {
		c.logger.Warn("credit topup event has non-positive amount, terminating",
			zap.String("company_id", event.CompanyID),
			zap.Int64("amount_millicents", amountMillicents),
			zap.String("event_id", event.EventID),
		)
		return permanent(fmt.Errorf("invalid credit topup amount: %d millicents (company: %s, event: %s)",
			amountMillicents, event.CompanyID, event.EventID))
	}

	billingCycleStart, err := time.Parse(time.RFC3339Nano, event.BillingCycleStart)
	if err != nil {
		return permanent(fmt.Errorf("invalid billing_cycle_start %q: %w", event.BillingCycleStart, err))
	}

	c.logger.Info("processing durable credit topup trigger",
		zap.String("company_id", event.CompanyID),
		zap.Int64("amount_millicents", amountMillicents),
		zap.String("event_id", event.EventID),
	)

	ctx, cancel := context.WithTimeout(context.Background(), creditTopupHandleTimeout)
	defer cancel()

	return c.billingService.ChargeCreditTopup(ctx, event.CompanyID, event.EventID, billingCycleStart, amountMillicents)
}

func (c *BillingCron) startRolloverWorker() {
	c.logger.Info("billing rollover worker started")

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	c.runRollover()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.runRollover()
		}
	}
}

func (c *BillingCron) runRollover() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	err := c.billingService.ProcessRollovers(ctx)
	cancel()
	if err != nil {
		c.logger.Error("failed to process billing rollovers", zap.Error(err))
	}
}
