package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/threadify/engine/internal/domain"
)

// DecisionClassifier can interpret an action goal and advise on an eligible
// action. Its output never bypasses contract eligibility.
type DecisionClassifier interface {
	MatchAction(context.Context, string, []string) (string, error)
	Recommend(context.Context, string, DecisionSnapshot) (string, string, error)
}

// DecisionSnapshot contains bounded, validated process facts for advice.
type DecisionSnapshot struct {
	SuccessfulSteps []string                     `json:"successful_steps"`
	StepContext     map[string]map[string]string `json:"step_context,omitempty"`
	ThreadRefs      map[string]string            `json:"thread_refs,omitempty"`
}

// ResolveAction accepts exact authored action IDs without a model. Natural
// language is matched only against the IDs in the effective contract.
func ResolveAction(ctx context.Context, graph *domain.ContractGraph, action, goal string, classifier DecisionClassifier) (string, string, error) {
	if graph == nil {
		return "", "", fmt.Errorf("contract graph is required")
	}
	if (strings.TrimSpace(action) == "") == (strings.TrimSpace(goal) == "") {
		return "", "", fmt.Errorf("provide exactly one of action or goal")
	}
	actions := contractActions(graph)
	if action != "" {
		if _, exists := graph.Graph.Nodes[action]; !exists || !containsAction(actions, action) {
			return "", "", fmt.Errorf("action %q is not defined in the contract", action)
		}
		return action, "exact", nil
	}
	goal = strings.TrimSpace(goal)
	if len(goal) > 4096 {
		return "", "", fmt.Errorf("goal must be at most 4096 characters")
	}
	if containsAction(actions, goal) {
		return goal, "exact", nil
	}
	if classifier == nil {
		return "", "", fmt.Errorf("classifier is not configured for natural-language goals")
	}
	matched, err := classifier.MatchAction(ctx, goal, actions)
	if err != nil {
		return "", "", fmt.Errorf("action classification unavailable: %w", err)
	}
	if !containsAction(actions, matched) {
		return "", "", fmt.Errorf("goal did not identify one contract action")
	}
	return matched, "classifier", nil
}

func contractActions(graph *domain.ContractGraph) []string {
	actions := make([]string, 0, len(graph.Graph.Nodes))
	for id, node := range graph.Graph.Nodes {
		if node.Type != "parallel_group" {
			actions = append(actions, id)
		}
	}
	sort.Strings(actions)
	return actions
}

func containsAction(actions []string, target string) bool {
	return slices.Contains(actions, target)
}

func EvaluateNextPaths(threadID string, status domain.ThreadStatus, graph *domain.ContractGraph, successful []string) ([]*domain.NextPath, error) {
	if graph == nil {
		return nil, fmt.Errorf("contract graph is required")
	}
	paths := make([]*domain.NextPath, 0)
	for _, action := range contractActions(graph) {
		decision, err := EvaluateStepProposal(threadID, status, graph, action, successful)
		if err != nil {
			return nil, err
		}
		if !decision.Allowed && decision.Status != "requires_claim" {
			continue
		}
		// Bound the preview: contracts may contain cycles and parallel branches.
		appendPaths(threadID, status, graph, successful, []string{action}, decision.Status, &paths, 4, 32)
		if len(paths) >= 32 {
			break
		}
	}
	return paths, nil
}

func appendPaths(threadID string, threadStatus domain.ThreadStatus, graph *domain.ContractGraph, successful, path []string, status string, out *[]*domain.NextPath, maxDepth, maxPaths int) {
	if len(*out) >= maxPaths {
		return
	}
	next := append([]string(nil), graph.Graph.Nodes[path[len(path)-1]].Next...)
	sort.Strings(next)
	if len(path) >= maxDepth || len(next) == 0 {
		*out = append(*out, newNextPath(path, status))
		return
	}
	added := false
	for _, candidate := range next {
		node, ok := graph.Graph.Nodes[candidate]
		if !ok || node.Type == "parallel_group" || containsPath(path, candidate) {
			continue
		}
		futureSuccessful := append(append([]string(nil), successful...), path...)
		decision, err := EvaluateStepProposal(threadID, threadStatus, graph, candidate, futureSuccessful)
		if err != nil || (!decision.Allowed && decision.Status != "requires_claim") {
			continue
		}
		appendPaths(threadID, threadStatus, graph, successful, append(append([]string(nil), path...), candidate), status, out, maxDepth, maxPaths)
		added = true
		if len(*out) >= maxPaths {
			return
		}
	}
	if !added {
		*out = append(*out, newNextPath(path, status))
	}
}

func newNextPath(path []string, status string) *domain.NextPath {
	reason := "first action is eligible at this snapshot; later actions are conditional"
	if status == "requires_claim" {
		reason = "first action requires an atomic waitFor claim; later actions are conditional"
	}
	return &domain.NextPath{Actions: append([]string(nil), path...), Status: status, Reason: reason}
}

func containsPath(path []string, target string) bool {
	for _, action := range path {
		if action == target {
			return true
		}
	}
	return false
}
