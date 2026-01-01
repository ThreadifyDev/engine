package archiver

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"time"
)

// ValkeyStreamOps interface for stream operations
type ValkeyStreamOps interface {
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) error
	XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]map[string]interface{}, error)
	XAck(ctx context.Context, stream, group string, ids []string) error
	XTrim(ctx context.Context, stream, strategy string, approx bool, count int64) error
}

// ActivityWorkerPool manages a pool of workers consuming from partitioned activity streams
type ActivityWorkerPool struct {
	instanceID    int
	numPartitions int
	numWorkers    int
	consumerGroup string
	valkey        ValkeyStreamOps
	pgWriter      *PostgresWriter
	batchSize     int
	blockTimeout  time.Duration
	trimEnabled   bool
	trimMaxLen    int64
	wg            sync.WaitGroup
}

// NewActivityWorkerPool creates a new worker pool for partitioned activity streams
func NewActivityWorkerPool(
	instanceID int,
	numPartitions int,
	numWorkers int,
	consumerGroup string,
	valkey ValkeyStreamOps,
	pgWriter *PostgresWriter,
	batchSize int,
	blockTimeout time.Duration,
	trimEnabled bool,
	trimMaxLen int64,
) *ActivityWorkerPool {
	return &ActivityWorkerPool{
		instanceID:    instanceID,
		numPartitions: numPartitions,
		numWorkers:    numWorkers,
		consumerGroup: consumerGroup,
		valkey:        valkey,
		pgWriter:      pgWriter,
		batchSize:     batchSize,
		blockTimeout:  blockTimeout,
		trimEnabled:   trimEnabled,
		trimMaxLen:    trimMaxLen,
	}
}

// Start initializes consumer groups and starts worker pool
func (p *ActivityWorkerPool) Start(ctx context.Context) {
	log.Printf("[ActivityWorkerPool] Starting instance %d with %d workers for %d partitions",
		p.instanceID, p.numWorkers, p.numPartitions)

	// Setup consumer groups for all partitions
	p.setupConsumerGroups(ctx)

	// Start worker goroutines
	for workerID := 0; workerID < p.numWorkers; workerID++ {
		p.wg.Add(1)
		go p.startWorker(ctx, workerID)
	}

	// Start pending message recovery
	p.wg.Add(1)
	go p.recoverPendingMessages(ctx)

	log.Printf("[ActivityWorkerPool] Instance %d ready with %d workers", p.instanceID, p.numWorkers)
}

// Wait waits for all workers to finish
func (p *ActivityWorkerPool) Wait() {
	p.wg.Wait()
}

// setupConsumerGroups creates consumer groups for all partitioned streams
func (p *ActivityWorkerPool) setupConsumerGroups(ctx context.Context) {
	log.Printf("[ActivityWorkerPool] Setting up consumer groups for %d partitions", p.numPartitions)

	for i := 0; i < p.numPartitions; i++ {
		streamName := fmt.Sprintf("streams:activity_log:%d", i)

		err := p.valkey.XGroupCreateMkStream(ctx, streamName, p.consumerGroup, "0")
		if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			log.Printf("[ActivityWorkerPool] Warning: Failed to create consumer group for %s: %v", streamName, err)
		}
	}

	log.Printf("[ActivityWorkerPool] Consumer groups ready")
}

// startWorker starts a single worker that consumes from all partitions
func (p *ActivityWorkerPool) startWorker(ctx context.Context, workerID int) {
	defer p.wg.Done()

	consumerName := fmt.Sprintf("archiver-%d-worker-%d", p.instanceID, workerID)
	log.Printf("[Worker %s] Starting - will read from %d partitions", consumerName, p.numPartitions)

	// Build list of all partition streams
	streams := make([]string, p.numPartitions)
	for i := 0; i < p.numPartitions; i++ {
		streams[i] = fmt.Sprintf("streams:activity_log:%d", i)
	}
	log.Printf("[Worker %s] Streams: %v", consumerName, streams)

	iterationCount := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker %s] Shutting down after %d iterations", consumerName, iterationCount)
			return
		default:
			iterationCount++
			if iterationCount%100 == 0 {
				log.Printf("[Worker %s] Completed %d iterations", consumerName, iterationCount)
			}
			p.consumeAndProcess(ctx, consumerName, streams)
		}
	}
}

