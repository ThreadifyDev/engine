package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/threadify/engine/internal/dto"
	"threadify-go/shared/actionmapping"
)

type actionLinkStore struct {
	settings  actionmapping.Settings
	conflicts int
	err       error
}

func (s *actionLinkStore) Load(context.Context, string) (actionmapping.Settings, error) {
	return s.settings, s.err
}

func (s *actionLinkStore) Save(_ context.Context, _ string, revision string, rules []actionmapping.Rule) (actionmapping.Settings, error) {
	if s.err != nil {
		return actionmapping.Settings{}, s.err
	}
	if s.conflicts > 0 {
		s.conflicts--
		return actionmapping.Settings{}, actionmapping.ErrConflict
	}
	if revision != s.settings.Revision {
		return actionmapping.Settings{}, actionmapping.ErrConflict
	}
	s.settings = actionmapping.Settings{Rules: rules, Revision: "next"}
	return s.settings, nil
}

func TestCopyActionLinksToNewContractVersion(t *testing.T) {
	graph, err := json.Marshal(dto.ContractGraphDTO{Graph: dto.GraphDTO{Nodes: map[string]dto.GraphNode{
		"checkout": {Type: "step"}, "group": {Type: "parallel_group"},
	}}})
	require.NoError(t, err)
	store := &actionLinkStore{settings: actionmapping.Settings{Revision: "old", Rules: []actionmapping.Rule{
		{Action: "confirm", Contract: "orders", Version: 1, Step: "checkout"},
		{Action: "cancel", Contract: "orders", Version: 1, Step: "removed"},
		{Action: "group_action", Contract: "orders", Version: 1, Step: "group"},
		{Action: "confirm", Contract: "another", Version: 1, Step: "checkout"},
	}}, conflicts: 1}
	svc := &ContractService{actionMappings: store, logger: zap.NewNop()}
	result := svc.copyActionLinks(context.Background(), "company", "orders", 1, 2, graph)
	require.Equal(t, &ActionLinkCopyResult{Copied: 1, Skipped: 2}, result)
	require.Len(t, store.settings.Rules, 5)
	require.Equal(t, actionmapping.Rule{Action: "confirm", Contract: "orders", Version: 2, Step: "checkout"}, store.settings.Rules[4])

	again := svc.copyActionLinks(context.Background(), "company", "orders", 1, 2, graph)
	require.Zero(t, again.Copied)
	require.NotEmpty(t, again.Warning)
	require.Len(t, store.settings.Rules, 5)
}

func TestCopyActionLinksReportsStorageFailure(t *testing.T) {
	svc := &ContractService{actionMappings: &actionLinkStore{err: errors.New("offline")}, logger: zap.NewNop()}
	result := svc.copyActionLinks(context.Background(), "company", "orders", 1, 2, []byte(`{"graph":{"nodes":{}}}`))
	require.NotEmpty(t, result.Warning)
	require.Zero(t, result.Copied)
}
