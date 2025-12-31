package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threadify/engine/internal/handlers"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/pkg/validator"
)

func TestContractPreview_ValidYAML(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	contractValidator := validator.NewContractValidator()
	contractService := service.NewContractService(nil, nil, contractValidator, nil)
	handler := handlers.NewContractHandler(contractService, nil, nil)

	router := gin.New()
	handler.RegisterRoutes(router)

	// Test YAML
	validYAML := `
contract_name: test_contract
version: 1
description: Test contract
parties: [merchant, customer]
entry_points: [order_placed]
steps:
  - id: order_placed
    owner: merchant
    timeout: 5m
  - id: payment_validation
    owner: payment_processor
    timeout: 10m
transitions:
  - from: order_placed
    to: [payment_validation]
validation:
  max_duration: 1h
`

	req, _ := http.NewRequest("POST", "/v1/contracts/preview", bytes.NewBufferString(validYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	req.Header.Set("Authorization", "Bearer test-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response handlers.PreviewResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Valid)
	assert.NotEmpty(t, response.Mermaid)
	assert.Contains(t, response.Mermaid, "flowchart TD")
	assert.Empty(t, response.Errors)
}

func TestContractPreview_InvalidYAML(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	contractValidator := validator.NewContractValidator()
	contractService := service.NewContractService(nil, nil, contractValidator, nil)
	handler := handlers.NewContractHandler(contractService, nil, nil)

	router := gin.New()
	handler.RegisterRoutes(router)

	// Invalid YAML - missing required fields
	invalidYAML := `
contract_name: test_contract
description: Test contract
# Missing version, parties, entry_points, steps
`

	req, _ := http.NewRequest("POST", "/v1/contracts/preview", bytes.NewBufferString(invalidYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	req.Header.Set("Authorization", "Bearer test-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response handlers.PreviewResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Valid)
	assert.Empty(t, response.Mermaid)
	assert.NotEmpty(t, response.Errors)
}

func TestContractPreview_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	contractValidator := validator.NewContractValidator()
	contractService := service.NewContractService(nil, nil, contractValidator, nil)
	handler := handlers.NewContractHandler(contractService, nil, nil)

	router := gin.New()
	handler.RegisterRoutes(router)

	validYAML := `
contract_name: test_contract
version: 1
description: Test contract
parties: [merchant]
entry_points: [order_placed]
steps:
  - id: order_placed
    owner: merchant
`

	req, _ := http.NewRequest("POST", "/v1/contracts/preview", bytes.NewBufferString(validYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	// No Authorization header

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
