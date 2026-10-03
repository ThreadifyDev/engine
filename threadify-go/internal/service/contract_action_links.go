package service

import (
	"context"
	"encoding/json"
	"errors"

	"go.uber.org/zap"

	"github.com/threadify/engine/internal/dto"
	"threadify-go/shared/actionmapping"
)

type ActionLinkCopyResult struct {
	Copied  int    `json:"copied"`
	Skipped int    `json:"skipped"`
	Warning string `json:"warning,omitempty"`
}

func (s *ContractService) SetActionMappings(store actionmapping.Store) {
	s.actionMappings = store
}

// Copy only links whose target steps are still present. A compare-and-set retry
// preserves concurrent edits to links for other contracts and versions.
func (s *ContractService) copyActionLinks(ctx context.Context, company, contract string, fromVersion, toVersion int, graphJSON []byte) *ActionLinkCopyResult {
	if s.actionMappings == nil {
		return nil
	}
	result := &ActionLinkCopyResult{}
	var graph dto.ContractGraphDTO
	if err := json.Unmarshal(graphJSON, &graph); err != nil {
		s.logger.Warn("decode new contract graph for action links", zap.Error(err))
		result.Warning = "Could not copy action links; open the new version to add them."
		return result
	}
	for attempt := 0; attempt < 3; attempt++ {
		settings, err := s.actionMappings.Load(ctx, company)
		if err != nil {
			s.logger.Warn("load action links for new contract version", zap.Error(err))
			result.Warning = "Could not copy action links; open the new version to add them."
			return result
		}
		copied, skipped := make([]actionmapping.Rule, 0), 0
		for _, rule := range settings.Rules {
			if rule.Contract == contract && rule.Version == toVersion {
				result.Warning = "The new version already has action links; review them before editing."
				return result
			}
			if rule.Contract != contract || rule.Version != fromVersion {
				continue
			}
			if node, ok := graph.Graph.Nodes[rule.Step]; ok && node.Type != "parallel_group" {
				rule.Version = toVersion
				copied = append(copied, rule)
			} else {
				skipped++
			}
		}
		result.Skipped = skipped
		if len(copied) == 0 {
			return result
		}
		_, err = s.actionMappings.Save(ctx, company, settings.Revision, append(settings.Rules, copied...))
		if errors.Is(err, actionmapping.ErrConflict) {
			continue
		}
		if err != nil {
			s.logger.Warn("copy action links to new contract version", zap.Error(err))
			result.Warning = "Could not copy action links; open the new version to add them."
			return result
		}
		result.Copied = len(copied)
		return result
	}
	result.Warning = "Action links changed during publication; open the new version to review them."
	return result
}
