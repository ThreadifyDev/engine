package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventBuffer_Add(t *testing.T) {
	buffer := NewEventBuffer(10, 5*time.Second)

	event := StreamEvent{
		StreamID: "1234567890-0",
		Data: map[string]string{
			"threadId": "thread-123",
			"stepName": "payment",
		},
	}

	buffer.Add(event)

	assert.Equal(t, 1, buffer.Len())
}

func TestEventBuffer_ShouldFlush_Size(t *testing.T) {
	buffer := NewEventBuffer(3, 10*time.Second)

	// Add events up to max size
	for i := 0; i < 3; i++ {
		buffer.Add(StreamEvent{
			StreamID: "id-" + string(rune(i)),
			Data:     map[string]string{"key": "value"},
		})
	}

	assert.True(t, buffer.ShouldFlush())
}

func TestEventBuffer_ShouldFlush_Time(t *testing.T) {
	buffer := NewEventBuffer(100, 100*time.Millisecond)

	buffer.Add(StreamEvent{
		StreamID: "id-1",
		Data:     map[string]string{"key": "value"},
	})

	// Should not flush immediately
	assert.False(t, buffer.ShouldFlush())

	// Wait for flush interval
	time.Sleep(150 * time.Millisecond)

	// Should flush now
	assert.True(t, buffer.ShouldFlush())
}

func TestEventBuffer_GetAndClear(t *testing.T) {
	buffer := NewEventBuffer(10, 5*time.Second)

	events := []StreamEvent{
		{StreamID: "id-1", Data: map[string]string{"key": "value1"}},
		{StreamID: "id-2", Data: map[string]string{"key": "value2"}},
	}

	for _, e := range events {
		buffer.Add(e)
	}

	retrieved := buffer.GetAndClear()

	assert.Equal(t, 2, len(retrieved))
	assert.Equal(t, 0, buffer.Len())
	assert.Equal(t, "id-1", retrieved[0].StreamID)
	assert.Equal(t, "id-2", retrieved[1].StreamID)
}

func TestEventBuffer_Concurrent(t *testing.T) {
	buffer := NewEventBuffer(100, 5*time.Second)

	// Add events concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				buffer.Add(StreamEvent{
					StreamID: "id",
					Data:     map[string]string{"n": string(rune(n))},
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 100, buffer.Len())
}
