package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/graphql/scalars"
	"github.com/threadify/engine/internal/service"
)

type decisionFacts struct {
	thread     *domain.Thread
	graph      *domain.ContractGraph
	successful []string
	ownerID    string
}

// loadDecisionFacts uses the same contract version and validated success facts
// as the existing proposal query. A missing store is an error, never an empty
// history that could authorize an entry action.
func (r *queryResolver) loadDecisionFacts(ctx context.Context, threadID string) (*decisionFacts, error) {
	auth, err := getUserInfoFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}
	thread, err := r.threadRepo.GetThreadWithPermissionCheck(ctx, threadID, auth.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("thread unavailable: %w", err)
	}
	access, err := r.threadAccessService.CheckThreadAccess(ctx, threadID, auth.OwnerID, "thread.write.*", thread)
	if err != nil || !access {
		return nil, fmt.Errorf("thread write access required")
	}
	if thread.ContractName == "" || thread.ContractVersion == nil {
		return nil, fmt.Errorf("thread has no contract")
	}
	graph, err := r.contractValidator.GetContractGraph(ctx, thread.ContractName, *thread.ContractVersion, auth.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("contract unavailable: %w", err)
	}
	steps, err := r.stepStatePostgres.GetStepsWithPermissionCheck(ctx, threadID, auth.CompanyID, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("successful step history unavailable: %w", err)
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].LastUpdatedAt.Before(steps[j].LastUpdatedAt) })
	successful := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Status == "success" || step.Status == "completed" {
			successful = append(successful, step.StepName)
		}
	}
	hot, err := r.threadRepo.GetCompletedSteps(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("live successful step history unavailable: %w", err)
	}
	for _, key := range hot {
		name, _, _ := strings.Cut(key, ":")
		if name != "" {
			successful = append(successful, name)
		}
	}
	return &decisionFacts{thread: thread, graph: graph, successful: successful, ownerID: auth.OwnerID}, nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (r *queryResolver) evaluateCan(ctx context.Context, threadID string, action, goal *string, candidate scalars.JSON) (*domain.CanDecision, error) {
	facts, err := r.loadDecisionFacts(ctx, threadID)
	if err != nil {
		return nil, err
	}
	actionID, matchedBy, err := service.ResolveAction(ctx, facts.graph, optionalString(action), optionalString(goal), r.classifier)
	if err != nil {
		if goal != nil && optionalString(goal) != "" {
			return &domain.CanDecision{ThreadID: threadID, StepName: "", Status: "uncertain", MatchedBy: "none", Reason: err.Error(), RequiredSteps: []string{}, SatisfiedSteps: []string{}, MissingSteps: []string{}}, nil
		}
		return nil, err
	}
	decision, err := service.EvaluateStepProposal(threadID, facts.thread.Status, facts.graph, actionID, facts.successful)
	if err != nil {
		return nil, err
	}
	decision.MatchedBy = matchedBy
	if !decision.Allowed {
		return decision, nil
	}
	if rule := facts.graph.Validation; rule != nil && rule.MaxDuration != "" {
		if duration, err := time.ParseDuration(rule.MaxDuration); err == nil && time.Since(facts.thread.StartedAt) > duration {
			decision.Allowed, decision.Status, decision.Reason = false, "denied", "thread maximum duration exceeded"
			return decision, nil
		}
	}
	node := facts.graph.Graph.Nodes[actionID]
	if node.Owner != "" {
		ok, err := r.threadAccessService.ValidateUserRoleForStep(ctx, threadID, facts.ownerID, node.Owner)
		if err != nil || !ok {
			decision.Allowed, decision.Status, decision.Reason = false, "denied", "step owner role required"
			return decision, nil
		}
	}
	if candidate != nil {
		values, err := stringContext(candidate)
		if err != nil {
			return nil, err
		}
		if err := r.contractValidator.ValidateStepContext(ctx, node, values, threadID); err != nil {
			decision.Allowed, decision.Status, decision.Reason = false, "denied", err.Error()
		}
	}
	return decision, nil
}

func stringContext(value scalars.JSON) (map[string]string, error) {
	input, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("context must be a JSON object")
	}
	if len(input) > 128 {
		return nil, fmt.Errorf("context has too many fields")
	}
	result := make(map[string]string, len(input))
	for key, raw := range input {
		switch v := raw.(type) {
		case string, float64, bool:
			result[key] = fmt.Sprint(v)
		default:
			return nil, fmt.Errorf("context field %q must be a string, number, or boolean", key)
		}
		if len(result[key]) > 4096 {
			return nil, fmt.Errorf("context field %q is too long", key)
		}
	}
	return result, nil
}

