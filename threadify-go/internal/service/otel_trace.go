package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	collecttracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap"

	"github.com/threadify/engine/internal/domain"
)

const otelCorrelationWait = 2 * time.Second

var errOTelCorrelationTimeout = errors.New("timed out waiting for trace correlation")

type permanentOTelError struct {
	err error
}

func (e *permanentOTelError) Error() string { return e.err.Error() }

// OTelTraceService translates OTLP traces into the existing Threadify write model.
type OTelTraceService struct {
	threads      domain.OTelThreadWriter
	correlations domain.OTelTraceCorrelationRepository
	logger       *zap.Logger
}

type otelSpanEnvelope struct {
	span              *tracepb.Span
	resourceAttrs     map[string]*commonpb.AnyValue
	resourceSchemaURL string
	scope             *commonpb.InstrumentationScope
	scopeSchemaURL    string
}

func NewOTelTraceService(
	threads domain.OTelThreadWriter,
	correlations domain.OTelTraceCorrelationRepository,
	logger *zap.Logger,
) *OTelTraceService {
	return &OTelTraceService{threads: threads, correlations: correlations, logger: logger}
}

// Ingest accepts a decoded OTLP request and returns the protocol-level response.
// Invalid or rejected spans are reported using OTLP partial-success semantics.
func (s *OTelTraceService) Ingest(
	ctx context.Context,
	req *collecttracepb.ExportTraceServiceRequest,
	ownerID, companyID string,
) (*collecttracepb.ExportTraceServiceResponse, error) {
	groups, rejected, messages := groupOTelSpans(req)

	for traceID, spans := range groups {
		traceRejected, traceMessages, err := s.ingestTrace(ctx, ownerID, companyID, traceID, spans)
		if err != nil {
			return nil, err
		}
		rejected += traceRejected
		messages = append(messages, traceMessages...)
	}

	resp := &collecttracepb.ExportTraceServiceResponse{}
	if rejected > 0 {
		resp.PartialSuccess = &collecttracepb.ExportTracePartialSuccess{
			RejectedSpans: rejected,
			ErrorMessage:  otelErrorMessage(messages),
		}
	}
	return resp, nil
}

func groupOTelSpans(req *collecttracepb.ExportTraceServiceRequest) (map[string][]otelSpanEnvelope, int64, []string) {
	groups := make(map[string][]otelSpanEnvelope)
	var rejected int64
	var messages []string
	if req == nil {
		return groups, 0, nil
	}

	for _, resourceSpans := range req.GetResourceSpans() {
		resourceAttrs := keyValueMap(resourceSpans.GetResource().GetAttributes())
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				if err := validateOTelSpan(span); err != nil {
					rejected++
					messages = append(messages, err.Error())
					continue
				}

				traceID := hex.EncodeToString(span.GetTraceId())
				groups[traceID] = append(groups[traceID], otelSpanEnvelope{
					span:              span,
					resourceAttrs:     resourceAttrs,
					resourceSchemaURL: resourceSpans.GetSchemaUrl(),
					scope:             scopeSpans.GetScope(),
					scopeSchemaURL:    scopeSpans.GetSchemaUrl(),
				})
			}
		}
	}
	return groups, rejected, messages
}

