package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/service"
)

func TestNewAuthService(t *testing.T) {
	svc := service.NewAuthService(nil, 3600) // 1 hour cache TTL
	assert.NotNil(t, svc)
}

func TestAuthService_VerifyToken_InvalidCases(t *testing.T) {
	svc := service.NewAuthService(nil, 3600) // 1 hour cache TTL

	result, err := svc.ValidateApiKey("api-key-123")
	assert.Error(t, err)
	assert.Nil(t, result)
}
