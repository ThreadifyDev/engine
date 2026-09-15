package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"testing"
	"time"
)

// A socket read can reach the deadline before context's timer updates Err.
type deadlineBeforeCancellation struct{ context.Context }

func (deadlineBeforeCancellation) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Second), true
}
func TestWaitDeadlineIsTimeoutBeforeContextTimerRuns(t *testing.T) {
	ctx := deadlineBeforeCancellation{context.Background()}
	req := domain.WaitRequest{ThreadID: "thread", StepName: "charge", StepID: "event"}
	result := (&ThreadService{}).HandleWaitFor(ctx, req, "owner", "company")
	require.Equal(t, "timed_out", result.Decision)
	require.Equal(t, "event", result.StepID)
	require.Equal(t, "timed_out", (&ThreadService{}).AwaitWaitFor(ctx, req, "owner", "company").Decision)
}