func (s *OTelTraceService) ingestTrace(
	ctx context.Context,
	ownerID, companyID, traceID string,
	spans []otelSpanEnvelope,
) (int64, []string, error) {
	if err := validateTraceDirectives(spans); err != nil {
		return int64(len(spans)), []string{fmt.Sprintf("trace %s: %v", traceID, err)}, nil
	}

	// Preserve workflow order as closely as OTLP allows. Export batches are not
	// guaranteed to be ordered, but start time is a better contract progression
	// signal than protobuf nesting or span end order.
	sort.SliceStable(spans, func(i, j int) bool {
		return spans[i].span.GetStartTimeUnixNano() < spans[j].span.GetStartTimeUnixNano()
	})

	threadDescriptor := spans[0]
	for _, candidate := range spans {
		if len(candidate.span.GetParentSpanId()) == 0 {
			threadDescriptor = candidate
			break
		}
	}

	threadID, err := s.resolveThread(
		ctx,
		ownerID,
		companyID,
		traceID,
		threadDescriptor,
		spans[0].span.GetStartTimeUnixNano(),
	)
	if err != nil {
		var permanent *permanentOTelError
		if errors.As(err, &permanent) {
			return int64(len(spans)), []string{fmt.Sprintf("trace %s: %v", traceID, err)}, nil
		}
		return 0, nil, err
	}

	var rejected int64
	var messages []string
	for _, envelope := range spans {
		accepted, message, err := s.recordSpan(ctx, ownerID, companyID, traceID, threadID, envelope)
		if err != nil {
			return 0, nil, err
		}
		if !accepted {
			rejected++
			messages = append(messages, message)
			continue
		}
	}
	// OTLP exports only ended spans. Root completion is a run boundary, not
	// proof that every distributed child has arrived. Late spans remain valid.
	// A marker supports roots hidden behind an upstream HTTP/server span.
	if rejected == 0 {
		var endedAt uint64
		for _, envelope := range spans {
			if len(envelope.span.GetParentSpanId()) == 0 || attributeString(keyValueMap(envelope.span.GetAttributes()), "threadify.run.complete") == "true" {
				if end := envelope.span.GetEndTimeUnixNano(); end > endedAt {
					endedAt = end
				}
			}
		}
		if endedAt != 0 {
			if err := s.threads.CompleteTraceForIngestion(ctx, threadID, ownerID, companyID, traceID, time.Unix(0, int64(endedAt)).UTC()); err != nil {
				return 0, nil, err
			}
		}
	}
	return rejected, messages, nil
}

func (s *OTelTraceService) resolveThread(
	ctx context.Context,
	ownerID, companyID, traceID string,
	descriptor otelSpanEnvelope,
	traceStartedAt uint64,
) (string, error) {
	attrs := mergedAttributes(descriptor.resourceAttrs, descriptor.span.GetAttributes())
	explicitThreadID := attributeString(attrs, "threadify.thread_id")

	existing, err := s.correlations.GetThreadID(ctx, companyID, traceID)
	if err != nil {
		return "", err
	}
	if existing != "" {
		if explicitThreadID != "" && explicitThreadID != existing {
			return "", &permanentOTelError{err: errors.New("threadify.thread_id conflicts with the existing trace correlation")}
		}
		return existing, nil
	}

	if explicitThreadID != "" {
		if err := s.threads.ValidateThreadForIngestion(ctx, explicitThreadID, ownerID, companyID); err != nil {
			return "", &permanentOTelError{err: fmt.Errorf("invalid threadify.thread_id: %w", err)}
		}
		set, err := s.correlations.SetThreadIDIfAbsent(ctx, companyID, traceID, explicitThreadID)
		if err != nil {
			return "", err
		}
		if set {
			return explicitThreadID, nil
		}
		existing, err := s.correlations.GetThreadID(ctx, companyID, traceID)
		if err != nil {
			return "", err
		}
		if existing != explicitThreadID {
			return "", &permanentOTelError{err: errors.New("threadify.thread_id conflicts with a concurrent trace correlation")}
		}
		return existing, nil
	}

	deadline := time.NewTimer(otelCorrelationWait)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	token := uuid.NewString()

	for {
		acquired, err := s.correlations.AcquireCreationLock(ctx, companyID, traceID, token)
		if err != nil {
			return "", err
		}
		if acquired {
			return s.createCorrelatedThread(ctx, ownerID, companyID, traceID, token, descriptor, traceStartedAt)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			return "", errOTelCorrelationTimeout
		case <-ticker.C:
			existing, err := s.correlations.GetThreadID(ctx, companyID, traceID)
			if err != nil {
				return "", err
			}
			if existing != "" {
				return existing, nil
			}
		}
	}
}

