package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"threadify-go/shared/actionmapping"
)

// SetBrowserActionClassifier enables advisory matching of captured action names.
// Matching never completes a contract step or grants permission to perform one.
func (s *ThreadService) SetBrowserActionClassifier(classifier DecisionClassifier) {
	s.browserClassifier = classifier
}

func (s *ThreadService) SetBrowserActionMappings(store actionmapping.Store) {
	s.browserActionMappings = store
}

// ValidateActionMapping checks the authored step in the latest contract graph.
// Runtime recording checks the version pinned to each thread again.
func (s *ThreadService) ValidateActionMapping(ctx context.Context, companyID string, rule actionmapping.Rule) error {
	if rule.Version < 1 {
		return errors.New("contract version must be a positive integer")
	}
	graph, err := s.contractValidator.GetContractGraph(ctx, rule.Contract, rule.Version, companyID)
	if err != nil || graph == nil {
		return fmt.Errorf("contract %q version %d was not found", rule.Contract, rule.Version)
	}
	node, ok := graph.Graph.Nodes[rule.Step]
	if !ok || node.Type == "parallel_group" {
		return fmt.Errorf("step %q is not available in contract %q version %d", rule.Step, rule.Contract, rule.Version)
	}
	return nil
}

// RecordBrowserAction stores bounded UI action evidence separately from step state.
func (s *ThreadService) RecordBrowserAction(ctx context.Context, req *dto.BrowserActionRequest, ownerID, companyID string) (*dto.BrowserActionResponse, error) {
	if req == nil || req.ThreadID == "" || len(req.ThreadID) > 256 || strings.TrimSpace(req.Name) != req.Name || len(req.Name) < 1 || len(req.Name) > 128 ||
		strings.IndexFunc(req.Name, unicode.IsControl) >= 0 || len(req.EventType) < 1 || len(req.EventType) > 32 ||
		len(req.Path) > 256 || (req.Path != "" && !strings.HasPrefix(req.Path, "/")) || len(req.Context) > 32 ||
		len(req.EventID) > 128 || strings.IndexFunc(req.EventID, unicode.IsControl) >= 0 {
		return nil, errors.New("invalid browser action")
	}
	for key, value := range req.Context {
		if key == "" || len(key) > 128 || len(value) > 512 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
			return nil, errors.New("invalid browser action context")
		}
	}
	thread, err := s.repo.Get(ctx, req.ThreadID, domain.ThreadReadOptions{WriteBack: true})
	if err != nil || thread == nil || thread.CompanyID != companyID {
		return nil, errors.New("thread not found")
	}
	access, err := s.accessService.CheckThreadAccess(ctx, req.ThreadID, ownerID, "thread.write.*", thread)
	if err != nil || !access {
		return nil, errors.New("thread write access required")
	}
	if err := writableThread(thread); err != nil {
		return nil, err
	}
	if s.natsArchivalPublisher == nil {
		return nil, errors.New("activity recording unavailable")
	}

	classification, matchedStep, parentStep := "free_form", "", ""
	result := &dto.BrowserActionResponse{}
	if thread.ContractName != "" {
		classification = "substep"
		version := 0
		if thread.ContractVersion != nil {
			version = *thread.ContractVersion
		} else {
			contract, err := s.contractValidator.GetContractByNameAndCompany(ctx, thread.ContractName, companyID)
			if err != nil || contract == nil {
				return nil, errors.New("contract unavailable")
			}
			version = contract.LatestVersion
		}
		graph, err := s.contractValidator.GetContractGraph(ctx, thread.ContractName, version, companyID)
		if err != nil {
			return nil, errors.New("contract unavailable")
		}
		if s.browserActionMappings != nil {
			settings, err := s.browserActionMappings.Load(ctx, companyID)
			if err != nil {
				return nil, errors.New("action mappings unavailable")
			}
			if rule, ok := actionmapping.Match(settings.Rules, thread.ContractName, version, req.Name); ok {
				if node, exists := graph.Graph.Nodes[rule.Step]; exists && node.Type != "parallel_group" {
					now := time.Now().UTC().Format(time.RFC3339Nano)
					id := req.EventID
					if id == "" {
						id = uuid.NewString()
					}
					contextValues := req.Context
					if contextValues == nil {
						contextValues = map[string]string{}
					}
					step := s.HandleRecordEvent(ctx, &domain.RecordEventCmd{
						Action: "recordThreadEvent", ThreadID: req.ThreadID, StepName: rule.Step,
						Type: "browser_action", StartedAt: now, FinishedAt: now,
						Context: contextValues, Status: "success", ServiceName: "browser_auto_capture",
						IdempotencyKey: "browser-action:" + id,
					}, ownerID, companyID)
					result.MappedStep = rule.Step
					if step.Status == StepStatusSuccess {
						classification, matchedStep, result.StepID = "mapped_step", rule.Step, step.StepID
					} else {
						classification, result.Message = "mapping_rejected", step.Message
					}
				} else {
					classification, result.Message = "mapping_rejected", "mapped step is not in this thread's contract version"
				}
			}
		}
		if classification == "substep" {
			if node, ok := graph.Graph.Nodes[req.Name]; ok && node.Type != "parallel_group" {
				matchedStep = req.Name
			} else if s.browserClassifier != nil {
				if candidate, err := s.browserClassifier.MatchAction(ctx, req.Name, contractActions(graph)); err == nil {
					if node, ok := graph.Graph.Nodes[candidate]; ok && node.Type != "parallel_group" {
						matchedStep = candidate
					}
				}
			}
			if matchedStep != "" {
				classification = "step_candidate"
			} else if completed, err := s.repo.GetCompletedSteps(ctx, req.ThreadID); err == nil && len(completed) > 0 {
				parentStep, _, _ = strings.Cut(completed[len(completed)-1], ":")
			}
		}
	}
	contextJSON, err := json.Marshal(req.Context)
	if err != nil {
		return nil, errors.New("invalid browser action context")
	}
	if err := s.natsArchivalPublisher.PublishActivityLog(ctx, map[string]interface{}{
		"type": "browser_action", "threadId": req.ThreadID, "actor": ownerID,
		"name": req.Name, "eventType": req.EventType, "path": req.Path,
		"context": string(contextJSON), "classification": classification,
		"matchedStep": matchedStep, "parentStep": parentStep, "mappedStep": result.MappedStep,
		"stepId": result.StepID, "mappingMessage": result.Message, "eventId": req.EventID,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return nil, errors.New("failed to record browser action")
	}
	result.Classification = classification
	return result, nil
}
