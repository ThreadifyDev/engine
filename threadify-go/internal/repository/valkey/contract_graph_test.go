package valkey

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
)

func TestNewContractGraphRepository(t *testing.T) {
	t.Run("creates repository with correct TTL", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		assert.NotNil(t, repo)
		assert.Equal(t, 7200, repo.ttl)
	})
}

func TestContractGraphRepository_Save(t *testing.T) {
	t.Run("saves contract graph successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
			Graph: domain.Graph{
				Nodes: map[string]domain.GraphNode{
					"step-a": {
						ID:   "step-a",
						Type: "step",
					},
				},
				FinalStep: "step-a",
			},
		}

		err := repo.Save(context.Background(), graph)

		require.NoError(t, err)
		assert.Contains(t, mock.storage, "contract_graph:contract-1:v1")
	})

	t.Run("saves complex graph with multiple nodes", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-2",
			Version:    2,
			Graph: domain.Graph{
				Nodes: map[string]domain.GraphNode{
					"step-a": {
						ID:        "step-a",
						Type:      "step",
						DependsOn: []string{},
					},
					"step-b": {
						ID:        "step-b",
						Type:      "step",
						DependsOn: []string{"step-a"},
					},
				},
				FinalStep: "step-b",
			},
		}

		err := repo.Save(context.Background(), graph)

		require.NoError(t, err)

		// Verify we can retrieve it
		retrieved, err := repo.Get(context.Background(), "contract-2", 2)
		require.NoError(t, err)
		assert.Equal(t, "contract-2", retrieved.ContractID)
		assert.Equal(t, 2, retrieved.Version)
		assert.Len(t, retrieved.Graph.Nodes, 2)
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
		}

		err := repo.Save(context.Background(), graph)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save contract graph")
	})
}

func TestContractGraphRepository_Get(t *testing.T) {
	t.Run("retrieves contract graph successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
			Graph: domain.Graph{
				Nodes: map[string]domain.GraphNode{
					"step-a": {ID: "step-a", Type: "step"},
				},
				FinalStep: "step-a",
			},
		}

		// Save first
		err := repo.Save(context.Background(), graph)
		require.NoError(t, err)

		// Retrieve
		retrieved, err := repo.Get(context.Background(), "contract-1", 1)

		require.NoError(t, err)
		assert.Equal(t, "contract-1", retrieved.ContractID)
		assert.Equal(t, 1, retrieved.Version)
		assert.Equal(t, "step-a", retrieved.Graph.FinalStep)
	})

	t.Run("returns error when graph not found", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		_, err := repo.Get(context.Background(), "non-existent", 1)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get contract graph")
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewContractGraphRepository(mock, 7200)

		_, err := repo.Get(context.Background(), "contract-1", 1)

		assert.Error(t, err)
	})
}

func TestContractGraphRepository_Delete(t *testing.T) {
	t.Run("deletes contract graph successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
		}

		// Save first
		err := repo.Save(context.Background(), graph)
		require.NoError(t, err)

		// Delete
		err = repo.Delete(context.Background(), "contract-1", 1)

		require.NoError(t, err)
		assert.NotContains(t, mock.storage, "contract_graph:contract-1:v1")
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewContractGraphRepository(mock, 7200)

		err := repo.Delete(context.Background(), "contract-1", 1)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete contract graph")
	})
}

func TestContractGraphRepository_Exists(t *testing.T) {
	t.Run("returns true when graph exists", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
		}

		// Save first
		err := repo.Save(context.Background(), graph)
		require.NoError(t, err)

		// Check existence
		exists, err := repo.Exists(context.Background(), "contract-1", 1)

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false when graph does not exist", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		exists, err := repo.Exists(context.Background(), "non-existent", 1)

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewContractGraphRepository(mock, 7200)

		_, err := repo.Exists(context.Background(), "contract-1", 1)

		assert.Error(t, err)
	})
}

