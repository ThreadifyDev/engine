package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
)

func (s *ThreadService) HandleWaitFor(ctx context.Context, req domain.WaitRequest, owner, company string) *domain.WaitResult {
	if waitContextDone(ctx) {
		return waitContextResult(ctx, req)
	}
	reply := &domain.WaitResult{Action: "waitFor", Status: "success", ThreadID: req.ThreadID, StepName: req.StepName, StepID: req.StepID, InvocationID: req.InvocationID, Decision: "denied"}
	deny := func(message string) *domain.WaitResult { reply.Message = message; return reply }
	if owner == "" || company == "" || !s.connectionMgr.IsConnected(owner) {
		return deny("Not authenticated")
	}
	if req.ThreadID == "" || req.StepName == "" {
		return deny("Thread and step are required")
	}
	thread, err := s.getThread(req.ThreadID)
	if err != nil {
		return deny("Thread unavailable")
	}
	access, err := s.accessService.CheckThreadAccess(ctx, req.ThreadID, owner, "thread.write.*", thread)
	if err != nil || !access {
		return deny("Thread write access required")
	}
	if s.waitRepo == nil {
		reply.Decision = "unavailable"
		return deny("Wait service unavailable")
	}
	if req.StepID != "" {
		if _, err := uuid.Parse(req.StepID); err != nil {
			return deny("Invalid step event ID")
		}
		result, err := s.waitRepo.Result(ctx, req.ThreadID, req.StepID)
		if err != nil {
			reply.Decision = "unavailable"
			return deny(err.Error())
		}
		if result.StepName != req.StepName {
			return deny("Event does not belong to this step")
		}
		return result
	}
	if _, err := uuid.Parse(req.InvocationID); err != nil {
		return deny("A UUID invocationId is required")
	}
	if thread.ContractName == "" {
		return deny("Permission requires a contract")
	}
	version := 0
	if thread.ContractVersion != nil {
		version = *thread.ContractVersion
	}
	graph, err := s.contractValidator.GetContractGraph(ctx, thread.ContractName, version, thread.CompanyID)
	if err != nil {
		reply.Decision = "unavailable"
		return deny("Contract unavailable")
	}
	node, ok := graph.Graph.Nodes[req.StepName]
	if !ok || node.Type == "parallel_group" {
		return deny("Step not defined in contract")
	}
	if node.Owner != "" {
		hasRole, err := s.accessService.ValidateUserRoleForStep(ctx, thread.ID, owner, node.Owner)
		if err != nil || !hasRole {
			return deny("Step owner role required")
		}
	}
	if !req.Cancel {
		if thread.Status != domain.ThreadStatusActive {
			return deny("Thread is not active")
		}
		if violation := s.validationService.CheckMaxDuration(thread, graph, time.Now()); violation != nil {
			return deny(violation.Message)
		}
	}
	result, err := s.waitRepo.Claim(ctx, req, owner, graph)
	if err != nil {
		reply.Decision = "unavailable"
		return deny("Live validation state unavailable")
	}
	return result
}

// Network deadlines may fire before the context timer goroutine is scheduled.
func waitContextDone(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	deadline, ok := ctx.Deadline()
	return ok && !time.Now().Before(deadline)
}

func waitContextResult(ctx context.Context, req domain.WaitRequest) *domain.WaitResult {
	decision, message := "cancelled", "Wait cancelled"
	deadline, hasDeadline := ctx.Deadline()
	if ctx.Err() == context.DeadlineExceeded || hasDeadline && !time.Now().Before(deadline) {
		decision, message = "timed_out", "Timed out waiting for Threadify"
	}
	return &domain.WaitResult{Action: "waitFor", Status: "success", ThreadID: req.ThreadID, StepName: req.StepName, StepID: req.StepID, InvocationID: req.InvocationID, Decision: decision, Message: message}
}

// AwaitWaitFor is only called after the initial authenticated check. State stays
// authoritative in Valkey; notifications merely tell us when to check it again.
func (s *ThreadService) AwaitWaitFor(ctx context.Context, req domain.WaitRequest, owner, company string) *domain.WaitResult {
	if waitContextDone(ctx) {
		return waitContextResult(ctx, req)
	}
	if s.waitRepo == nil {
		return s.HandleWaitFor(ctx, req, owner, company)
	}
	// The final result may already exist by the time the response is deferred.
	if result := s.HandleWaitFor(ctx, req, owner, company); result.Decision != "pending" {
		return result
	}
	changes, closeWatch, err := s.waitRepo.Watch(ctx, req.ThreadID)
	if err != nil {
		if waitContextDone(ctx) {
			return waitContextResult(ctx, req)
		}
		return &domain.WaitResult{Action: "waitFor", Status: "success", ThreadID: req.ThreadID, StepName: req.StepName, StepID: req.StepID, InvocationID: req.InvocationID, Decision: "unavailable", Message: "Validation notification channel unavailable"}
	}
	defer closeWatch()
	// Expiry is a one-shot deadline, not a polling interval.
	var expiry <-chan time.Time
	if req.StepID == "" {
		if thread, err := s.getThread(req.ThreadID); err == nil && thread.ContractName != "" {
			version := 0
			if thread.ContractVersion != nil {
				version = *thread.ContractVersion
			}
			if graph, err := s.contractValidator.GetContractGraph(ctx, thread.ContractName, version, thread.CompanyID); err == nil && graph.Validation != nil {
				if duration, err := time.ParseDuration(graph.Validation.MaxDuration); err == nil {
					timer := time.NewTimer(time.Until(thread.StartedAt.Add(duration)) + time.Nanosecond)
					defer timer.Stop()
					expiry = timer.C
				}
			}
		}
	}
	for {
		if waitContextDone(ctx) {
			return waitContextResult(ctx, req)
		}
		result := s.HandleWaitFor(ctx, req, owner, company)
		if result.Decision != "pending" {
			return result
		}
		select {
		case <-ctx.Done():
			return waitContextResult(ctx, req)
		case _, ok := <-changes:
			if !ok {
				if waitContextDone(ctx) {
					return waitContextResult(ctx, req)
				}
				result.Decision = "unavailable"
				result.Message = "Validation notification channel closed"
				return result
			}
		case <-expiry:
			expiry = nil
		}
	}
}
