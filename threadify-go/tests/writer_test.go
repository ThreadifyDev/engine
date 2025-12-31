package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockValkeyClient for testing
type MockValkeyClient struct {
	mock.Mock
}

func (m *MockValkeyClient) XAck(ctx context.Context, stream, group string, ids []string) error {
	args := m.Called(ctx, stream, group, ids)
	return args.Error(0)
}

func TestWriter_ProcessBatch_Success(t *testing.T) {
	mockValkey := new(MockValkeyClient)
	writer := NewWriter(mockValkey, 3, 1*time.Second, 16*time.Second)

	events := []StreamEvent{
		{StreamID: "id-1", Data: map[string]string{"key": "value1"}},
		{StreamID: "id-2", Data: map[string]string{"key": "value2"}},
	}

	// Mock successful ACK
	mockValkey.On("XAck", mock.Anything, "test-stream", "test-group", []string{"id-1", "id-2"}).Return(nil)

	err := writer.ProcessBatch(context.Background(), "test-stream", "test-group", events, func(ctx context.Context, events []StreamEvent) error {
		// Simulate successful write
		return nil
	})

	assert.NoError(t, err)
	mockValkey.AssertExpectations(t)
}

func TestWriter_ProcessBatch_WriteFailure_Retry(t *testing.T) {
	mockValkey := new(MockValkeyClient)
	writer := NewWriter(mockValkey, 3, 100*time.Millisecond, 1*time.Second)

	events := []StreamEvent{
		{StreamID: "id-1", Data: map[string]string{"key": "value1"}},
	}

	callCount := 0
	writeFunc := func(ctx context.Context, events []StreamEvent) error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil // Success on 3rd attempt
	}

	// Mock successful ACK after retries
	mockValkey.On("XAck", mock.Anything, "test-stream", "test-group", []string{"id-1"}).Return(nil)

	err := writer.ProcessBatch(context.Background(), "test-stream", "test-group", events, writeFunc)

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
	mockValkey.AssertExpectations(t)
}

func TestWriter_ProcessBatch_MaxRetriesExceeded(t *testing.T) {
	mockValkey := new(MockValkeyClient)
	writer := NewWriter(mockValkey, 2, 10*time.Millisecond, 100*time.Millisecond)

	events := []StreamEvent{
		{StreamID: "id-1", Data: map[string]string{"key": "value1"}},
	}

	writeFunc := func(ctx context.Context, events []StreamEvent) error {
		return errors.New("persistent error")
	}

	err := writer.ProcessBatch(context.Background(), "test-stream", "test-group", events, writeFunc)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")
	// Should NOT call XAck on failure
	mockValkey.AssertNotCalled(t, "XAck")
}

func TestWriter_ProcessBatch_EmptyBatch(t *testing.T) {
	mockValkey := new(MockValkeyClient)
	writer := NewWriter(mockValkey, 3, 1*time.Second, 16*time.Second)

	events := []StreamEvent{}

	err := writer.ProcessBatch(context.Background(), "test-stream", "test-group", events, func(ctx context.Context, events []StreamEvent) error {
		return nil
	})

	assert.NoError(t, err)
	mockValkey.AssertNotCalled(t, "XAck")
}
