package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

func TestContractRepository_SoftDelete_AllowsRecreateWithSameName(t *testing.T) {
	// Skip if no test database available
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test database
	connString := "postgres://postgres:postgres@localhost:5432/threadify_test?sslmode=disable"
	db, err := database.NewPostgresDB(connString, 5)
	if err != nil {
		t.Skip("Test database not available:", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Initialize schema
	err = db.InitSchema(ctx)
	require.NoError(t, err)

	repo := postgres.NewContractRepository(db.Pool, zap.NewNop())

	// Test scenario: Create -> Soft Delete -> Create again with same name
	t.Run("can create contract with same name after soft delete", func(t *testing.T) {
		contractName := "test_contract_" + uuid.New().String()[:8]
		ownerID := "test_owner_" + uuid.New().String()[:8]

		// 1. Create first contract
		hash1 := "hash1"
		contract1 := &models.Contract{
			ID:            uuid.New().String(),
			Name:          contractName,
			Description:   "First version",
			ContentHash:   &hash1,
			LatestVersion: 1,
			OwnerID:       ownerID,
			IsPublic:      false,
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := repo.Create(ctx, contract1)
		require.NoError(t, err, "Should create first contract")

		// 2. Soft delete the contract
		err = repo.SoftDelete(ctx, contract1.ID, time.Now())
		require.NoError(t, err, "Should soft delete contract")

		// 3. Create new contract with same name (should succeed now)
		hash2 := "hash2"
		contract2 := &models.Contract{
			ID:            uuid.New().String(),
			Name:          contractName, // Same name!
			Description:   "Second version",
			ContentHash:   &hash2,
			LatestVersion: 1,
			OwnerID:       ownerID,
			IsPublic:      false,
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err = repo.Create(ctx, contract2)
		assert.NoError(t, err, "Should allow creating contract with same name after soft delete")

		// 4. Verify we can retrieve the new contract
		retrieved, err := repo.GetByName(ctx, contractName)
		require.NoError(t, err)
		assert.Equal(t, contract2.ID, retrieved.ID, "Should retrieve the new contract, not the deleted one")
		assert.Equal(t, "Second version", retrieved.Description)
	})

	t.Run("cannot create duplicate active contracts", func(t *testing.T) {
		contractName := "duplicate_test_" + uuid.New().String()[:8]
		ownerID := "test_owner_" + uuid.New().String()[:8]

		// Create first contract
		hash1 := "hash1"
		contract1 := &models.Contract{
			ID:            uuid.New().String(),
			Name:          contractName,
			Description:   "First",
			ContentHash:   &hash1,
			LatestVersion: 1,
			OwnerID:       ownerID,
			IsPublic:      false,
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := repo.Create(ctx, contract1)
		require.NoError(t, err)

		// Try to create duplicate (should fail)
		hash2 := "hash2"
		contract2 := &models.Contract{
			ID:            uuid.New().String(),
			Name:          contractName, // Same name
			Description:   "Duplicate",
			ContentHash:   &hash2,
			LatestVersion: 1,
			OwnerID:       ownerID,
			IsPublic:      false,
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err = repo.Create(ctx, contract2)
		assert.Error(t, err, "Should not allow duplicate active contracts")
	})
}
