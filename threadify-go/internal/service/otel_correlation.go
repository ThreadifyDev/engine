package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	shderrors "threadify-go/shared/errors"
)

type otelWorkflowOption struct{}

// WithOTelWorkflowRunID controls only the workflow fallback, never explicit references.
func WithOTelWorkflowRunID(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, otelWorkflowOption{}, enabled)
}

// OTelUseWorkflowRunID defaults to enabled for existing exporter configurations.
func OTelUseWorkflowRunID(ctx context.Context) bool {
	enabled, ok := ctx.Value(otelWorkflowOption{}).(bool)
	return !ok || enabled
}

// threadKeyCorrelationID keeps arbitrary caller references out of trace-ID and Redis key namespaces.
func threadKeyCorrelationID(ref string) string {
	sum := sha256.Sum256([]byte(ref))
	return "ref:" + hex.EncodeToString(sum[:])
}

// correlatedThreadID preserves legacy trace IDs and survives correlation-cache expiry.
func correlatedThreadID(companyID, correlationID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(companyID+":"+correlationID)).String()
}

// threadKeyForTrace resolves a single authoritative reference before any writes.
func threadKeyForTrace(spans []otelSpanEnvelope, useWorkflow bool) (string, error) {
	keys := []string{"threadify.thread_key"}
	if useWorkflow {
		keys = append(keys, "workflow.run_id")
	}
	for _, key := range keys {
		selected := ""
		for _, envelope := range spans {
			attrs := mergedAttributes(envelope.resourceAttrs, envelope.span.GetAttributes())
			value, exists := attrs[key]
			if !exists {
				continue
			}
			text, ok := value.GetValue().(*commonpb.AnyValue_StringValue)
			if !ok {
				return "", fmt.Errorf("%s must be a string", key)
			}
			ref := strings.TrimSpace(text.StringValue)
			if len(ref) > 1024 {
				return "", fmt.Errorf("%s exceeds 1024 bytes", key)
			}
			if ref == "" {
				if key == "threadify.thread_key" {
					return "", errors.New("threadKey must be a non-empty string")
				}
				continue
			}
			if selected != "" && selected != ref {
				return "", fmt.Errorf("conflicting %s values within one trace", key)
			}
			selected = ref
		}
		if selected != "" {
			return selected, nil
		}
	}
	return "", nil
}

// bindTrace never silently moves an already accepted trace to another thread.
func (s *OTelTraceService) bindTrace(ctx context.Context, companyID, traceID, threadID string) (string, error) {
	set, err := s.correlations.SetThreadIDIfAbsent(ctx, companyID, traceID, threadID)
	if err != nil {
		return "", err
	}
	if !set {
		existing, err := s.correlations.GetThreadID(ctx, companyID, traceID)
		if err != nil {
			return "", err
		}
		if existing != threadID {
			return "", &permanentOTelError{err: errors.New("correlation conflicts with the existing trace binding")}
		}
	}
	return threadID, nil
}

// validateCorrelation checks authorization and the persisted contract, including after restarts.
func (s *OTelTraceService) validateCorrelation(ctx context.Context, id, owner, company, contract string) error {
	thread, err := s.threads.LookupThreadForIngestion(ctx, id, owner, company)
	if err != nil {
		return err
	}
	if err := writableThread(thread); err != nil {
		return &permanentOTelError{err: err}
	}
	if contract != "" {
		name, version := parseContractIdentifier(contract)
		if thread.ContractName != name || (version > 0 && (thread.ContractVersion == nil || *thread.ContractVersion != version)) {
			return &permanentOTelError{err: errors.New("contract conflicts with the existing thread binding")}
		}
	}
	return nil
}

// Terminal threads retain their key permanently and never accept further steps.
func writableThread(thread *domain.Thread) error {
	switch thread.Status {
	case domain.ThreadStatusCompleted, domain.ThreadStatusCancelled, domain.ThreadStatusClosed, domain.ThreadStatusFailed:
		return fmt.Errorf("Cannot add steps to %s thread", thread.Status)
	default:
		return nil
	}
}

