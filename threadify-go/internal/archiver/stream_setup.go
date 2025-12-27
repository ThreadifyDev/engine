package archiver

import (
	"context"
	"fmt"

	"github.com/threadify/engine/internal/interfaces"
)

// StreamSetup handles initialization of Valkey streams and consumer groups
type StreamSetup struct {
	client interfaces.ValkeyClient
}

// NewStreamSetup creates a new stream setup helper
func NewStreamSetup(client interfaces.ValkeyClient) *StreamSetup {
	return &StreamSetup{client: client}
}

// EnsureConsumerGroup creates a consumer group if it doesn't exist
// This is idempotent - safe to call multiple times
func (s *StreamSetup) EnsureConsumerGroup(ctx context.Context, stream, group string) error {
	fmt.Printf("Ensuring consumer group '%s' exists for stream '%s'\n", group, stream)

	// Create consumer group with MKSTREAM (creates stream if doesn't exist)
	// Start from "0" to process all messages (or "$" for only new messages)
	err := s.client.XGroupCreateMkStream(ctx, stream, group, "0")
	if err != nil {
		// Check if error is "BUSYGROUP" (group already exists)
		if isGroupExistsError(err) {
			fmt.Printf("Consumer group '%s' already exists for stream '%s'\n", group, stream)
			return nil
		}
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	fmt.Printf("Created consumer group '%s' for stream '%s'\n", group, stream)
	return nil
}

// isGroupExistsError checks if the error is because the group already exists
func isGroupExistsError(err error) bool {
	if err == nil {
		return false
	}
	// Redis returns "BUSYGROUP Consumer Group name already exists"
	return err.Error() == "BUSYGROUP Consumer Group name already exists"
}

// EnsureAllStreams sets up all required streams and consumer groups
func (s *StreamSetup) EnsureAllStreams(ctx context.Context, consumerGroup string, streams []string) error {
	for _, stream := range streams {
		if err := s.EnsureConsumerGroup(ctx, stream, consumerGroup); err != nil {
			return fmt.Errorf("failed to setup stream %s: %w", stream, err)
		}
	}
	return nil
}

// GetRequiredStreams returns the list of streams the archiver needs
func GetRequiredStreams() []string {
	return []string{
		"streams:step_events",
		"streams:thread_metadata",
		"streams:audit_logs",
		"streams:invitations",
		"streams:thread_access",
	}
}
