package service

import (
	"context"
	"sync"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

// AccessWrite represents a pending access grant/update operation.
type AccessWrite struct {
	ThreadID    string
	UserID      string
	Role        string
	RuntimeRole string
	Permissions []string // Resolved permissions from runtime_role
	InvitedBy   string
	ResultChan  chan error // For synchronous error handling if needed
}

// AccessBatcher batches thread access writes to reduce Valkey load.
// Uses hybrid flushing: batch size OR timeout (whichever comes first).
type AccessBatcher struct {
	buffer        chan *AccessWrite
	batchSize     int
	flushInterval time.Duration
	accessRepo    interfaces.AccessRepository
	luaScripts    interfaces.LuaScriptManager
	stopChan      chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
	logger        *zap.Logger
}

// NewAccessBatcher creates a new access batcher.
func NewAccessBatcher(
	bufferSize int,
	batchSize int,
	flushInterval time.Duration,
	accessRepo interfaces.AccessRepository,
	luaScripts interfaces.LuaScriptManager,
	logger *zap.Logger,
) *AccessBatcher {
	return &AccessBatcher{
		buffer:        make(chan *AccessWrite, bufferSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		accessRepo:    accessRepo,
		luaScripts:    luaScripts,
		stopChan:      make(chan struct{}),
		logger:        logger,
	}
}

// Start begins the batching goroutine.
func (b *AccessBatcher) Start() {
	b.wg.Add(1)
	go b.run()
	b.logger.Info("access batcher started",
		zap.Int("batch_size", b.batchSize),
		zap.Duration("flush_interval", b.flushInterval),
	)
}

// Stop gracefully stops the batcher, drains the buffer, and flushes remaining items.
func (b *AccessBatcher) Stop() {
	b.stopOnce.Do(func() {
		close(b.stopChan)
	})
	b.wg.Wait()
	b.logger.Info("access batcher stopped")
}

// Write queues an access write operation (non-blocking).
// Falls back to a synchronous write if the buffer is full.
func (b *AccessBatcher) Write(write *AccessWrite) error {
	select {
	case <-b.stopChan:
		return context.Canceled
	default:
	}

	select {
	case b.buffer <- write:
		return nil
	case <-b.stopChan:
		return context.Canceled
	default:
		b.logger.Warn("access batcher buffer full, writing synchronously",
			zap.String("thread_id", write.ThreadID),
			zap.String("user_id", write.UserID),
		)
		return b.writeSync(write)
	}
}

// run is the main batching loop.
func (b *AccessBatcher) run() {
	defer b.wg.Done()

	batch := make([]*AccessWrite, 0, b.batchSize)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case write := <-b.buffer:
			batch = append(batch, write)
			if len(batch) >= b.batchSize {
				b.flush(batch)
				batch = make([]*AccessWrite, 0, b.batchSize)
				ticker.Reset(b.flushInterval)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				b.flush(batch)
				batch = make([]*AccessWrite, 0, b.batchSize)
			}

		case <-b.stopChan:
			// Drain any writes that arrived before the channel closed.
			for {
				select {
				case write := <-b.buffer:
					batch = append(batch, write)
				default:
					if len(batch) > 0 {
						b.flush(batch)
					}
					return
				}
			}
		}
	}
}

// flush writes a batch of access operations to Valkey.
func (b *AccessBatcher) flush(batch []*AccessWrite) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	successCount := 0
	errorCount := 0

	for _, write := range batch {
		_, err := b.accessRepo.GrantOrUpdateAccess(
			ctx,
			write.ThreadID,
			write.UserID,
			write.Role,
			write.RuntimeRole,
			write.Permissions,
			write.InvitedBy,
			b.luaScripts,
			nil, // threadData — not creating thread
			nil, // threadTTL — not creating thread
		)
		if err != nil {
			b.logger.Error("failed to grant access in batch",
				zap.String("thread_id", write.ThreadID),
				zap.String("user_id", write.UserID),
				zap.Error(err),
			)
			errorCount++
		} else {
			successCount++
		}
		sendResult(write.ResultChan, err)
	}

	b.logger.Debug("flushed batch",
		zap.Int("size", len(batch)),
		zap.Int("success", successCount),
		zap.Int("errors", errorCount),
		zap.Duration("duration", time.Since(start)),
	)
}

// writeSync writes an access operation synchronously (fallback when buffer is full).
func (b *AccessBatcher) writeSync(write *AccessWrite) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := b.accessRepo.GrantOrUpdateAccess(
		ctx,
		write.ThreadID,
		write.UserID,
		write.Role,
		write.RuntimeRole,
		write.Permissions,
		write.InvitedBy,
		b.luaScripts,
		nil, // threadData
		nil, // threadTTL
	)
	return err
}

// sendResult delivers err to the result channel if one was provided, non-blocking.
func sendResult(ch chan error, err error) {
	if ch == nil {
		return
	}
	select {
	case ch <- err:
	default:
	}
}