func validateThreadKeyRefs(thread *domain.Thread, refs map[string]string) error {
	if key, supplied := refs["threadify.thread_key"]; supplied && key != thread.Refs["threadify.thread_key"] {
		return errors.New("threadKey cannot be changed through refs")
	}
	return nil
}

// LookupThreadForIngestion preserves storage failures instead of treating outages as permission to recreate.
func (s *ThreadService) LookupThreadForIngestion(ctx context.Context, id, owner, company string) (*domain.Thread, error) {
	thread, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, shderrors.ErrThreadNotFound
	}
	if thread.CompanyID != company {
		return nil, &permanentOTelError{err: shderrors.ErrAccessDenied}
	}
	allowed, err := s.accessService.CheckThreadAccess(ctx, id, owner, "thread.write.*", thread)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, &permanentOTelError{err: shderrors.ErrAccessDenied}
	}
	return thread, nil
}

// startSDKThread adapts the reserved SDK reference to the same atomic OTLP resolver.
func (s *OTelTraceService) startSDKThread(ctx context.Context, req *domain.StartThreadCmd, owner, company string) *domain.StartThreadResponse {
	fail := func(err error) *domain.StartThreadResponse {
		return &domain.StartThreadResponse{Action: ActionStartThread, Status: StepStatusError, Message: err.Error()}
	}
	attrs := map[string]*commonpb.AnyValue{}
	serviceName := req.ServiceName
	if serviceName == "" {
		serviceName = req.Refs["serviceName"]
	}
	for key, value := range map[string]string{"threadify.contract": req.ContractName, "threadify.role": req.Role, "threadify.label": StartThreadLabel(req), "threadify.service": serviceName} {
		attrs[key] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
	}
	key := req.ThreadKey
	if key == "" {
		key = req.Refs["threadify.thread_key"]
	}
	if key != "" || req.Action == "thread" {
		attrs["threadify.thread_key"] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: key}}
	}
	if len(req.Tags) > 0 {
		values := make([]*commonpb.AnyValue, 0, len(req.Tags))
		for _, tag := range req.Tags {
			values = append(values, &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: tag}})
		}
		attrs["threadify.tags"] = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: values}}}
	}
	envelope := otelSpanEnvelope{resourceAttrs: attrs, span: &tracepb.Span{Name: StartThreadLabel(req)}}
	ref, err := threadKeyForTrace([]otelSpanEnvelope{envelope}, true)
	if err != nil {
		return fail(err)
	}
	if ref != "" {
		attrs["threadify.thread_key"] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: ref}}
	}
	for key, value := range req.Refs {
		if key != "threadify.thread_key" && key != "otel_trace_id" {
			attrs["threadify.ref."+key] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
		}
	}
	traceID := req.Refs["otel_trace_id"]
	var id string
	if traceID == "" && ref != "" {
		id, err = s.resolveCorrelation(ctx, owner, company, threadKeyCorrelationID(ref), envelope, uint64(time.Now().UnixNano()))
	} else {
		raw, decodeErr := hex.DecodeString(traceID)
		if decodeErr != nil || !validOTelID(raw, 16) {
			return fail(errors.New("correlated SDK start requires a threadKey or valid otel_trace_id"))
		}
		id, err = s.resolveThread(ctx, owner, company, traceID, envelope, uint64(time.Now().UnixNano()))
	}
	if err != nil {
		return fail(err)
	}
	if err := s.validateCorrelation(ctx, id, owner, company, req.ContractName); err != nil {
		return fail(err)
	}
	thread, err := s.threads.LookupThreadForIngestion(ctx, id, owner, company)
	if err != nil {
		return fail(err)
	}
	if ref == "" {
		ref = thread.Refs["threadify.thread_key"]
	}
	return &domain.StartThreadResponse{
		Action: ActionStartThread, Status: StepStatusSuccess, ThreadID: id, ThreadKey: ref,
		Label: thread.Label, ContractID: thread.ContractID, ContractName: thread.ContractName,
		ContractVersion: thread.ContractVersion, Refs: thread.Refs, Tags: thread.Tags,
	}
}
