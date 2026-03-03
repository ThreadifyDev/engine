package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/internal/interfaces"
)

// Unified MockValkeyClient for all tests
type MockValkeyClient struct {
	mock.Mock
	data         map[string]string
	hash         map[string]map[string]string
	currentSteps map[string][]string
	stepStates   map[string]map[string]string
}

func NewMockValkeyClient() *MockValkeyClient {
	return &MockValkeyClient{
		data:         make(map[string]string),
		hash:         make(map[string]map[string]string),
		currentSteps: make(map[string][]string),
		stepStates:   make(map[string]map[string]string),
	}
}

// Basic operations (from thread_access_test.go)
func (m *MockValkeyClient) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *MockValkeyClient) Get(ctx context.Context, key string) (string, error) {
	return m.data[key], nil
}

func (m *MockValkeyClient) Exists(ctx context.Context, key string) (bool, error) {
	_, exists := m.data[key]
	return exists, nil
}

func (m *MockValkeyClient) Del(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		delete(m.data, key)
		delete(m.hash, key)
	}
	return nil
}

func (m *MockValkeyClient) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	delete(m.hash, key)
	return nil
}

func (m *MockValkeyClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	// Mock implementation - just return success
	return nil
}

func (m *MockValkeyClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return 0, nil
}

func (m *MockValkeyClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	// Mock implementation - return empty slice for testing
	return []string{}, nil
}

// Hash operations
func (m *MockValkeyClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	if m.hash[key] == nil {
		m.hash[key] = make(map[string]string)
	}
	for i := 0; i < len(values); i += 2 {
		field := values[i].(string)
		value := values[i+1].(string)
		m.hash[key][field] = value
	}
	return nil
}

func (m *MockValkeyClient) HGet(ctx context.Context, key, field string) (string, error) {
	if m.stepStates[key] != nil {
		return m.stepStates[key][field], nil
	}
	if m.hash[key] != nil {
		return m.hash[key][field], nil
	}
	return "", nil
}

func (m *MockValkeyClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if m.stepStates[key] != nil {
		result := make(map[string]string)
		for k, v := range m.stepStates[key] {
			result[k] = v
		}
		return result, nil
	}
	if m.hash[key] != nil {
		result := make(map[string]string)
		for k, v := range m.hash[key] {
			result[k] = v
		}
		return result, nil
	}
	return make(map[string]string), nil
}

func (m *MockValkeyClient) HDel(ctx context.Context, key string, fields ...string) error {
	if m.hash[key] != nil {
		for _, field := range fields {
			delete(m.hash[key], field)
		}
	}
	return nil
}

// Sorted set operations (for validation)
func (m *MockValkeyClient) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	threadID := extractThreadIDFromKey(key)
	return m.currentSteps[threadID], nil
}

func (m *MockValkeyClient) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return nil
}

func (m *MockValkeyClient) ZRem(ctx context.Context, key string, members ...string) error {
	return nil
}

func (m *MockValkeyClient) ZCard(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) ZScore(ctx context.Context, key, member string) (float64, error) {
	return 0, nil
}

func (m *MockValkeyClient) ZRank(ctx context.Context, key, member string) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) ZRangeByScore(ctx context.Context, key string, min, max string) ([]string, error) {
	return nil, nil
}

func (m *MockValkeyClient) ZCount(ctx context.Context, key string, min, max string) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	return nil
}

func (m *MockValkeyClient) ZRemRangeByScore(ctx context.Context, key string, min, max string) error {
	return nil
}

// Stream operations
func (m *MockValkeyClient) XAdd(ctx context.Context, key string, values map[string]interface{}) (string, error) {
	return "", nil
}

func (m *MockValkeyClient) XRead(ctx context.Context, key string, count int64) ([]map[string]string, error) {
	return nil, nil
}

func (m *MockValkeyClient) XReadGroup(ctx context.Context, group, consumer, key string, count int64) ([]map[string]string, error) {
	return nil, nil
}

func (m *MockValkeyClient) XAck(ctx context.Context, stream, group string, ids []string) error {
	if len(m.ExpectedCalls) == 0 {
		return nil
	}
	args := m.Called(ctx, stream, group, ids)
	return args.Error(0)
}

func (m *MockValkeyClient) XDel(ctx context.Context, key, id string) error {
	return nil
}

func (m *MockValkeyClient) XLen(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) XTrim(ctx context.Context, key string, maxLen int64) error {
	return nil
}

func (m *MockValkeyClient) XGroupCreate(ctx context.Context, key, group, start string) error {
	return nil
}