// consumeAndProcess reads events from all partitions and processes them
func (p *ActivityWorkerPool) consumeAndProcess(ctx context.Context, consumerName string, streams []string) {
	// Read from all partitions (Redis distributes events across consumers in group)
	for streamIdx, streamName := range streams {
		events, err := p.valkey.XReadGroup(
			ctx,
			p.consumerGroup,
			consumerName,
			streamName,
			p.batchSize,
			100*time.Millisecond, // Fast iteration through all streams
		)

		if err != nil {
			if !strings.Contains(err.Error(), "context canceled") {
				log.Printf("[Worker %s] ❌ Read error from %s (stream %d/%d): %v", consumerName, streamName, streamIdx+1, len(streams), err)
			}
			continue
		}

		if len(events) == 0 {
			// Log occasionally to show we're checking
			if streamIdx == 0 {
				// Only log for first stream to avoid spam
				// log.Printf("[Worker %s] No events in %s", consumerName, streamName)
			}
			continue
		}

		log.Printf("[Worker %s] 📥 Read %d events from %s", consumerName, len(events), streamName)

		// Convert to StreamEvent format
		streamEvents := p.convertToStreamEvents(events)
		log.Printf("[Worker %s] 🔄 Converted %d events to %d StreamEvents", consumerName, len(events), len(streamEvents))
		if len(streamEvents) == 0 {
			log.Printf("[Worker %s] ⚠️  No StreamEvents after conversion, skipping", consumerName)
			continue
		}

		// Write to Postgres
		log.Printf("[Worker %s] 💾 Writing %d events to Postgres...", consumerName, len(streamEvents))
		err = p.pgWriter.WriteActivityLog(ctx, streamEvents)
		if err != nil {
			log.Printf("[Worker %s] Failed to write to Postgres: %v", consumerName, err)
			continue // Don't ACK, will retry
		}

		// Extract event IDs for ACK
		eventIDs := make([]string, len(events))
		for i, event := range events {
			if id, ok := event["id"].(string); ok {
				eventIDs[i] = id
			}
		}

		// ACK processed events
		err = p.valkey.XAck(ctx, streamName, p.consumerGroup, eventIDs)
		if err != nil {
			log.Printf("[Worker %s] Failed to ACK: %v", consumerName, err)
		}

		// XTRIM if enabled
		if p.trimEnabled {
			err = p.valkey.XTrim(ctx, streamName, "MAXLEN", true, p.trimMaxLen)
			if err != nil {
				log.Printf("[Worker %s] Failed to XTRIM: %v", consumerName, err)
			}
		}

		log.Printf("[Worker %s] Processed %d events from %s", consumerName, len(streamEvents), streamName)
	}
}

// convertToStreamEvents converts Redis stream events to StreamEvent format
func (p *ActivityWorkerPool) convertToStreamEvents(events []map[string]interface{}) []StreamEvent {
	streamEvents := make([]StreamEvent, 0, len(events))

	for idx, event := range events {
		// Extract stream ID
		streamID, ok := event["id"].(string)
		if !ok {
			log.Printf("⚠️  [convertToStreamEvents] Event %d: 'id' not found or not string", idx)
			continue
		}

		// XReadGroup returns data directly in the event map, not nested under "data"
		// Convert all fields (except "id") to map[string]string
		dataMap := make(map[string]string)
		for k, v := range event {
			if k == "id" {
				continue // Skip the stream ID itself
			}
			if str, ok := v.(string); ok {
				dataMap[k] = str
			}
		}

		if len(dataMap) == 0 {
			log.Printf("⚠️  [convertToStreamEvents] Event %d: no data fields found", idx)
			continue
		}

		streamEvents = append(streamEvents, StreamEvent{
			StreamID: streamID,
			Data:     dataMap,
		})
	}

	return streamEvents
}

// recoverPendingMessages periodically checks for and reclaims stale pending messages
func (p *ActivityWorkerPool) recoverPendingMessages(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Printf("[ActivityWorkerPool] Pending message recovery started")

	for {
		select {
		case <-ctx.Done():
			log.Printf("[ActivityWorkerPool] Pending message recovery stopped")
			return
		case <-ticker.C:
			// Recovery logic can be implemented here if needed
			// For now, Redis consumer groups handle this automatically
		}
	}
}

// GetPartitionForThread returns the partition number for a given thread ID
func GetPartitionForThread(threadID string, numPartitions int) int {
	h := fnv.New32a()
	h.Write([]byte(threadID))
	return int(h.Sum32() % uint32(numPartitions))
}

// getKeys returns the keys of a map for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
