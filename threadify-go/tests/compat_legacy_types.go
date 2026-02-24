package tests

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

type StartThreadRequest = models.StartThreadRequest
type JoinThreadRequest = models.JoinThreadRequest
type RecordEventRequest = models.RecordEventRequest

type StreamEvent struct {
	StreamID string
	Data     map[string]string
}

type EventBuffer struct {
	mu            sync.Mutex
	events        []StreamEvent
	maxSize       int
	flushInterval time.Duration
	lastFlush     time.Time
}

func NewEventBuffer(maxSize int, flushInterval time.Duration) *EventBuffer {
	return &EventBuffer{
		events:        make([]StreamEvent, 0, maxSize),
		maxSize:       maxSize,
		flushInterval: flushInterval,
		lastFlush:     time.Now(),
	}
}

func (b *EventBuffer) Add(event StreamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
}

func (b *EventBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func (b *EventBuffer) ShouldFlush() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) >= b.maxSize {
		return true
	}
	if len(b.events) == 0 {
		return false
	}
	return time.Since(b.lastFlush) >= b.flushInterval
}

func (b *EventBuffer) GetAndClear() []StreamEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]StreamEvent, len(b.events))
	copy(out, b.events)
	b.events = b.events[:0]
	b.lastFlush = time.Now()
	return out
}

type StreamReader interface {
	XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]StreamEvent, error)
}

type Consumer struct {
	stream     string
	group      string
	consumerID string
	reader     StreamReader
	buffer     *EventBuffer
	batchSize  int
	block      time.Duration
}

func NewConsumer(stream, group, consumerID string, reader StreamReader, buffer *EventBuffer, batchSize int, block time.Duration) *Consumer {
	return &Consumer{
		stream:     stream,
		group:      group,
		consumerID: consumerID,
		reader:     reader,
		buffer:     buffer,
		batchSize:  batchSize,
		block:      block,
	}
}

func (c *Consumer) Read(ctx context.Context) error {
	events, err := c.reader.XReadGroup(ctx, c.group, c.consumerID, c.stream, c.batchSize, c.block)
	if err != nil {
		return err
	}
	for _, event := range events {
		c.buffer.Add(event)
	}
	return nil
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_ = c.Read(ctx)
		}
	}
}