func (s *OTelTraceService) createCorrelatedThread(
	ctx context.Context,
	ownerID, companyID, traceID, token string,
	descriptor otelSpanEnvelope,
	traceStartedAt uint64,
) (string, error) {
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.correlations.ReleaseCreationLock(releaseCtx, companyID, traceID, token); err != nil {
			s.logger.Warn("failed to release OTLP trace creation lock", zap.String("trace_id", traceID), zap.Error(err))
		}
	}()

	// Recheck under the distributed lock in case another request completed just
	// before this request acquired it.
	existing, err := s.correlations.GetThreadID(ctx, companyID, traceID)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}

	attrs := mergedAttributes(descriptor.resourceAttrs, descriptor.span.GetAttributes())
	serviceName := otelServiceName(attrs, descriptor.scope)
	contractName := attributeString(attrs, "threadify.contract")
	role := attributeString(attrs, "threadify.role")
	if contractName != "" && role == "" {
		role = strings.TrimSuffix(serviceName, "-service")
		if role == "" || role == "unknown" {
			role = "participant"
		}
	}
	label := attributeString(attrs, "threadify.label")
	if label == "" {
		label = descriptor.span.GetName()
	}

	refs := otelRefs(attrs)
	refs["otel_trace_id"] = traceID
	candidateThreadID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(companyID+":"+traceID)).String()
	resp := s.threads.StartThreadForIngestion(ctx, &domain.StartThreadCmd{
		Action:       ActionStartThread,
		ThreadID:     candidateThreadID,
		Label:        label,
		ContractName: contractName,
		Role:         role,
		ServiceName:  serviceName,
		StartedAt:    otelTimestamp(traceStartedAt),
		Refs:         refs,
		Tags:         attributeStringSlice(attrs, "threadify.tags"),
	}, ownerID, companyID)
	if resp == nil || resp.Status != StepStatusSuccess || resp.ThreadID == "" {
		// A previous process may have created the deterministic thread and exited
		// before publishing the correlation. Recover that thread before rejecting.
		if err := s.threads.ValidateThreadForIngestion(ctx, candidateThreadID, ownerID, companyID); err == nil {
			set, setErr := s.correlations.SetThreadIDIfAbsent(ctx, companyID, traceID, candidateThreadID)
			if setErr != nil {
				return "", setErr
			}
			if set {
				return candidateThreadID, nil
			}
			if existing, getErr := s.correlations.GetThreadID(ctx, companyID, traceID); getErr == nil && existing != "" {
				return existing, nil
			}
		}
		message := "failed to start Threadify thread"
		if resp != nil && resp.Message != "" {
			message = resp.Message
		}
		if isPermanentOTelStartFailure(message) {
			return "", &permanentOTelError{err: errors.New(message)}
		}
		return "", errors.New(message)
	}

	set, err := s.correlations.SetThreadIDIfAbsent(ctx, companyID, traceID, resp.ThreadID)
	if err != nil {
		return "", err
	}
	if !set {
		existing, err := s.correlations.GetThreadID(ctx, companyID, traceID)
		if err != nil {
			return "", err
		}
		if existing == "" {
			return "", errors.New("trace correlation was not available after concurrent creation")
		}
		if existing != resp.ThreadID {
			s.logger.Warn("discarding concurrently-created OTLP thread correlation",
				zap.String("trace_id", traceID),
				zap.String("created_thread_id", resp.ThreadID),
				zap.String("winning_thread_id", existing),
			)
		}
		return existing, nil
	}
	return resp.ThreadID, nil
}

