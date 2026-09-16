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

// externalCorrelationID keeps arbitrary caller references out of trace-ID and Redis key namespaces.
func externalCorrelationID(ref string) string {
	sum := sha256.Sum256([]byte(ref))
	return "ref:" + hex.EncodeToString(sum[:])
}

// correlatedThreadID preserves legacy trace IDs and survives correlation-cache expiry.
func correlatedThreadID(companyID, correlationID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(companyID+":"+correlationID)).String()
}

// externalRefForTrace resolves a single authoritative reference before any writes.
func externalRefForTrace(spans []otelSpanEnvelope, useWorkflow bool) (string, error) {
	keys := []string{"threadify.external_ref"}
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
	if contract != "" {
		name, version := parseContractIdentifier(contract)
		if thread.ContractName != name || (version > 0 && (thread.ContractVersion == nil || *thread.ContractVersion != version)) {
			return &permanentOTelError{err: errors.New("contract conflicts with the existing thread binding")}
		}
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
	for key, value := range map[string]string{"threadify.external_ref": req.Refs["threadify.external_ref"], "threadify.contract": req.ContractName, "threadify.role": req.Role, "threadify.label": StartThreadLabel(req), "threadify.service": serviceName} {
		attrs[key] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
	}
	if len(req.Tags) > 0 {
		values := make([]*commonpb.AnyValue, 0, len(req.Tags))
		for _, tag := range req.Tags {
			values = append(values, &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: tag}})
		}
		attrs["threadify.tags"] = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: values}}}
	}
	envelope := otelSpanEnvelope{resourceAttrs: attrs, span: &tracepb.Span{Name: StartThreadLabel(req)}}
	ref, err := externalRefForTrace([]otelSpanEnvelope{envelope}, true)
	if err != nil {
		return fail(err)
	}
	if ref == "" && req.Refs["threadify.external_ref"] != "" {
		return fail(errors.New("external reference must not be blank"))
	}
	attrs["threadify.external_ref"] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: ref}}
	for key, value := range req.Refs {
		if key != "threadify.external_ref" && key != "otel_trace_id" {
			attrs["threadify.ref."+key] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
		}
	}
	traceID := req.Refs["otel_trace_id"]
	raw, err := hex.DecodeString(traceID)
	if err != nil || !validOTelID(raw, 16) {
		return fail(errors.New("correlated SDK start requires a valid otel_trace_id"))
	}
	id, err := s.resolveThread(ctx, owner, company, traceID, envelope, uint64(time.Now().UnixNano()))
	if err != nil {
		return fail(err)
	}
	return &domain.StartThreadResponse{Action: ActionStartThread, Status: StepStatusSuccess, ThreadID: id}
}