func TestContractGraphRepository_ExtendTTL(t *testing.T) {
	t.Run("extends TTL successfully", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
		}

		// Save first
		err := repo.Save(context.Background(), graph)
		require.NoError(t, err)

		// Extend TTL
		err = repo.ExtendTTL(context.Background(), "contract-1", 1)

		require.NoError(t, err)
	})

	t.Run("returns error when graph does not exist", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		err := repo.ExtendTTL(context.Background(), "non-existent", 1)

		assert.Error(t, err)
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewContractGraphRepository(mock, 7200)

		err := repo.ExtendTTL(context.Background(), "contract-1", 1)

		assert.Error(t, err)
	})
}

func TestContractGraphRepository_DeleteByContract(t *testing.T) {
	t.Run("deletes all versions of a contract", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		// Save multiple versions
		graph1 := &domain.ContractGraph{ContractID: "contract-1", Version: 1}
		graph2 := &domain.ContractGraph{ContractID: "contract-1", Version: 2}
		graph3 := &domain.ContractGraph{ContractID: "contract-1", Version: 3}

		err := repo.Save(context.Background(), graph1)
		require.NoError(t, err)
		err = repo.Save(context.Background(), graph2)
		require.NoError(t, err)
		err = repo.Save(context.Background(), graph3)
		require.NoError(t, err)

		// Delete all versions
		err = repo.DeleteByContract(context.Background(), "contract-1")

		require.NoError(t, err)
		assert.NotContains(t, mock.storage, "contract_graph:contract-1:v1")
		assert.NotContains(t, mock.storage, "contract_graph:contract-1:v2")
		assert.NotContains(t, mock.storage, "contract_graph:contract-1:v3")
	})

	t.Run("does not delete other contracts", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		// Save graphs for different contracts
		graph1 := &domain.ContractGraph{ContractID: "contract-1", Version: 1}
		graph2 := &domain.ContractGraph{ContractID: "contract-2", Version: 1}

		err := repo.Save(context.Background(), graph1)
		require.NoError(t, err)
		err = repo.Save(context.Background(), graph2)
		require.NoError(t, err)

		// Delete only contract-1
		err = repo.DeleteByContract(context.Background(), "contract-1")

		require.NoError(t, err)
		assert.NotContains(t, mock.storage, "contract_graph:contract-1:v1")
		assert.Contains(t, mock.storage, "contract_graph:contract-2:v1")
	})

	t.Run("returns error when valkey fails", func(t *testing.T) {
		mock := NewMockValkeyService()
		mock.SetError(errors.New("valkey error"))
		repo := NewContractGraphRepository(mock, 7200)

		err := repo.DeleteByContract(context.Background(), "contract-1")

		assert.Error(t, err)
	})
}

func TestContractGraphRepository_KeyFormat(t *testing.T) {
	t.Run("generates correct key format", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		graph := &domain.ContractGraph{
			ContractID: "my-contract-123",
			Version:    5,
		}

		err := repo.Save(context.Background(), graph)
		require.NoError(t, err)

		assert.Contains(t, mock.storage, "contract_graph:my-contract-123:v5")
	})
}

func TestContractGraphRepository_MultipleVersions(t *testing.T) {
	t.Run("handles multiple versions independently", func(t *testing.T) {
		mock := NewMockValkeyService()
		repo := NewContractGraphRepository(mock, 7200)

		// Save different versions
		graph1 := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    1,
			Graph: domain.Graph{
				FinalStep: "step-a",
			},
		}
		graph2 := &domain.ContractGraph{
			ContractID: "contract-1",
			Version:    2,
			Graph: domain.Graph{
				FinalStep: "step-b",
			},
		}

		err := repo.Save(context.Background(), graph1)
		require.NoError(t, err)
		err = repo.Save(context.Background(), graph2)
		require.NoError(t, err)

		// Retrieve both versions
		retrieved1, err := repo.Get(context.Background(), "contract-1", 1)
		require.NoError(t, err)
		assert.Equal(t, "step-a", retrieved1.Graph.FinalStep)

		retrieved2, err := repo.Get(context.Background(), "contract-1", 2)
		require.NoError(t, err)
		assert.Equal(t, "step-b", retrieved2.Graph.FinalStep)
	})
}
