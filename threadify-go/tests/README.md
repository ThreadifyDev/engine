# Testing Guide for Threadify Go

This document explains how to run and write tests for the Threadify Engine Go implementation.

## Running Tests

### Run All Tests
```bash
go test ./...
```

### Run Tests with Coverage
```bash
go test ./... -cover
```

### Run Tests with Detailed Coverage Report
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run Tests for Specific Package
```bash
# Test services only
go test ./internal/services/...

# Test queue only
go test ./internal/queue/...

# Test handlers only
go test ./internal/handlers/...
```

### Run Specific Test
```bash
go test ./internal/services -run TestAuthService_CreateToken
```

### Run Tests in Verbose Mode
```bash
go test -v ./...
```

## Test Structure

Tests are organized alongside the code they test, following Go conventions:

```
internal/
├── services/
│   ├── auth.go
│   ├── auth_test.go          # Tests for auth service
│   ├── thread.go
│   ├── thread_test.go        # Tests for thread service
│   ├── contract.go
│   └── valkey.go
├── queue/
│   ├── client_queue.go
│   └── client_queue_test.go  # Tests for client queue
└── handlers/
    ├── contracts.go
    └── websocket.go
```

## Test Coverage

### Current Test Files

1. **`internal/services/auth_test.go`**
   - Tests JWT token creation
   - Tests token verification
   - Tests token expiration
   - Tests invalid tokens

2. **`internal/services/thread_test.go`**
   - Tests WebSocket connection handling
   - Tests thread lifecycle (connect, start, record, close)
   - Tests authentication requirements
   - Tests error handling

3. **`internal/queue/client_queue_test.go`**
   - Tests client addition to queue
   - Tests client retrieval
   - Tests client removal
   - Tests TTL configuration
   - Tests JSON serialization/deserialization

## Writing Tests

### Test Naming Convention
- Test files: `*_test.go`
- Test functions: `Test<FunctionName>`
- Subtests: Use `t.Run("description", func(t *testing.T) {...})`

### Example Test Structure
```go
func TestServiceMethod(t *testing.T) {
    t.Run("successful case", func(t *testing.T) {
        // Arrange
        service := NewService()
        
        // Act
        result := service.Method()
        
        // Assert
        assert.Equal(t, expected, result)
    })
    
    t.Run("error case", func(t *testing.T) {
        // Test error handling
    })
}
```

### Using Mocks
We use `github.com/stretchr/testify/mock` for mocking dependencies:

```go
type MockDependency struct {
    mock.Mock
}

func (m *MockDependency) Method(arg string) error {
    args := m.Called(arg)
    return args.Error(0)
}

// In test:
mockDep := new(MockDependency)
mockDep.On("Method", "test").Return(nil)
service := NewService(mockDep)
// ... test code ...
mockDep.AssertExpectations(t)
```

## Dependencies

The project uses these testing libraries:
- `github.com/stretchr/testify/assert` - Assertions
- `github.com/stretchr/testify/require` - Required assertions (fail fast)
- `github.com/stretchr/testify/mock` - Mocking framework

## Best Practices

1. **Test Independence**: Each test should be independent and not rely on other tests
2. **Use Table-Driven Tests**: For testing multiple scenarios with similar logic
3. **Mock External Dependencies**: Database, Redis, HTTP clients should be mocked
4. **Test Edge Cases**: Empty strings, nil values, boundary conditions
5. **Clear Test Names**: Use descriptive names that explain what is being tested
6. **Arrange-Act-Assert**: Structure tests with clear setup, execution, and verification

## Integration Tests

For integration tests that require real database/Redis connections:

```bash
# Set up test environment
docker-compose -f docker-compose.yml up -d

# Run integration tests (when implemented)
go test -tags=integration ./...

# Clean up
docker-compose down
```

## Continuous Integration

Tests are automatically run on:
- Every pull request
- Every commit to main branch
- Before deployment

Minimum coverage requirement: **80%**

## Troubleshooting

### Tests Fail with "missing go.sum entry"
```bash
go mod tidy
```

### Mock Expectations Not Met
Check that:
1. Mock methods are called with expected arguments
2. `AssertExpectations(t)` is called at the end of the test
3. Arguments match exactly (use `mock.Anything` for flexible matching)

### Race Condition Detected
Run tests with race detector:
```bash
go test -race ./...
```

## Future Test Additions

- [ ] Contract service integration tests
- [ ] WebSocket handler tests
- [ ] Middleware tests (rate limiting, auth)
- [ ] Database layer tests
- [ ] End-to-end API tests
