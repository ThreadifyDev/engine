package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/pkg/validator"
)

func TestCompositionAllowsAlternativeTerminalEntries(t *testing.T) {
	contract := &validator.Contract{Steps: []validator.Step{{ID: "approved"}, {ID: "rejected"}}}
	graph := &domain.ContractGraph{Graph: domain.Graph{
		EntryPoints: []string{"approved", "rejected"}, TerminalSteps: []string{"approved", "rejected"},
	}}
	require.Empty(t, validateComposition(contract, graph))
}

func TestEquivalentContractDurations(t *testing.T) {
	require.True(t, sameContractDuration("1m", "60s"))
	require.True(t, sameContractDuration("0.5d", "12h"))
	require.False(t, sameContractDuration("1m", "61s"))
}
