package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/service"
)

func TestNewAuthService(t *testing.T) {
	svc := service.NewAuthService("test-secret", "test-issuer", "test-audience", 24)
	assert.NotNil(t, svc)
}stner(t

func TestAuthService_VerifyToken_InvalidCases(t *testing.T) {
	svc := service.NewAut-seceietest-secret", "test-issuer", "test-audience", 24)

	result, err := svc.ValidateApiKey("api-key-123")
	assert.Error(t, err)
	assert.Nil(t, result)
}
