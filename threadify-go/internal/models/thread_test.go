package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewThread(t *testing.T) {
	t.Run("creates thread with correct initial state", func(t *testing.T) {
		thread := NewThread("thread-123", "contract-456", 1, "owner-789")

		assert.Equal(t, "thread-123", thread.ID)
		assert.Equal(t, "contract-456", thread.ContractID)
		assert.Equal(t, 1, thread.ContractVersion)
		assert.Equal(t, "owner-789", thread.OwnerID)
		assert.Equal(t, ThreadStatusActive, thread.Status)
		assert.NotNil(t, thread.Context)
		assert.NotNil(t, thread.Steps)
		assert.Empty(t, thread.Steps)
		assert.False(t, thread.StartedAt.IsZero())
		assert.Nil(t, thread.CompletedAt)
		assert.Empty(t, thread.Error)
	})
}

func TestThread_StartStep(t *testing.T) {
	t.Run("starts a new step", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		thread.StartStep("step-a")

		assert.Equal(t, "step-a", thread.CurrentStep)
		require.NotNil(t, thread.Steps["step-a"])
		assert.Equal(t, "step-a", thread.Steps["step-a"].ID)
		assert.Equal(t, StepStatusInProgress, thread.Steps["step-a"].Status)
		assert.NotNil(t, thread.Steps["step-a"].StartedAt)
		assert.Nil(t, thread.Steps["step-a"].CompletedAt)
		assert.Equal(t, 0, thread.Steps["step-a"].RetryCount)
	})

	t.Run("restarts an existing step", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		// Start step first time
		thread.StartStep("step-a")
		firstStartTime := *thread.Steps["step-a"].StartedAt

		// Wait a bit and restart
		time.Sleep(10 * time.Millisecond)
		thread.StartStep("step-a")

		assert.Equal(t, StepStatusInProgress, thread.Steps["step-a"].Status)
		assert.True(t, thread.Steps["step-a"].StartedAt.After(firstStartTime))
	})
}

func TestThread_CompleteStep(t *testing.T) {
	t.Run("completes a step with context", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.StartStep("step-a")

		context := map[string]interface{}{
			"result": "success",
			"data":   123,
		}
		thread.CompleteStep("step-a", context)

		assert.Equal(t, StepStatusCompleted, thread.Steps["step-a"].Status)
		assert.NotNil(t, thread.Steps["step-a"].CompletedAt)
		assert.Equal(t, "success", thread.Steps["step-a"].Context["result"])
		assert.Equal(t, 123, thread.Steps["step-a"].Context["data"])
	})

	t.Run("completes a step without context", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.StartStep("step-a")

		thread.CompleteStep("step-a", nil)

		assert.Equal(t, StepStatusCompleted, thread.Steps["step-a"].Status)
		assert.NotNil(t, thread.Steps["step-a"].CompletedAt)
	})

	t.Run("does nothing for non-existent step", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		// Should not panic
		thread.CompleteStep("non-existent", nil)

		assert.Nil(t, thread.Steps["non-existent"])
	})
}

func TestThread_FailStep(t *testing.T) {
	t.Run("fails a step with error message", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.StartStep("step-a")

		thread.FailStep("step-a", "connection timeout")

		assert.Equal(t, StepStatusFailed, thread.Steps["step-a"].Status)
		assert.NotNil(t, thread.Steps["step-a"].CompletedAt)
		assert.Equal(t, "connection timeout", thread.Steps["step-a"].Error)
	})

	t.Run("does nothing for non-existent step", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		// Should not panic
		thread.FailStep("non-existent", "error")

		assert.Nil(t, thread.Steps["non-existent"])
	})
}

func TestThread_Complete(t *testing.T) {
	t.Run("marks thread as completed", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		thread.Complete()

		assert.Equal(t, ThreadStatusCompleted, thread.Status)
		assert.NotNil(t, thread.CompletedAt)
		assert.False(t, thread.CompletedAt.IsZero())
	})
}

func TestThread_Fail(t *testing.T) {
	t.Run("marks thread as failed with error", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		thread.Fail("workflow error")

		assert.Equal(t, ThreadStatusFailed, thread.Status)
		assert.NotNil(t, thread.CompletedAt)
		assert.Equal(t, "workflow error", thread.Error)
	})
}

func TestThread_Cancel(t *testing.T) {
	t.Run("marks thread as cancelled", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		thread.Cancel()

		assert.Equal(t, ThreadStatusCancelled, thread.Status)
		assert.NotNil(t, thread.CompletedAt)
	})
}

func TestThread_UpdateContext(t *testing.T) {
	t.Run("updates thread context", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		thread.UpdateContext("user", "john")
		thread.UpdateContext("count", 42)

		assert.Equal(t, "john", thread.Context["user"])
		assert.Equal(t, 42, thread.Context["count"])
	})

	t.Run("initializes context if nil", func(t *testing.T) {
		thread := &Thread{}

		thread.UpdateContext("key", "value")

		assert.NotNil(t, thread.Context)
		assert.Equal(t, "value", thread.Context["key"])
	})
}