func (s *OTelTraceService) recordSpan(
	ctx context.Context,
	ownerID, companyID, traceID, threadID string,
	envelope otelSpanEnvelope,
) (bool, string, error) {
	span := envelope.span
	spanID := hex.EncodeToString(span.GetSpanId())
	claimToken, duplicate, err := s.claimSpan(ctx, companyID, traceID, spanID)
	if err != nil {
		return false, "", err
	}
	if duplicate {
		return true, "", nil
	}
	claimCompleted := false
	defer func() {
		if claimCompleted {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.correlations.ReleaseSpan(releaseCtx, companyID, traceID, spanID, claimToken); err != nil {
			s.logger.Warn("failed to release OTLP span claim", zap.String("trace_id", traceID), zap.String("span_id", spanID), zap.Error(err))
		}
	}()

	attrs := mergedAttributes(envelope.resourceAttrs, span.GetAttributes())
	stepName := attributeString(keyValueMap(span.GetAttributes()), "threadify.step_name")
	if stepName == "" {
		stepName = span.GetName()
	}

	status := StepStatusSuccess
	if span.GetStatus().GetCode() == tracepb.Status_STATUS_CODE_ERROR {
		status = StepStatusFailed
	}

	refs := otelRefs(attrs)
	refs["otel_trace_id"] = traceID
	cmd := &domain.RecordEventCmd{
		Action:         ActionRecordThreadEvent,
		ThreadID:       threadID,
		StepName:       stepName,
		Type:           "otel_span",
		StartedAt:      otelTimestamp(span.GetStartTimeUnixNano()),
		FinishedAt:     otelTimestamp(span.GetEndTimeUnixNano()),
		Context:        otelContext(envelope.resourceAttrs, span.GetAttributes(), traceID, spanID, span.GetParentSpanId()),
		Refs:           refs,
		Status:         status,
		ServiceName:    otelServiceName(attrs, envelope.scope),
		IdempotencyKey: "otel:" + traceID + ":" + spanID,
		ThreadifyMetadata: map[string]interface{}{
			"message": span.GetStatus().GetMessage(),
			"otel": map[string]interface{}{
				"trace_state":         span.GetTraceState(),
				"span_kind":           span.GetKind().String(),
				"status_description":  span.GetStatus().GetMessage(),
				"scope_name":          envelope.scope.GetName(),
				"scope_version":       envelope.scope.GetVersion(),
				"resource_schema_url": envelope.resourceSchemaURL,
				"scope_schema_url":    envelope.scopeSchemaURL,
			},
		},
		SubSteps: otelSubSteps(span),
	}

	resp := s.threads.RecordEventForIngestion(ctx, cmd, ownerID, companyID)
	if resp != nil && (resp.Status == StepStatusSuccess || resp.IsDuplicate) {
		if err := s.correlations.CompleteSpan(ctx, companyID, traceID, spanID, claimToken); err != nil {
			return false, "", err
		}
		claimCompleted = true
		return true, "", nil
	}
	message := "failed to record span"
	if resp != nil && resp.Message != "" {
		message = resp.Message
	}
	if !isPermanentOTelRecordFailure(message) {
		return false, "", fmt.Errorf("record span %s: %s", spanID, message)
	}
	return false, fmt.Sprintf("span %s: %s", spanID, message), nil
}

func isPermanentOTelStartFailure(message string) bool {
	return strings.HasPrefix(message, "Invalid request") ||
		strings.HasPrefix(message, "Not authenticated") ||
		strings.HasPrefix(message, "Invalid startedAt") ||
		strings.HasPrefix(message, "Role is required") ||
		strings.HasPrefix(message, "Role '")
}

func isPermanentOTelRecordFailure(message string) bool {
	permanentPrefixes := []string{
		"Invalid request",
		"Not authenticated",
		"Thread ID is required",
		"StepName is required",
		"Status is required",
		"StartedAt is required",
		"FinishedAt is required",
		"Context is required",
		"PAYLOAD_TOO_LARGE",
		"Thread not found",
		"Access denied",
		"Cannot add steps to completed thread",
		"Step '",
		"Thread must start with one of the entry points",
		"Step validation failed",
	}
	for _, prefix := range permanentPrefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func (s *OTelTraceService) claimSpan(ctx context.Context, companyID, traceID, spanID string) (string, bool, error) {
	deadline := time.NewTimer(otelCorrelationWait)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	token := uuid.NewString()

	for {
		state, err := s.correlations.GetSpanState(ctx, companyID, traceID, spanID)
		if err != nil {
			return "", false, err
		}
		if state == "done" {
			return "", true, nil
		}
		if state == "" {
			claimed, err := s.correlations.ClaimSpan(ctx, companyID, traceID, spanID, token)
			if err != nil {
				return "", false, err
			}
			if claimed {
				return token, false, nil
			}
		}

		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-deadline.C:
			return "", false, errors.New("timed out waiting for concurrent span ingestion")
		case <-ticker.C:
		}
	}
}

func validateOTelSpan(span *tracepb.Span) error {
	if span == nil {
		return errors.New("span is nil")
	}
	if !validOTelID(span.GetTraceId(), 16) {
		return errors.New("span has an invalid trace_id")
	}
	if !validOTelID(span.GetSpanId(), 8) {
		return errors.New("span has an invalid span_id")
	}
	if parent := span.GetParentSpanId(); len(parent) != 0 && !validOTelID(parent, 8) {
		return errors.New("span has an invalid parent_span_id")
	}
	if strings.TrimSpace(span.GetName()) == "" {
		return errors.New("span name is required")
	}
	if span.GetStartTimeUnixNano() == 0 || span.GetEndTimeUnixNano() == 0 || span.GetEndTimeUnixNano() < span.GetStartTimeUnixNano() {
		return errors.New("span has invalid start or end timestamps")
	}
	return nil
}

func validateTraceDirectives(spans []otelSpanEnvelope) error {
	keys := []string{"threadify.thread_id", "threadify.contract", "threadify.label", "threadify.role"}
	seen := make(map[string]string, len(keys)+1)
	for _, envelope := range spans {
		attrs := mergedAttributes(envelope.resourceAttrs, envelope.span.GetAttributes())
		for _, key := range keys {
			value := strings.TrimSpace(attributeString(attrs, key))
			if value == "" {
				continue
			}
			if previous, exists := seen[key]; exists && previous != value {
				return fmt.Errorf("conflicting %s values within one trace", key)
			}
			seen[key] = value
		}

		tagValue, hasTags := attrs["threadify.tags"]
		if !hasTags {
			continue
		}
		tags, err := parseAttributeStringSlice(tagValue)
		if err != nil {
			return fmt.Errorf("invalid threadify.tags: %w", err)
		}
		if len(tags) > 0 {
			canonicalTags := append([]string(nil), tags...)
			sort.Strings(canonicalTags)
			encoded, _ := json.Marshal(canonicalTags)
			value := string(encoded)
			if previous, exists := seen["threadify.tags"]; exists && previous != value {
				return errors.New("conflicting threadify.tags values within one trace")
			}
			seen["threadify.tags"] = value
		}
	}
	return nil
}

func validOTelID(value []byte, expectedLength int) bool {
	if len(value) != expectedLength {
		return false
	}
	for _, b := range value {
		if b != 0 {
			return true
		}
	}
	return false
}

func keyValueMap(values []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	result := make(map[string]*commonpb.AnyValue, len(values))
	for _, value := range values {
		if value != nil && value.GetKey() != "" {
			result[value.GetKey()] = value.GetValue()
		}
	}
	return result
}

func mergedAttributes(resource map[string]*commonpb.AnyValue, span []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	result := make(map[string]*commonpb.AnyValue, len(resource)+len(span))
	for key, value := range resource {
		result[key] = value
	}
	for key, value := range keyValueMap(span) {
		result[key] = value
	}
	return result
}

func attributeString(attrs map[string]*commonpb.AnyValue, key string) string {
	return anyValueString(attrs[key])
}

func attributeStringSlice(attrs map[string]*commonpb.AnyValue, key string) []string {
	result, _ := parseAttributeStringSlice(attrs[key])
	return result
}

func parseAttributeStringSlice(value *commonpb.AnyValue) ([]string, error) {
	if value == nil {
		return nil, errors.New("value must be a string or string array")
	}
	var candidates []string
	if array := value.GetArrayValue(); array != nil {
		result := make([]string, 0, len(array.GetValues()))
		for _, item := range array.GetValues() {
			if _, ok := item.GetValue().(*commonpb.AnyValue_StringValue); !ok {
				return nil, errors.New("array values must be strings")
			}
			if tag := strings.TrimSpace(anyValueString(item)); tag != "" {
				result = append(result, tag)
			}
		}
		candidates = result
	} else if _, ok := value.GetValue().(*commonpb.AnyValue_StringValue); ok {
		if tag := strings.TrimSpace(anyValueString(value)); tag != "" {
			candidates = []string{tag}
		}
	} else {
		return nil, errors.New("value must be a string or string array")
	}

	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, tag := range candidates {
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result, nil
}

func anyValueString(value *commonpb.AnyValue) string {
	if value == nil || value.GetValue() == nil {
		return ""
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return strconv.FormatBool(typed.BoolValue)
	case *commonpb.AnyValue_IntValue:
		return strconv.FormatInt(typed.IntValue, 10)
	case *commonpb.AnyValue_DoubleValue:
		return strconv.FormatFloat(typed.DoubleValue, 'g', -1, 64)
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue, *commonpb.AnyValue_KvlistValue:
		encoded, _ := json.Marshal(anyValueNative(value))
		return string(encoded)
	default:
		return ""
	}
}

func anyValueNative(value *commonpb.AnyValue) interface{} {
	if value == nil || value.GetValue() == nil {
		return nil
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonpb.AnyValue_IntValue:
		return typed.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		result := make([]interface{}, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			result = append(result, anyValueNative(item))
		}
		return result
	case *commonpb.AnyValue_KvlistValue:
		result := make(map[string]interface{}, len(typed.KvlistValue.GetValues()))
		for _, item := range typed.KvlistValue.GetValues() {
			result[item.GetKey()] = anyValueNative(item.GetValue())
		}
		return result
	default:
		return nil
	}
}

func otelRefs(attrs map[string]*commonpb.AnyValue) map[string]string {
	refs := make(map[string]string)
	for key, value := range attrs {
		if strings.HasPrefix(key, "threadify.ref.") {
			refKey := strings.TrimPrefix(key, "threadify.ref.")
			refValue := anyValueString(value)
			if refKey != "" && refValue != "" {
				refs[refKey] = refValue
			}
		}
	}
	return refs
}

func otelContext(resourceAttrs map[string]*commonpb.AnyValue, spanAttrs []*commonpb.KeyValue, traceID, spanID string, parentSpanID []byte) map[string]string {
	spanAttrMap := keyValueMap(spanAttrs)
	contextData := make(map[string]string, len(resourceAttrs)+len(spanAttrMap)+3)
	applyOTelContextLayer(contextData, resourceAttrs)
	applyOTelContextLayer(contextData, spanAttrMap)
	contextData["otel.trace_id"] = traceID
	contextData["otel.span_id"] = spanID
	if len(parentSpanID) > 0 {
		contextData["otel.parent_span_id"] = hex.EncodeToString(parentSpanID)
	}
	return contextData
}

func applyOTelContextLayer(destination map[string]string, attrs map[string]*commonpb.AnyValue) {
	// Ordinary keys are applied first; explicit threadify.context.* aliases win
	// deterministically within each resource/span layer.
	for key, value := range attrs {
		if isThreadifyDirective(key) || strings.HasPrefix(key, "threadify.ref.") || strings.HasPrefix(key, "threadify.context.") {
			continue
		}
		destination[key] = anyValueString(value)
	}
	for key, value := range attrs {
		if !strings.HasPrefix(key, "threadify.context.") {
			continue
		}
		contextKey := strings.TrimPrefix(key, "threadify.context.")
		if contextKey != "" {
			destination[contextKey] = anyValueString(value)
		}
	}
}

func isThreadifyDirective(key string) bool {
	switch key {
	case "threadify.thread_id", "threadify.contract", "threadify.label", "threadify.step_name",
		"threadify.role", "threadify.service", "threadify.tags":
		return true
	default:
		return false
	}
}

func otelServiceName(attrs map[string]*commonpb.AnyValue, scope *commonpb.InstrumentationScope) string {
	if serviceName := attributeString(attrs, "threadify.service"); serviceName != "" {
		return serviceName
	}
	if serviceName := attributeString(attrs, "service.name"); serviceName != "" {
		return serviceName
	}
	if scope != nil && scope.GetName() != "" {
		return scope.GetName()
	}
	return "unknown"
}

func otelSubSteps(span *tracepb.Span) []domain.SubStepCmd {
	result := make([]domain.SubStepCmd, 0, len(span.GetEvents()))
	for _, event := range span.GetEvents() {
		if event == nil || strings.TrimSpace(event.GetName()) == "" {
			continue
		}
		recordedAt := event.GetTimeUnixNano()
		if recordedAt == 0 {
			recordedAt = span.GetEndTimeUnixNano()
		}
		payload := make(map[string]interface{}, len(event.GetAttributes()))
		for key, value := range keyValueMap(event.GetAttributes()) {
			payload[key] = anyValueNative(value)
		}
		result = append(result, domain.SubStepCmd{
			Name:       event.GetName(),
			Status:     StepStatusSuccess,
			Payload:    payload,
			RecordedAt: otelTimestamp(recordedAt),
		})
	}
	return result
}

func otelTimestamp(nanos uint64) string {
	seconds := nanos / uint64(time.Second)
	remainder := nanos % uint64(time.Second)
	return time.Unix(int64(seconds), int64(remainder)).UTC().Format(time.RFC3339Nano)
}

func otelErrorMessage(messages []string) string {
	if len(messages) == 0 {
		return "one or more spans were rejected"
	}
	seen := make(map[string]struct{}, len(messages))
	unique := make([]string, 0, len(messages))
	for _, message := range messages {
		if _, exists := seen[message]; exists {
			continue
		}
		seen[message] = struct{}{}
		unique = append(unique, message)
	}
	result := strings.Join(unique, "; ")
	if len(result) > 1024 {
		return result[:1021] + "..."
	}
	return result
}
