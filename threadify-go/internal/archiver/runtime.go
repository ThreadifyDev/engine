package archiver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/config"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"go.uber.org/zap"
)

const defaultShutdownTimeout = 10 * time.Second

// Runtime owns all persistence consumers, but not their broker or database.
// Start's context bounds initialization. Call Close before closing dependencies;
// canceling the application's request context does not cancel pending DB writes.
type Runtime struct {
	js           JetStreamPublisher
	cfg          *config.Config
	logger       *zap.Logger
	batchSize    int
	streams      []*runtimeStream
	mu           sync.Mutex
	started      bool
	closing      bool
	stop         chan struct{}
	done         chan struct{}
	metadataDone chan struct{}
	shutdownCtx  context.Context
	cancelWrites context.CancelFunc
	shutdownErr  error
	wg           sync.WaitGroup
}

type runtimeStream struct {
	name, subject, durable string
	interval               time.Duration
	processor              func(context.Context, []jetstream.Msg) error
	iter                   jetstream.MessagesContext
	healthy                atomic.Bool
}

func NewRuntime(js JetStreamPublisher, db DBExecer, metrics MetricsInvalidator, cfg *config.Config, logger *zap.Logger) (*Runtime, error) {
	if js == nil || db == nil || cfg == nil {
		return nil, errors.New("archiver requires JetStream, PostgreSQL, and configuration")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	size := cfg.Archiver.Streams.BatchSize
	interval := cfg.Archiver.Streams.BlockTimeout
	stepInterval := cfg.Archiver.Streams.StepStateFlushInterval
	if size <= 0 {
		return nil, errors.New("archiver batch size must be positive")
	}
	if interval <= 0 {
		return nil, errors.New("archiver block timeout must be positive")
	}
	if stepInterval <= 0 {
		stepInterval = 5 * time.Second
	}
	general, err := NewNATSConsumer(js, db, metrics, size, interval, "archiver", cfg, logger)
	if err != nil {
		return nil, err
	}
	steps, err := NewStepStateConsumer(js, db, size, stepInterval, "step-state-archivers", logger)
	if err != nil {
		return nil, err
	}
	r := &Runtime{js: js, cfg: cfg, logger: logger, batchSize: size, stop: make(chan struct{}), done: make(chan struct{}), metadataDone: make(chan struct{})}
	for _, s := range []struct {
		name, subject string
		process       func(context.Context, []jetstream.Msg) error
	}{
		{"activity_log", "activity.log", general.processActivityLog},
		{"thread_metadata", "metadata.thread", general.processThreadMetadata},
		{"thread_access", "access.thread", general.processThreadAccess},
		{"thread_validations", "validations.thread", general.processThreadValidations},
		{"thread_notifications", "notifications.thread", general.processThreadNotifications},
		{"usage_sync", "usage.sync", general.processUsageSync},
	} {
		r.streams = append(r.streams, &runtimeStream{name: s.name, subject: s.subject, durable: "archiver-" + s.name, interval: interval, processor: s.process})
	}
	r.streams = append(r.streams, &runtimeStream{name: natsrepo.StreamStepState, subject: natsrepo.SubjectStepState, durable: "step-state-archivers", interval: stepInterval, processor: steps.processMessages})
	return r, nil
}

// Start establishes every durable consumer and iterator before any worker runs.
// A partial startup releases all established iterators and returns an error.
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.closing {
		return errors.New("archiver runtime already started or closed")
	}
	for _, s := range r.streams {
		consumer, err := r.js.CreateOrUpdateConsumer(ctx, s.name, jetstream.ConsumerConfig{
			Durable: s.durable, FilterSubject: s.subject, AckPolicy: jetstream.AckExplicitPolicy,
			MaxDeliver: r.cfg.NATS.ArchiverMaxDeliver,
			AckWait:    time.Duration(r.cfg.NATS.ArchiverAckWaitSeconds) * time.Second,
		})
		if err == nil {
			s.iter, err = consumer.Messages(jetstream.PullMaxMessages(r.batchSize))
		}
		if err != nil {
			for _, opened := range r.streams {
				if opened.iter != nil {
					opened.iter.Stop()
					opened.iter = nil
				}
			}
			return fmt.Errorf("start archiver consumer %s: %w", s.name, err)
		}
	}
	writeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancelWrites = cancel
	r.started = true
	for _, s := range r.streams {
		s.healthy.Store(true)
		r.wg.Add(1)
		go r.runStream(writeCtx, s)
	}
	go func() { r.wg.Wait(); cancel(); close(r.done) }()
	r.logger.Info("persistence consumers ready", zap.Int("streams", len(r.streams)))
	return nil
}

func (r *Runtime) IsHealthy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started || r.closing {
		return false
	}
	for _, s := range r.streams {
		if !s.healthy.Load() {
			return false
		}
	}
	return true
}