func (r *queryResolver) evaluateShould(ctx context.Context, threadID string, action, goal *string) (*domain.ShouldDecision, error) {
	can, err := r.evaluateCan(ctx, threadID, action, goal, nil)
	if err != nil {
		return nil, err
	}
	result := &domain.ShouldDecision{ThreadID: threadID, StepName: can.StepName, Eligible: can.Allowed, Recommendation: "unavailable", Reason: can.Reason}
	if !can.Allowed {
		result.Recommendation = "no"
		return result, nil
	}
	if r.classifier == nil {
		result.Reason = "classifier is not configured; action is contract-eligible"
		return result, nil
	}
	facts, err := r.loadDecisionFacts(ctx, threadID)
	if err != nil {
		return nil, err
	}
	snapshot, err := r.decisionSnapshot(ctx, facts)
	if err != nil {
		result.Reason = "validated context unavailable; action remains contract-eligible"
		return result, nil
	}
	recommendation, reason, err := r.classifier.Recommend(ctx, can.StepName, snapshot)
	if err != nil {
		result.Reason = "classifier unavailable; action remains contract-eligible"
		return result, nil
	}
	result.Recommendation, result.Reason = recommendation, reason
	return result, nil
}

func (r *queryResolver) decisionSnapshot(ctx context.Context, facts *decisionFacts) (service.DecisionSnapshot, error) {
	snapshot := service.DecisionSnapshot{
		SuccessfulSteps: append([]string(nil), facts.successful...),
		StepContext:     make(map[string]map[string]string),
		ThreadRefs:      make(map[string]string),
	}
	for key, value := range facts.thread.Refs {
		if len(snapshot.ThreadRefs) >= 32 {
			break
		}
		if strings.HasPrefix(key, "threadify.") || key == "otel_trace_id" || len(key) > 128 || len(value) > 512 {
			continue
		}
		snapshot.ThreadRefs[key] = value
	}
	reader, ok := r.contractValidator.(interface {
		ValidatedStepContent(context.Context, string, string) (map[string]string, error)
	})
	if !ok {
		return snapshot, fmt.Errorf("validated content reader unavailable")
	}
	seen := make(map[string]bool)
	for i := len(facts.successful) - 1; i >= 0 && len(snapshot.StepContext) < 12; i-- {
		step := facts.successful[i]
		if seen[step] {
			continue
		}
		seen[step] = true
		content, err := reader.ValidatedStepContent(ctx, facts.thread.ID, step)
		if err != nil {
			return snapshot, err
		}
		var declared map[string]bool
		if node, ok := facts.graph.Graph.Nodes[step]; ok && node.BusinessContext != nil {
			declared = make(map[string]bool, len(node.BusinessContext.Required)+len(node.BusinessContext.Optional))
			for _, field := range node.BusinessContext.Required {
				declared[field] = true
			}
			for _, field := range node.BusinessContext.Optional {
				declared[field] = true
			}
		}
		bounded := make(map[string]string)
		for field, value := range content {
			if len(bounded) >= 32 {
				break
			}
			if declared != nil && !declared[field] {
				continue
			}
			if len(field) <= 128 && len(value) <= 512 {
				bounded[field] = value
			}
		}
		snapshot.StepContext[step] = bounded
	}
	return snapshot, nil
}

func (r *queryResolver) evaluateNext(ctx context.Context, threadID string) (*domain.NextDecision, error) {
	facts, err := r.loadDecisionFacts(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if rule := facts.graph.Validation; rule != nil && rule.MaxDuration != "" {
		if duration, err := time.ParseDuration(rule.MaxDuration); err == nil && time.Since(facts.thread.StartedAt) > duration {
			return &domain.NextDecision{ThreadID: threadID, Paths: []*domain.NextPath{}}, nil
		}
	}
	paths, err := service.EvaluateNextPaths(threadID, facts.thread.Status, facts.graph, facts.successful)
	if err != nil {
		return nil, err
	}
	filtered := make([]*domain.NextPath, 0, len(paths))
	for _, path := range paths {
		node := facts.graph.Graph.Nodes[path.Actions[0]]
		if node.Owner != "" {
			ok, err := r.threadAccessService.ValidateUserRoleForStep(ctx, threadID, facts.ownerID, node.Owner)
			if err != nil || !ok {
				continue
			}
		}
		filtered = append(filtered, path)
	}
	return &domain.NextDecision{ThreadID: threadID, Paths: filtered}, nil
}