type Writer struct {
	valkey         *MockValkeyClient
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

func NewWriter(valkey *MockValkeyClient, maxAttempts int, initialBackoff, maxBackoff time.Duration) *Writer {
	return &Writer{
		valkey:         valkey,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
}

func (w *Writer) ProcessBatch(ctx context.Context, stream, group string, events []StreamEvent, writeFn func(context.Context, []StreamEvent) error) error {
	if len(events) == 0 {
		return nil
	}

	backoff := w.initialBackoff
	var lastErr error
	for attempt := 1; attempt <= w.maxAttempts; attempt++ {
		if err := writeFn(ctx, events); err == nil {
			ids := make([]string, 0, len(events))
			for _, event := range events {
				ids = append(ids, event.StreamID)
			}
			return w.valkey.XAck(ctx, stream, group, ids)
		} else {
			lastErr = err
		}

		if attempt < w.maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > w.maxBackoff {
				backoff = w.maxBackoff
			}
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

type StreamConfig struct {
	ConsumerGroup string
	BlockTimeout  time.Duration
	BatchSize     int
}

type BufferConfig struct {
	Size          int
	FlushInterval time.Duration
}

type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type ArchiverConfig struct {
	Streams StreamConfig
	Buffers map[string]BufferConfig
	Retry   RetryConfig
}

type queue struct {
	buffer   *EventBuffer
	consumer *Consumer
	writer   *Writer
	writeFn  func(context.Context, []StreamEvent) error
}

type Archiver struct {
	config ArchiverConfig
	reader StreamReader
	valkey *MockValkeyClient
	prefix string
	queues map[string]*queue
}

func NewArchiver(config ArchiverConfig, reader StreamReader, valkey *MockValkeyClient, prefix string) *Archiver {
	a := &Archiver{
		config: config,
		reader: reader,
		valkey: valkey,
		prefix: prefix,
		queues: make(map[string]*queue),
	}

	for name, bufferCfg := range config.Buffers {
		stream := "streams:" + name
		consumerID := prefix + "-" + name
		buffer := NewEventBuffer(bufferCfg.Size, bufferCfg.FlushInterval)
		a.queues[name] = &queue{
			buffer:   buffer,
			consumer: NewConsumer(stream, config.Streams.ConsumerGroup, consumerID, reader, buffer, config.Streams.BatchSize, config.Streams.BlockTimeout),
			writer:   NewWriter(valkey, config.Retry.MaxAttempts, config.Retry.InitialBackoff, config.Retry.MaxBackoff),
			writeFn: func(context.Context, []StreamEvent) error {
				return nil
			},
		}
	}

	return a
}

func (a *Archiver) RegisterQueue(name string, size int, flushInterval time.Duration, writeFn func(context.Context, []StreamEvent) error) {
	stream := "streams:" + name
	consumerID := a.prefix + "-" + name
	buffer := NewEventBuffer(size, flushInterval)
	a.queues[name] = &queue{
		buffer:   buffer,
		consumer: NewConsumer(stream, a.config.Streams.ConsumerGroup, consumerID, a.reader, buffer, a.config.Streams.BatchSize, a.config.Streams.BlockTimeout),
		writer:   NewWriter(a.valkey, a.config.Retry.MaxAttempts, a.config.Retry.InitialBackoff, a.config.Retry.MaxBackoff),
		writeFn:  writeFn,
	}
}

func (a *Archiver) Start(ctx context.Context) {
	for _, q := range a.queues {
		go q.consumer.Run(ctx)
	}
}

type Session struct {
	ownerID   string
	threadIDs []string
}

type WebSocketHandler struct {
	invitationService *service.InvitationTokenService
	threadService     interface{}
	stepEventService  interface{}
	auditService      interface{}
}

func (h *WebSocketHandler) handleInviteParty(session *Session, req *models.InvitePartyRequest) interface{} {
	if len(session.threadIDs) == 0 {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: "no active thread"}
	}

	allowed := []string{"external_partner", "supplier", "merchant", "payment_gateway", "viewer", "reader"}
	if !slices.Contains(allowed, req.Role) {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: "invalid role"}
	}

	expiry, err := h.invitationService.ParseExpiry(req.ExpiresIn)
	if err != nil {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: err.Error()}
	}

	accessLevel := strings.TrimSpace(req.AccessLevel)
	if accessLevel == "" {
		accessLevel = "external"
	}
	if err := h.invitationService.ValidateAccessLevel(accessLevel); err != nil {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: err.Error()}
	}

	token, err := h.invitationService.CreateToken(session.threadIDs[0], session.ownerID, req.Role, accessLevel, expiry)
	if err != nil {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: err.Error()}
	}

	return models.InvitePartyResponse{
		Action:      req.Action,
		Status:      "success",
		ThreadToken: token,
		Role:        req.Role,
		AccessLevel: accessLevel,
		ExpiresAt:   time.Now().Add(expiry).Unix(),
		Message:     "invitation created",
	}
}

func (h *WebSocketHandler) handleJoinThread(session *Session, req *models.JoinThreadRequest) interface{} {
	if req.ThreadToken == "" {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: "Invalid thread token"}
	}

	claims, err := h.invitationService.ValidateToken(req.ThreadToken)
	if err != nil {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: "Invalid thread token"}
	}

	if claims == nil {
		return models.ErrorResponse{Action: req.Action, Status: "error", Message: "Invalid thread token"}
	}

	if !slices.Contains(session.threadIDs, claims.ThreadID) {
		session.threadIDs = append(session.threadIDs, claims.ThreadID)
	}

	return models.JoinThreadResponse{
		Action:      req.Action,
		Status:      "success",
		ThreadID:    claims.ThreadID,
		Role:        claims.Role,
		AccessLevel: claims.AccessLevel,
		Message:     "joined thread",
	}
}