// Close stops pulls, flushes received batches using ctx, and awaits all readers
// and writers. A context without a deadline gets a ten-second shutdown bound.
// Failed writes remain unacknowledged or are negatively acknowledged for replay.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	if !r.closing {
		r.closing = true
		if !r.started {
			close(r.done)
		} else {
			shutdownCtx, cancel := context.WithCancel(ctx)
			if _, ok := ctx.Deadline(); !ok {
				cancel()
				shutdownCtx, cancel = context.WithTimeout(ctx, defaultShutdownTimeout)
			}
			r.shutdownCtx = shutdownCtx
			context.AfterFunc(shutdownCtx, r.cancelWrites)
			close(r.stop)
			// Stop unblocks Next. Readers never block indefinitely on their output queue.
			for _, s := range r.streams {
				s.iter.Stop()
			}
			go func() { <-r.done; cancel() }()
		}
	}
	done := r.done
	shutdownCtx := r.shutdownCtx
	r.mu.Unlock()
	if shutdownCtx == nil {
		return nil
	}
	select {
	case <-done:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	case <-shutdownCtx.Done():
		select {
		case <-done:
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.shutdownErr
		default:
			return shutdownCtx.Err()
		}
	}
}

func (r *Runtime) processingContext(normal context.Context) (context.Context, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return r.shutdownCtx, true
	}
	return normal, false
}

// Reference writes received before shutdown must follow the metadata batches
// that create their threads. This does not claim to drain unread broker backlog;
// unacknowledged deliveries remain durable for replay.
func (r *Runtime) waitForShutdownMetadata(ctx context.Context, s *runtimeStream) error {
	if s.name == "thread_metadata" || s.name == "usage_sync" {
		return ctx.Err()
	}
	select {
	case <-r.metadataDone:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) runStream(ctx context.Context, s *runtimeStream) {
	defer r.wg.Done()
	if s.name == "thread_metadata" {
		defer close(r.metadataDone)
	}
	defer s.healthy.Store(false)
	messages := make(chan jetstream.Msg, r.batchSize)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(messages)
		backoff := 100 * time.Millisecond
		for {
			msg, err := s.iter.Next()
			if err != nil {
				select {
				case <-r.stop:
					return
				default:
				}
				s.healthy.Store(false)
				if errors.Is(err, jetstream.ErrMsgIteratorClosed) {
					r.logger.Error("archiver iterator closed unexpectedly", zap.String("stream", s.name), zap.Error(err))
					return
				}
				r.logger.Error("archiver pull failed", zap.String("stream", s.name), zap.Error(err))
				StreamConsumptionErrors.WithLabelValues(s.name).Inc()
				timer := time.NewTimer(backoff)
				select {
				case <-r.stop:
					timer.Stop()
					return
				case <-timer.C:
				}
				backoff = min(2*backoff, 30*time.Second)
				continue
			}
			backoff = 100 * time.Millisecond
			select {
			case messages <- msg:
			case <-r.stop:
				return
			}
		}
	}()
	defer func() { s.iter.Stop(); <-readerDone }()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	batch := make([]jetstream.Msg, 0, r.batchSize)
	flush := func(reason string) {
		if len(batch) == 0 {
			return
		}
		writeCtx, closing := r.processingContext(ctx)
		if closing {
			reason = "shutdown"
		}
		BatchSize.WithLabelValues(s.name).Observe(float64(len(batch)))
		start := time.Now()
		var err error
		if closing {
			err = r.waitForShutdownMetadata(writeCtx, s)
		}
		if err == nil {
			err = s.processor(writeCtx, batch)
		}
		if !closing {
			var shutdownCtx context.Context
			shutdownCtx, closing = r.processingContext(ctx)
			// A normal in-flight batch can discover a missing thread just as
			// shutdown begins. Retry once after metadata completes instead of
			// treating the scheduling race as a permanent shutdown failure.
			if closing && errors.Is(err, ErrThreadNotFound) && s.name != "thread_metadata" {
				if waitErr := r.waitForShutdownMetadata(shutdownCtx, s); waitErr != nil {
					err = waitErr
				} else {
					err = s.processor(shutdownCtx, batch)
				}
			}
		}
		BatchProcessingDuration.WithLabelValues(s.name).Observe(time.Since(start).Seconds())
		s.healthy.Store(err == nil)
		if err != nil {
			r.logger.Error("archiver batch failed", zap.String("stream", s.name), zap.Error(err))
			BatchesProcessed.WithLabelValues(s.name, "error").Inc()
			if closing {
				r.mu.Lock()
				r.shutdownErr = errors.Join(r.shutdownErr, fmt.Errorf("flush %s: %w", s.name, err))
				r.mu.Unlock()
			}
			for _, msg := range batch {
				if errors.Is(err, ErrThreadNotFound) || s.name == natsrepo.StreamStepState {
					_ = msg.NakWithDelay(5 * time.Second)
				} else {
					_ = msg.Nak()
				}
				RetryAttempts.WithLabelValues(s.name).Inc()
			}
		} else {
			BatchesProcessed.WithLabelValues(s.name, "success").Inc()
			for _, msg := range batch {
				if err := msg.Ack(); err != nil {
					r.logger.Warn("archiver acknowledgement failed", zap.String("stream", s.name), zap.Error(err))
				}
			}
		}
		batch = batch[:0]
		BufferFlushes.WithLabelValues(s.name, reason).Inc()
	}
	appendMessage := func(msg jetstream.Msg) {
		StreamMessagesConsumed.WithLabelValues(s.name).Inc()
		batch = append(batch, msg)
		if len(batch) >= r.batchSize {
			flush("size")
		}
	}
	for {
		select {
		case <-r.stop:
			for msg := range messages {
				appendMessage(msg)
			}
			flush("shutdown")
			return
		case msg, ok := <-messages:
			if !ok {
				flush("shutdown")
				return
			}
			appendMessage(msg)
		case <-ticker.C:
			flush("time")
		}
	}
}