func TestThread_ToJSON_FromJSON(t *testing.T) {
	t.Run("serializes and deserializes thread", func(t *testing.T) {
		original := NewThread("thread-1", "contract-1", 1, "owner-1")
		original.StartStep("step-a")
		original.CompleteStep("step-a", map[string]interface{}{"result": "ok"})
		original.UpdateContext("user", "alice")

		// Serialize
		jsonData, err := original.ToJSON()
		require.NoError(t, err)
		assert.NotEmpty(t, jsonData)

		// Deserialize
		restored, err := FromJSON(jsonData)
		require.NoError(t, err)

		assert.Equal(t, original.ID, restored.ID)
		assert.Equal(t, original.ContractID, restored.ContractID)
		assert.Equal(t, original.ContractVersion, restored.ContractVersion)
		assert.Equal(t, original.OwnerID, restored.OwnerID)
		assert.Equal(t, original.Status, restored.Status)
		assert.Equal(t, original.CurrentStep, restored.CurrentStep)
		assert.Equal(t, "alice", restored.Context["user"])
		assert.NotNil(t, restored.Steps["step-a"])
		assert.Equal(t, StepStatusCompleted, restored.Steps["step-a"].Status)
	})

	t.Run("handles completed thread", func(t *testing.T) {
		original := NewThread("thread-1", "contract-1", 1, "owner-1")
		original.Complete()

		jsonData, err := original.ToJSON()
		require.NoError(t, err)

		restored, err := FromJSON(jsonData)
		require.NoError(t, err)

		assert.Equal(t, ThreadStatusCompleted, restored.Status)
		assert.NotNil(t, restored.CompletedAt)
	})

	t.Run("handles failed thread", func(t *testing.T) {
		original := NewThread("thread-1", "contract-1", 1, "owner-1")
		original.Fail("test error")

		jsonData, err := original.ToJSON()
		require.NoError(t, err)

		restored, err := FromJSON(jsonData)
		require.NoError(t, err)

		assert.Equal(t, ThreadStatusFailed, restored.Status)
		assert.Equal(t, "test error", restored.Error)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		_, err := FromJSON([]byte("invalid json"))
		assert.Error(t, err)
	})
}

func TestThread_IsActive(t *testing.T) {
	t.Run("returns true for active thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		assert.True(t, thread.IsActive())
	})

	t.Run("returns false for completed thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.Complete()
		assert.False(t, thread.IsActive())
	})

	t.Run("returns false for failed thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.Fail("error")
		assert.False(t, thread.IsActive())
	})

	t.Run("returns false for cancelled thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.Cancel()
		assert.False(t, thread.IsActive())
	})
}

func TestThread_IsCompleted(t *testing.T) {
	t.Run("returns false for active thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		assert.False(t, thread.IsCompleted())
	})

	t.Run("returns true for completed thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.Complete()
		assert.True(t, thread.IsCompleted())
	})

	t.Run("returns true for failed thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.Fail("error")
		assert.True(t, thread.IsCompleted())
	})

	t.Run("returns true for cancelled thread", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.Cancel()
		assert.True(t, thread.IsCompleted())
	})
}

func TestThread_WorkflowScenario(t *testing.T) {
	t.Run("complete workflow with multiple steps", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		// Step A
		thread.StartStep("step-a")
		assert.Equal(t, "step-a", thread.CurrentStep)
		thread.CompleteStep("step-a", map[string]interface{}{"output": "data-a"})

		// Step B
		thread.StartStep("step-b")
		assert.Equal(t, "step-b", thread.CurrentStep)
		thread.CompleteStep("step-b", map[string]interface{}{"output": "data-b"})

		// Step C
		thread.StartStep("step-c")
		thread.CompleteStep("step-c", nil)

		// Complete thread
		thread.Complete()

		assert.Equal(t, ThreadStatusCompleted, thread.Status)
		assert.Equal(t, 3, len(thread.Steps))
		assert.Equal(t, StepStatusCompleted, thread.Steps["step-a"].Status)
		assert.Equal(t, StepStatusCompleted, thread.Steps["step-b"].Status)
		assert.Equal(t, StepStatusCompleted, thread.Steps["step-c"].Status)
		assert.True(t, thread.IsCompleted())
		assert.False(t, thread.IsActive())
	})

	t.Run("workflow with failed step", func(t *testing.T) {
		thread := NewThread("thread-1", "contract-1", 1, "owner-1")

		// Step A succeeds
		thread.StartStep("step-a")
		thread.CompleteStep("step-a", nil)

		// Step B fails
		thread.StartStep("step-b")
		thread.FailStep("step-b", "network error")

		// Fail thread
		thread.Fail("step-b failed")

		assert.Equal(t, ThreadStatusFailed, thread.Status)
		assert.Equal(t, StepStatusCompleted, thread.Steps["step-a"].Status)
		assert.Equal(t, StepStatusFailed, thread.Steps["step-b"].Status)
		assert.Equal(t, "network error", thread.Steps["step-b"].Error)
		assert.Equal(t, "step-b failed", thread.Error)
	})
}
