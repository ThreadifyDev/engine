package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/pkg/contractcontent"
	"go.uber.org/zap"
)

func TestStepValidationAcceptsOptionalContextWhenAbsentOrPresent(t *testing.T) {
	svc := NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop())
	node := domain.GraphNode{BusinessContext: &domain.BusinessContext{
		Required: []string{"order_id"}, Optional: []string{"provider_id"},
	}}
	require.NoError(t, svc.ValidateStepContext(context.Background(), node, map[string]string{"order_id": "o-1"}))
	require.NoError(t, svc.ValidateStepContext(context.Background(), node, map[string]string{
		"order_id": "o-1", "provider_id": "p-1", "http.route": "/checkout",
	}))
	require.ErrorContains(t, svc.ValidateStepContext(context.Background(), node, map[string]string{"provider_id": "p-1"}), `required context field "order_id" is missing`)
}

func TestStepValidationCannotUseUndeclaredContext(t *testing.T) {
	svc := NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop())
	node := domain.GraphNode{
		BusinessContext: &domain.BusinessContext{Optional: []string{"provider_id"}},
		ContentRules:    []contractcontent.Rule{{Field: "approval_code", Operator: "nonempty"}},
	}
	err := svc.ValidateStepContext(context.Background(), node, map[string]string{
		"provider_id": "p-1", "approval_code": "secret-extra",
	})
	require.ErrorContains(t, err, `required content field "approval_code" is missing`)
}

func TestStepWithoutDeclaredContextCannotUseSubmittedFields(t *testing.T) {
	svc := NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop())
	node := domain.GraphNode{
		ContentRules: []contractcontent.Rule{{Field: "approval_code", Operator: "nonempty"}},
	}
	provided := map[string]string{"approval_code": "secret-extra", "otel.trace_id": "trace-1"}
	require.ErrorContains(t, svc.ValidateStepContext(context.Background(), node, provided), `required content field "approval_code" is missing`)
	registered, extra := registeredStepContext(node, provided)
	require.Empty(t, registered)
	require.Equal(t, provided, extra)
}
