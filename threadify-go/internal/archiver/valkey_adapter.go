package archiver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/threadify/engine/internal/interfaces"
)

// ValkeyStreamAdapter adapts the Valkey client to archiver interfaces
type ValkeyStreamAdapter struct {
	client interfaces.ValkeyClient
}

// NewValkeyStreamAdapter creates a new adapter
func NewValkeyStreamAdapter(client interfaces.ValkeyClient) *ValkeyStreamAdapter {
	return &ValkeyStreamAdapter{client: client}
}

// XReadGroup implements StreamReader interface
func (a *ValkeyStreamAdapter) XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]StreamEvent, error) {
	results, err := a.client.XReadGroup(ctx, group, consumer, stream, count, block)
	if err != nil {
		return nil, err
	}

	events := make([]StreamEvent, 0, len(results))
	for _, result := range results {
		// Extract stream ID and data
		streamID, _ := result["id"].(string)
		delete(result, "id") // Remove id from data

		// Convert map[string]interface{} to map[string]string
		data := make(map[string]string)
		for k, v := range result {
			if str, ok := v.(string); ok {
				data[k] = str
			}
		}

		events = append(events, StreamEvent{
			StreamID: streamID,
			Data:     data,
		})
	}

	return events, nil
}

// XAck implements ValkeyClient interface
func (a *ValkeyStreamAdapter) XAck(ctx context.Context, stream, group string, ids []string) error {
	return a.client.XAck(ctx, stream, group, ids)
}

// WriteEventToStream writes an event to a stream with pipeline
func WriteEventToStream(ctx context.Context, client interfaces.ValkeyClient, threadID, stream string, event map[string]interface{}) error {
	// Convert event to JSON for List storage
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Use pipeline for atomic write
	pipe := client.Pipeline()

	// Write to List (for immediate access)
	listKey := "thread:" + threadID + ":activity"
	pipe.LPush(ctx, listKey, string(eventJSON))

	// Write to Stream (for archival) with MAXLEN
	streamValues := make(map[string]interface{})
	streamValues["maxlen"] = "~"
	streamValues["limit"] = 100000
	for k, v := range event {
		streamValues[k] = v
	}
	pipe.XAdd(ctx, stream, streamValues)

	_, err = pipe.Exec(ctx)
	return err
}