// List operations
func (m *MockValkeyClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return nil
}

func (m *MockValkeyClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	return nil
}

func (m *MockValkeyClient) LPop(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *MockValkeyClient) RPop(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *MockValkeyClient) LLen(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) LIndex(ctx context.Context, key string, index int64) (string, error) {
	return "", nil
}

func (m *MockValkeyClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return nil, nil
}

func (m *MockValkeyClient) LTrim(ctx context.Context, key string, start, stop int64) error {
	return nil
}

// Set operations
func (m *MockValkeyClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return nil
}

func (m *MockValkeyClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	return nil
}

func (m *MockValkeyClient) SCard(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return nil, nil
}

// Hash operations extended
func (m *MockValkeyClient) HExists(ctx context.Context, key, field string) (bool, error) {
	if m.hash[key] != nil {
		_, exists := m.hash[key][field]
		return exists, nil
	}
	return false, nil
}

func (m *MockValkeyClient) HKeys(ctx context.Context, key string) ([]string, error) {
	if m.hash[key] == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(m.hash[key]))
	for k := range m.hash[key] {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *MockValkeyClient) HVals(ctx context.Context, key string) ([]string, error) {
	if m.hash[key] == nil {
		return nil, nil
	}
	vals := make([]string, 0, len(m.hash[key]))
	for _, v := range m.hash[key] {
		vals = append(vals, v)
	}
	return vals, nil
}

func (m *MockValkeyClient) HLen(ctx context.Context, key string) (int64, error) {
	if m.hash[key] == nil {
		return 0, nil
	}
	return int64(len(m.hash[key])), nil
}

func (m *MockValkeyClient) HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	return 0, nil
}

func (m *MockValkeyClient) HIncrByFloat(ctx context.Context, key, field string, incr float64) (float64, error) {
	return 0, nil
}

// Extended operations
func (m *MockValkeyClient) SetEX(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return m.Set(ctx, key, value.(string), expiration)
}

func (m *MockValkeyClient) Pipeline() interfaces.ValkeyPipeline {
	return &MockValkeyPipeline{}
}

// MockValkeyPipeline implements ValkeyPipeline interface for testing
type MockValkeyPipeline struct{}

func (p *MockValkeyPipeline) HSet(ctx context.Context, key string, values ...interface{}) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) HDel(ctx context.Context, key string, fields ...string) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) LPush(ctx context.Context, key string, values ...interface{}) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) XAdd(ctx context.Context, stream string, values map[string]interface{}) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) Expire(ctx context.Context, key string, expiration time.Duration) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) Del(ctx context.Context, keys ...string) interfaces.ValkeyPipeline {
	return p
}

func (p *MockValkeyPipeline) Exec(ctx context.Context) ([]interface{}, error) {
	return nil, nil
}

// Script operations
func (m *MockValkeyClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return nil, nil
}

func (m *MockValkeyClient) EvalSHA(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error) {
	return nil, nil
}

func (m *MockValkeyClient) ScriptExists(ctx context.Context, sha1 ...string) ([]bool, error) {
	return nil, nil
}

func (m *MockValkeyClient) ScriptLoad(ctx context.Context, script string) (string, error) {
	return "", nil
}

func (m *MockValkeyClient) ScriptFlush(ctx context.Context) error {
	return nil
}

func (m *MockValkeyClient) ExecuteWithBackoff(ctx context.Context, operation func() error) error {
	return operation()
}

// Helper methods for validation testing
func (m *MockValkeyClient) SetCurrentSteps(threadID string, steps []string) {
	m.currentSteps[threadID] = steps
}

func (m *MockValkeyClient) SetStepState(threadID, stepName, idempKey, field, value string) {
	key := fmt.Sprintf("thread:%s:steps:%s:%s", threadID, stepName, idempKey)
	if m.stepStates[key] == nil {
		m.stepStates[key] = make(map[string]string)
	}
	m.stepStates[key][field] = value
}

func extractThreadIDFromKey(key string) string {
	// Extract thread ID from key format like "thread:123:current_steps"
	// This is a simplified extraction for testing
	if len(key) > 7 && key[:7] == "thread:" {
		parts := fmt.Sprintf("%s", key)
		if len(parts) > 7 {
			return parts[7:10] // Simplified for testing
		}
	}
	return "test-thread"
}

// Ensure MockValkeyClient implements ValkeyClient interface
var _ interfaces.ValkeyClient = (*MockValkeyClient)(nil)
