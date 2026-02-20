package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/threadify/engine/internal/repository/valkey"
)

// AccessWrite represents a pending access grant/update operation
type AccessWrite struct {
	ThreadID    string
	UserID      string
	Role        string
	RuntimeRole string
	Permissions []string // Resolved permissions from runtime_role
	InvitedBy   string
	ResultChan  chan error // For synchronous error handling if needed
}

// AccessBatcher batches thread access writes to reduce Valkey load
// Uses hybrid flushing: batch size OR timeout (whichever comes first)
type AccessBatcher struct {
	buffer        chan *AccessWrite
	batchSize     int
	flushInterval time.Duration
	accessRepo    *valkey.AccessRepository
	luaScripts    *valkey.LuaScriptManager
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewAccessBatcher creates a new access batcher
func NewAccessBatcher(
	bufferSize int,
	batchSize int,
	flushInterval time.Duration,
	accessRepo *valkey.AccessRepository,
	luaScripts *valkey.LuaScriptManager,
) *AccessBatcher {
	return &AccessBatcher{
		buffer:        make(chan *AccessWrite, bufferSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		accessRepo:    accessRepo,
		luaScripts:    luaScripts,
		stopChan:      make(chan struct{}),
	}
}

// Start begins the batching goroutine
func (b *AccessBatcher) Start() {
	b.wg.Add(1)
	go b.run()
	log.Printf("[ACCESS-BATCHER] Started (batch_size=%d, flush_interval=%v)", b.batchSize, b.flushInterval)
}

// Stop gracefully stops the batcher and flushes remaining items
func (b *AccessBatcher) Stop() {
	close(b.stopChan)
	b.wg.Wait()
	log.Printf("[ACCESS-BATCHER] Stopped")
}

// Write queues an access write operation (non-blocking)
func (b *AccessBatcher) Write(write *AccessWrite) error {
	select {
	case b.buffer <- write:
		return nil
	case <-b.stopChan:
		return context.Canceled
	default:
		// Buffer full - write synchronously to avoid blocking caller
		log.Printf("[WARN] Access batcher buffer full, writing synchronously")
		return b.writeSync(write)
	}
}

// run is the main batching loop
func (b *AccessBatcher) run() {
	defer b.wg.Done()

	batch := make([]*AccessWrite, 0, b.batchSize)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case write := <-b.buffer:
			batch = append(batch, write)

			// Flush if batch is full
			if len(batch) >= b.batchSize {
				b.flush(batch)
				batch = make([]*AccessWrite, 0, b.batchSize)
				ticker.Reset(b.flushInterval)
			}

		case <-ticker.C:
			// Flush on timeout if batch has items
			if len(batch) > 0 {
				b.flush(batch)
				batch = make([]*AccessWrite, 0, b.batchSize)
			}

		case <-b.stopChan:
			// Flush remaining items on shutdown
			if len(batch) > 0 {
				b.flush(batch)
			}
			return
		}
	}
}

// flush writes a batch of access operations to Valkey
func (b *AccessBatcher) flush(batch []*AccessWrite) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	successCount := 0
	errorCount := 0

	// Process each access write
	// Note: We could optimize further by grouping by threadID and using a single Lua script call
	// but for now, process individually with timeout context
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
			nil, // threadData - not creating thread
			nil, // threadTTL - not creating thread
		)

		if err != nil {
			log.Printf("[ERROR] Failed to grant access in batch: threadID=%s, userID=%s, error=%v",
				write.ThreadID, write.UserID, err)
			errorCount++

			// Send error back if result channel exists
			if write.ResultChan != nil {
				select {
				case write.ResultChan <- err:
				default:
				}
			}
		} else {
			successCount++

			// Send success back if result channel exists
			if write.ResultChan != nil {
				select {
				case write.ResultChan <- nil:
				default:
				}
			}
		}
	}

	duration := time.Since(start)
	log.Printf("[ACCESS-BATCHER] Flushed batch: size=%d, success=%d, errors=%d, duration=%v",
		len(batch), successCount, errorCount, duration)
}

// writeSync writes an access operation synchronously (fallback when buffer is full)
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
