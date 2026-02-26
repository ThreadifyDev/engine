package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/service"
)

func TestConnectionService_BasicOperations(t *testing.T) {
	conn := service.NewConnectionService()

	// Test connection
	err := conn.ConnectWithOwnerAndCompany("owner-123", "api-key-123", "test-service", "company-abc")
	assert.NoError(t, err)

	// Test IsConnected
	assert.True(t, conn.IsConnected("owner-123"))
	assert.False(t, conn.IsConnected("non-existent"))

	// Test GetClient
	client, exists := conn.GetClient("owner-123")
	assert.True(t, exists)
	assert.Equal(t, "owner-123", client.OwnerID)
	assert.Equal(t, "company-abc", client.CompanyID)
	assert.Equal(t, "api-key-123", client.ApiKey)
	assert.Equal(t, "test-service", client.ServiceName)
	assert.True(t, client.ConnectedAt.Before(time.Now().Add(time.Second)))

	// Test GetClient for non-existent
	_, exists = conn.GetClient("non-existent")
	assert.False(t, exists)

	// Test Disconnect
	err = conn.Disconnect("owner-123")
	assert.NoError(t, err)
	assert.False(t, conn.IsConnected("owner-123"))
	_, exists = conn.GetClient("owner-123")
	assert.False(t, exists)
}

func TestConnectionService_ConcurrentAccess(t *testing.T) {
	conn := service.NewConnectionService()

	// Test concurrent connections
	for i := 0; i < 10; i++ {
		ownerID := fmt.Sprintf("owner-%d", i)
		companyID := fmt.Sprintf("company-%d", i)
		err := conn.ConnectWithOwnerAndCompany(ownerID, "api-key", "service", companyID)
		assert.NoError(t, err)
	}

	// Verify all connections exist
	for i := 0; i < 10; i++ {
		ownerID := fmt.Sprintf("owner-%d", i)
		assert.True(t, conn.IsConnected(ownerID))
	}

	// Test concurrent disconnections
	for i := 0; i < 10; i++ {
		ownerID := fmt.Sprintf("owner-%d", i)
		err := conn.Disconnect(ownerID)
		assert.NoError(t, err)
	}

	// Verify all connections are gone
	for i := 0; i < 10; i++ {
		ownerID := fmt.Sprintf("owner-%d", i)
		assert.False(t, conn.IsConnected(ownerID))
	}
}
