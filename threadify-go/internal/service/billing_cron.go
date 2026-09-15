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
	"go.uber.org/zap"
)

const (
	creditTopupHandleTimeout   = 30 * time.Second
	creditTopupConsumerBackoff = 5 * time.Second
)

type billingCronService interface {
	ChargeCreditTopup(context.Context, string, string, time.Time, int64) error
	ProcessRollovers(context.Context) error
}

type BillingCron struct {
	billingService billingCronService
	logger         *zap.Logger
	js             jetstream.JetStream
	stopChan       chan struct{}
	mu             sync.Mutex
	started        bool
	closing        bool
	iter           jetstream.MessagesContext
	cancel         context.CancelFunc
	done           chan struct{}
	wg             sync.WaitGroup
}

func NewBillingCron(
	billingService *BillingOrchestrator,
	js jetstream.JetStream,
	logger *zap.Logger,
) *BillingCron {
	return &BillingCron{
		billingService: billingService,
		js:             js,
		logger:         logger,
		stopChan:       make(chan struct{}),
		done:           make(chan struct{}),
	}
}

// Start establishes the top-up consumer before reporting readiness.
func (c *BillingCron) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started || c.closing {
		return fmt.Errorf("billing cron already started or stopped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	if c.js != nil {
		iter, err := c.openCreditTopupIterator(ctx)
		if err != nil {
			cancel()
			return err
		}
		c.iter = iter
	}
	c.started = true
	if c.iter != nil {
		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.startCreditTopupConsumer(ctx) }()
	}
	c.wg.Add(1)
	go func() { defer c.wg.Done(); c.startRolloverWorker(ctx) }()
	go func() { c.wg.Wait(); cancel(); close(c.done) }()
	c.logger.Info("billing consumers started")
	return nil
}

func (c *BillingCron) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.StopContext(ctx)
}

// StopContext stops new pulls and lets accepted billing work finish within ctx.
// If the budget expires, cancellation reaches outstanding provider/DB requests.
func (c *BillingCron) StopContext(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	c.mu.Lock()
	if !c.closing {
		c.closing = true
		close(c.stopChan)
		if c.iter != nil {
			c.iter.Stop()
		}
		if !c.started {
			close(c.done)
		}
	}
	cancelWork := c.cancel
	c.mu.Unlock()
	var stopCancellation func() bool
	if cancelWork != nil {
		stopCancellation = context.AfterFunc(ctx, cancelWork)
		defer stopCancellation()
	}
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		if cancelWork != nil {
			cancelWork()
		}
		return ctx.Err()
	}
}

func (c *BillingCron) openCreditTopupIterator(ctx context.Context) (jetstream.MessagesContext, error) {
	setupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	consumer, err := c.js.CreateOrUpdateConsumer(setupCtx, "usage_sync", jetstream.ConsumerConfig{
		Durable: "billing-credit-topup", FilterSubject: "credit.topup", AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("create credit topup consumer: %w", err)
	}
	iter, err := consumer.Messages(jetstream.PullMaxMessages(1))
	if err != nil {
		return nil, fmt.Errorf("get credit topup messages: %w", err)
	}
	return iter, nil
}

func (c *BillingCron) startCreditTopupConsumer(ctx context.Context) {
	for {
		c.mu.Lock()
		iter := c.iter
		closing := c.closing
		c.mu.Unlock()
		if closing {
			return
		}
		if err := c.runCreditTopupConsumer(ctx, iter); err != nil {
			c.logger.Error("credit topup consumer exited, will reconnect", zap.Error(err))
		}
		iter.Stop()
		for {
			timer := time.NewTimer(creditTopupConsumerBackoff)
			select {
			case <-c.stopChan:
				timer.Stop()
				return
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			next, err := c.openCreditTopupIterator(ctx)
			if err != nil {
				c.logger.Error("reconnect credit topup consumer", zap.Error(err))
				continue
			}
			c.mu.Lock()
			if c.closing {
				c.mu.Unlock()
				next.Stop()
				return
			}
			c.iter = next
			c.mu.Unlock()
			break
		}
	}
}

func (c *BillingCron) runCreditTopupConsumer(ctx context.Context, iter jetstream.MessagesContext) error {
	for {
		select {
		case <-c.stopChan:
			return nil
		default:
		}
		msg, err := iter.Next()
		if err != nil {
			select {
			case <-c.stopChan:
				return nil
			default:
			}
			return fmt.Errorf("credit topup iterator: %w", err)
		}
		// Stop can race with Next delivering a message. Leave it pending for replay.
		select {
		case <-c.stopChan:
			return nil
		default:
		}
		if err := c.handleCreditTopupContext(ctx, msg); err != nil {
			var permErr *permanentError
			if errors.As(err, &permErr) {
				c.logger.Error("permanent credit topup failure, terminating message", zap.Error(err), zap.Binary("payload", msg.Data()))
				_ = msg.Term()
			} else {
				c.logger.Error("transient credit topup failure, will retry", zap.Error(err))
				_ = msg.Nak()
			}
		} else {
			_ = msg.Ack()
		}
	}
}

type permanentError struct {
	cause error
}

func (e *permanentError) Error() string { return e.cause.Error() }
func (e *permanentError) Unwrap() error { return e.cause }

func permanent(err error) error { return &permanentError{cause: err} }

// PermanentError is an exported alias for permanentError (primarily for tests).
type PermanentError = permanentError

type creditTopupEvent struct {
	CompanyID         string      `json:"company_id"`
	EventID           string      `json:"event_id"`
	BillingCycleStart string      `json:"billing_cycle_start"`
	Amount            json.Number `json:"amount"`
}

func (c *BillingCron) handleCreditTopup(msg jetstream.Msg) error {
	return c.handleCreditTopupContext(context.Background(), msg)
}

func (c *BillingCron) handleCreditTopupContext(parent context.Context, msg jetstream.Msg) error {
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

	ctx, cancel := context.WithTimeout(parent, creditTopupHandleTimeout)
	defer cancel()

	return c.billingService.ChargeCreditTopup(ctx, event.CompanyID, event.EventID, billingCycleStart, amountMillicents)
}

// HandleCreditTopup is an exported wrapper around handleCreditTopup (primarily for tests).
func (c *BillingCron) HandleCreditTopup(msg jetstream.Msg) error {
	return c.handleCreditTopup(msg)
}

func (c *BillingCron) startRolloverWorker(ctx context.Context) {
	c.logger.Info("billing rollover worker started")

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	select {
	case <-c.stopChan:
		return
	default:
	}
	c.runRolloverContext(ctx)

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.runRolloverContext(ctx)
		}
	}
}

func (c *BillingCron) runRollover() {
	c.runRolloverContext(context.Background())
}

func (c *BillingCron) runRolloverContext(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	err := c.billingService.ProcessRollovers(ctx)
	cancel()
	if err != nil {
		c.logger.Error("failed to process billing rollovers", zap.Error(err))
	}
}

// RunRollover is an exported wrapper around runRollover (primarily for tests).
func (c *BillingCron) RunRollover() {
	c.runRollover()
}
