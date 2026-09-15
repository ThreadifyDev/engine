package archiver

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOTelCompletionSnapshotCreatesBeforeCompleting(t *testing.T) {
	event := StreamEvent{Data: map[string]string{"action": "otel_completed", "threadId": "trace-thread", "status": "completed", "startedAt": "2026-09-15T10:00:00Z", "completedAt": "2026-09-15T10:00:01Z"}}
	insert, completion, refs := partitionThreadEvents([]StreamEvent{event})
	require.Len(t, insert, 1)
	require.Len(t, completion, 1)
	require.Empty(t, refs)
	require.Equal(t, "active", insert[0].Data["status"])
	require.Equal(t, "completed", event.Data["status"], "must not mutate the terminal snapshot")
	require.Equal(t, event.Data["startedAt"], insert[0].Data["startedAt"])
	require.Equal(t, event.Data["completedAt"], completion[0].Data["completedAt"])
}

func TestOTelCompletionReplayDoesNotDuplicateMetadataUpserts(t *testing.T) {
	end := StreamEvent{Data: map[string]string{"action": "otel_completed", "threadId": "trace-thread", "status": "completed"}}
	start := StreamEvent{Data: map[string]string{"threadId": "trace-thread", "status": "active"}}
	for _, events := range [][]StreamEvent{{end, end}, {start, end, end}} {
		insert, completion, _ := partitionThreadEvents(events)
		require.Len(t, insert, 1)
		require.Len(t, completion, 2)
	}
}
