package valkey

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
)

// MockValkeyService is a mock implementation of ValkeyService for testing
type MockValkeyService struct {
	storage map[string]string
	err     error
}

func NewMockValkeyService() *MockValkeyService {
	return &MockValkeyService{
		storage: make(map[string]string),
	}
}

func (m *MockValkeyService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if m.err != nil {
		return m.err
	}
	m.storage[key] = value
	return nil
}

func (m *MockValkeyService) Get(ctx context.Context, key string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	value, exists := m.storage[key]
	if !exists {
		return "", errors.New("key not found")
	}
	return value, nil
}

func (m *MockValkeyService) Delete(ctx context.Context, key string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.storage, key)
	return nil
}

func (m *MockValkeyService) Exists(ctx context.Context, key string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, exists := m.storage[key]
	return exists, nil
}

func (m *MockValkeyService) Keys(ctx context.Context, pattern string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	keys := make([]string, 0, len(m.storage))
	// Simple pattern matching: convert * to match any characters
	for k := range m.storage {
		// Simple pattern match - just check prefix for now
		if matchPattern(k, pattern) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// matchPattern does simple pattern matching for Redis-style patterns
func matchPattern(key, pattern string) bool {
	// Simple implementation: split on * and check parts
	if pattern == "*" {
		return true
	}
	// For patterns like "thread:*", check if key starts with "thread:"
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	}
	return key == pattern
}

func (m *MockValkeyService) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if m.err != nil {
		return m.err
	}
	// Mock doesn't actually expire, just check if key exists
	if _, exists := m.storage[key]; !exists {
		return errors.New("key not found")
	}
	return nil
}

func (m *MockValkeyService) SetError(err error) {
	m.err = err
}

func TestNewThreadRepository(t *testing.T) {
	t.Run("creates repository with correct TTL", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)

		assert.NotNil(t, repo)
		assert.Equal(t, 3600, repo.ttl)
	})
}

func TestThreadRepository_Save(t *testing.T) {
	t.Run("saves thread successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")

		err := repo.Save(context.Background(), thread)

		require.NoError(t, err)
		assert.Contains(t, mock.storage, "thread:thread-1")
	})

	t.Run("saves thread with steps", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.StartStep("step-a")
		thread.CompleteStep("step-a", map[string]interface{}{"result": "ok"})

		err := repo.Save(context.Background(), thread)

		require.NoError(t, err)
		// Verify we can retrieve it
		retrieved, err := repo.Get(context.Background(), "thread-1")
		require.NoError(t, err)
		assert.Equal(t, "thread-1", retrieved.ID)
		assert.NotNil(t, retrieved.Steps["step-a"])
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")

		err := repo.Save(context.Background(), thread)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save thread")
	})
}

func TestThreadRepository_Get(t *testing.T) {
	t.Run("retrieves thread successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")
		thread.UpdateContext("key", "value")

		// Save first
		err := repo.Save(context.Background(), thread)
		require.NoError(t, err)

		// Retrieve
		retrieved, err := repo.Get(context.Background(), "thread-1")

		require.NoError(t, err)
		assert.Equal(t, "thread-1", retrieved.ID)
		assert.Equal(t, "contract-1", *retrieved.ContractID)
		assert.Equal(t, 1, *retrieved.ContractVersion)
		assert.Equal(t, "owner-1", retrieved.OwnerID)
		assert.Equal(t, "value", retrieved.Context["key"])
	})

	t.Run("returns error when thread not found", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)

		_, err := repo.Get(context.Background(), "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get thread")
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewThreadRepository(mock, 3600)

		_, err := repo.Get(context.Background(), "thread-1")

		assert.Error(t, err)
	})
}

func TestThreadRepository_Delete(t *testing.T) {
	t.Run("deletes thread successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")

		// Save first
		err := repo.Save(context.Background(), thread)
		require.NoError(t, err)

		// Delete
		err = repo.Delete(context.Background(), "thread-1")

		require.NoError(t, err)
		assert.NotContains(t, mock.storage, "thread:thread-1")
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewThreadRepository(mock, 3600)

		err := repo.Delete(context.Background(), "thread-1")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete thread")
	})
}

func TestThreadRepository_Exists(t *testing.T) {
	t.Run("returns true when thread exists", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")

		// Save first
		err := repo.Save(context.Background(), thread)
		require.NoError(t, err)

		// Check existence
		exists, err := repo.Exists(context.Background(), "thread-1")

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false when thread does not exist", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)

		exists, err := repo.Exists(context.Background(), "non-existent")

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewThreadRepository(mock, 3600)

		_, err := repo.Exists(context.Background(), "thread-1")

		assert.Error(t, err)
	})
}

func TestThreadRepository_GetByOwner(t *testing.T) {
	t.Run("returns thread IDs for owner", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)

		// Save multiple threads
		thread1 := models.NewThread("thread-1", "contract-1", 1, "owner-1")
		thread2 := models.NewThread("thread-2", "contract-2", 1, "owner-1")

		err := repo.Save(context.Background(), thread1)
		require.NoError(t, err)
		err = repo.Save(context.Background(), thread2)
		require.NoError(t, err)

		// Get by owner
		threadIDs, err := repo.GetByOwner(context.Background(), "owner-1")

		require.NoError(t, err)
		assert.Len(t, threadIDs, 2)
		assert.Contains(t, threadIDs, "thread-1")
		assert.Contains(t, threadIDs, "thread-2")
	})

	t.Run("returns empty list when no threads", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)

		threadIDs, err := repo.GetByOwner(context.Background(), "owner-1")

		require.NoError(t, err)
		assert.Empty(t, threadIDs)
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewThreadRepository(mock, 3600)

		_, err := repo.GetByOwner(context.Background(), "owner-1")

		assert.Error(t, err)
	})
}

func TestThreadRepository_ExtendTTL(t *testing.T) {
	t.Run("extends TTL successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("thread-1", "contract-1", 1, "owner-1")

		// Save first
		err := repo.Save(context.Background(), thread)
		require.NoError(t, err)

		// Extend TTL
		err = repo.ExtendTTL(context.Background(), "thread-1")

		require.NoError(t, err)
	})

	t.Run("returns error when thread does not exist", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)

		err := repo.ExtendTTL(context.Background(), "non-existent")

		assert.Error(t, err)
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewThreadRepository(mock, 3600)

		err := repo.ExtendTTL(context.Background(), "thread-1")

		assert.Error(t, err)
	})
}

func TestThreadRepository_KeyFormat(t *testing.T) {
	t.Run("generates correct key format", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewThreadRepository(mock, 3600)
		thread := models.NewThread("my-thread-123", "contract-1", 1, "owner-1")

		err := repo.Save(context.Background(), thread)
		require.NoError(t, err)

		assert.Contains(t, mock.storage, "thread:my-thread-123")
	})
}
