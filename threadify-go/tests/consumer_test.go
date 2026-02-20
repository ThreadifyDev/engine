package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStreamReader for testing
type MockStreamReader struct {
	mock.Mock
}

func (m *MockStreamReader) XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]StreamEvent, error) {
	args := m.Called(ctx, group, consumer, stream, count, block)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]StreamEvent), args.Error(1)
}

func TestConsumer_Read_Success(t *testing.T) {
	mockReader := new(MockStreamReader)
	buffer := NewEventBuffer(10, 5*time.Second)
	consumer := NewConsumer("test-stream", "test-group", "consumer-1", mockReader, buffer, 100, 5*time.Second)

	events := []StreamEvent{
		{StreamID: "id-1", Data: map[string]string{"key": "value1"}},
		{StreamID: "id-2", Data: map[string]string{"key": "value2"}},
	}

	mockReader.On("XReadGroup", mock.Anything, "test-group", "consumer-1", "test-stream", 100, 5*time.Second).Return(events, nil)

	err := consumer.Read(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, 2, buffer.Len())
	mockReader.AssertExpectations(t)
}

func TestConsumer_Read_NoEvents(t *testing.T) {
	mockReader := new(MockStreamReader)
	buffer := NewEventBuffer(10, 5*time.Second)
	consumer := NewConsumer("test-stream", "test-group", "consumer-1", mockReader, buffer, 100, 5*time.Second)

	mockReader.On("XReadGroup", mock.Anything, "test-group", "consumer-1", "test-stream", 100, 5*time.Second).Return([]StreamEvent{}, nil)

	err := consumer.Read(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, 0, buffer.Len())
	mockReader.AssertExpectations(t)
}

func TestConsumer_Read_Error(t *testing.T) {
	mockReader := new(MockStreamReader)
	buffer := NewEventBuffer(10, 5*time.Second)
	consumer := NewConsumer("test-stream", "test-group", "consumer-1", mockReader, buffer, 100, 5*time.Second)

	mockReader.On("XReadGroup", mock.Anything, "test-group", "consumer-1", "test-stream", 100, 5*time.Second).Return(nil, errors.New("connection error"))

	err := consumer.Read(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection error")
	mockReader.AssertExpectations(t)
}

func TestConsumer_Run_StopsOnContextCancel(t *testing.T) {
	mockReader := new(MockStreamReader)
	buffer := NewEventBuffer(10, 5*time.Second)
	consumer := NewConsumer("test-stream", "test-group", "consumer-1", mockReader, buffer, 100, 100*time.Millisecond)

	// Mock will be called multiple times until context is cancelled
	mockReader.On("XReadGroup", mock.Anything, "test-group", "consumer-1", "test-stream", 100, 100*time.Millisecond).Return([]StreamEvent{}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	// Run should exit when context is cancelled
	consumer.Run(ctx)

	// Should have been called at least once
	mockReader.AssertCalled(t, "XReadGroup", mock.Anything, "test-group", "consumer-1", "test-stream", 100, 100*time.Millisecond)
}
