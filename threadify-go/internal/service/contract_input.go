package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/threadify/engine/internal/domain"
	"threadify-go/shared/actionmapping"
)

type contractInputMatch struct {
	classification string
	step           string
	message        string
	exact          bool
}

// matchContractInput shares version-pinned input mappings and advisory Jev
// matching across capture sources. Only an authored mapping records a new name
// as a contract step; semantic matches remain evidence for review.
func (s *ThreadService) matchContractInput(ctx context.Context, thread *domain.Thread, name string) (contractInputMatch, error) {
	result := contractInputMatch{classification: "substep"}
	version := 0
	if thread.ContractVersion != nil {
		version = *thread.ContractVersion
	} else {
		contract, err := s.contractValidator.GetContractByNameAndCompany(ctx, thread.ContractName, thread.CompanyID)
		if err != nil || contract == nil {
			return result, errors.New("contract unavailable")
		}
		version = contract.LatestVersion
	}
	graph, err := s.contractValidator.GetContractGraph(ctx, thread.ContractName, version, thread.CompanyID)
	if err != nil || graph == nil {
		return result, errors.New("contract unavailable")
	}
	if s.browserActionMappings != nil {
		settings, err := s.browserActionMappings.Load(ctx, thread.CompanyID)
		if err != nil {
			return result, errors.New("input mappings unavailable")
		}
		if rule, ok := actionmapping.Match(settings.Rules, thread.ContractName, version, name); ok {
			result.step = rule.Step
			if node, exists := graph.Graph.Nodes[rule.Step]; exists && node.Type != "parallel_group" {
				result.classification = "mapped_step"
			} else {
				result.classification = "mapping_rejected"
				result.message = "mapped step is not in this thread's contract version"
			}
			return result, nil
		}
	}
	if node, ok := graph.Graph.Nodes[name]; ok && node.Type != "parallel_group" {
		result.classification, result.step, result.exact = "step_candidate", name, true
	} else if s.browserClassifier != nil {
		if candidate, err := s.browserClassifier.MatchAction(ctx, name, contractActions(graph)); err == nil {
			if node, ok := graph.Graph.Nodes[candidate]; ok && node.Type != "parallel_group" {
				result.classification, result.step = "step_candidate", candidate
			}
		}
	}
	return result, nil
}

func (s *ThreadService) recordTraceInput(ctx context.Context, req *domain.RecordEventCmd, thread *domain.Thread, owner, company string, inputBytes int) *domain.RecordEventResponse {
	fail := func(message string) *domain.RecordEventResponse {
		return &domain.RecordEventResponse{Action: ActionRecordThreadEvent, Status: StepStatusError, Message: message}
	}
	// Explicit step directives retain the ordinary validation and idempotency path.
	otel, _ := req.ThreadifyMetadata["otel"].(map[string]interface{})
	if explicit, _ := otel["explicit_step_name"].(bool); explicit {
		return s.recordEvent(ctx, req, owner, company, false)
	}
	match, err := s.matchContractInput(ctx, thread, req.StepName)
	if err != nil {
		return fail(err.Error())
	}
	if match.exact {
		return s.recordEvent(ctx, req, owner, company, false)
	}
	if err := writableThread(thread); err != nil {
		return fail(err.Error())
	}
	if s.natsArchivalPublisher == nil {
		return fail("activity recording unavailable")
	}

	name := req.StepName
	response := &domain.RecordEventResponse{Action: ActionRecordThreadEvent, Status: StepStatusSuccess, ThreadID: req.ThreadID}
	mappedStep, matchedStep, parentStep := "", "", ""
	if match.classification == "mapped_step" {
		copyReq := *req
		copyReq.StepName = match.step
		response = s.recordEvent(ctx, &copyReq, owner, company, false)
		mappedStep = match.step
		if response.Status == StepStatusSuccess || response.IsDuplicate {
			matchedStep = match.step
		} else {
			match.classification, match.message = "mapping_rejected", response.Message
		}
	} else if match.classification == "mapping_rejected" {
		mappedStep = match.step
		response = fail(match.message)
	} else if match.classification == "step_candidate" {
		matchedStep = match.step
	} else if completed, err := s.repo.GetCompletedSteps(ctx, req.ThreadID); err == nil && len(completed) > 0 {
		parentStep, _, _ = strings.Cut(completed[len(completed)-1], ":")
	}
	contextJSON, err := json.Marshal(req.Context)
	if err != nil {
		return fail("invalid trace input context")
	}
	if match.classification == "step_candidate" || match.classification == "substep" {
		if err := s.planService.DecrementIngress(ctx, company, int64(inputBytes)); err != nil {
			return fail("Threadify license unavailable: " + err.Error())
		}
	}
	if err := s.natsArchivalPublisher.PublishActivityLog(ctx, map[string]interface{}{
		"type": "trace_input", "threadId": req.ThreadID, "actor": owner,
		"name": name, "eventType": "otel_span", "eventId": req.IdempotencyKey,
		"context": string(contextJSON), "classification": match.classification,
		"matchedStep": matchedStep, "mappedStep": mappedStep, "parentStep": parentStep,
		"stepId": response.StepID, "mappingMessage": match.message,
		"startedAt": req.StartedAt, "finishedAt": req.FinishedAt, "status": req.Status,
		"serviceName": req.ServiceName, "refs": req.Refs, "metadata": req.ThreadifyMetadata,
		"subSteps": req.SubSteps, "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return fail("failed to record trace input")
	}
	return response
}
