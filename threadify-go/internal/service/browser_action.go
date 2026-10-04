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
		match, err := s.matchContractInput(ctx, thread, req.Name)
		if err != nil {
			return nil, err
		}
		classification = match.classification
		switch classification {
		case "mapped_step":
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
				Action: "recordThreadEvent", ThreadID: req.ThreadID, StepName: match.step,
				Type: "browser_action", StartedAt: now, FinishedAt: now,
				Context: contextValues, Status: "success", ServiceName: "browser_auto_capture",
				IdempotencyKey: "browser-action:" + id,
			}, ownerID, companyID)
			result.MappedStep = match.step
			if step.Status == StepStatusSuccess || step.IsDuplicate {
				matchedStep, result.StepID = match.step, step.StepID
			} else {
				classification, result.Message = "mapping_rejected", step.Message
			}
		case "mapping_rejected":
			result.MappedStep, result.Message = match.step, match.message
		case "step_candidate":
			matchedStep = match.step
		case "substep":
			if completed, err := s.repo.GetCompletedSteps(ctx, req.ThreadID); err == nil && len(completed) > 0 {
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
