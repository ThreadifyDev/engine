package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	shderrors "threadify-go/shared/errors"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/service/tests/common"
	"github.com/threadify/engine/pkg/validator"
	"go.uber.org/zap"
)

const includedFeature = `Feature: identity_required
Version: 2

Rule: Verify identity
  When step "identity_verified" is submitted
  Then owner must be "reviewer"
  And this step is an entry point
  And this step is terminal
`

const composingFeature = `Feature: refund_processing
Version: 1
Include: identity_required:2

Rule: Issue refund
  When step "issue_refund" is submitted
  Then owner must be "processor"
  And step "identity_verified" must have succeeded
  And this step is terminal
`

func compiledInclude(t *testing.T) string {
	t.Helper()
	contract, result := validator.NewContractValidator().Validate(includedFeature)
	require.True(t, result.IsValid, "%v", result.Errors)
	full, _, err := validator.NewContractValidator().SerializeContract(contract)
	require.NoError(t, err)
	return full
}

func expectIncludeLookup(deps *common.MockedDependencies, content string) {
	deps.ContractRepo.EXPECT().GetByNameAndCompany(gomock.Any(), "identity_required", "company").Return(&domain.Contract{ID: "included"}, nil)
	deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "included", 2).Return(&domain.ContractVersion{Content: content, Version: 2}, nil)
}

func TestContractIncludePreviewAndPublish(t *testing.T) {
	deps := common.NewMockDeps(t)
	expectIncludeLookup(deps, compiledInclude(t))
	svc := service.NewContractService(deps.ContractRepo, deps.PlanSvc, zap.NewNop())
	contract, graph, result, err := svc.PreviewContract(context.Background(), "company", composingFeature)
	require.NoError(t, err)
	require.True(t, result.IsValid, "%v", result.Errors)
	require.Len(t, contract.Steps, 2)
	require.Contains(t, graph.Graph.Nodes, "identity_verified")
	require.Contains(t, graph.Graph.Nodes, "issue_refund")
	require.Contains(t, graph.Graph.EntryPoints, "identity_verified")
	require.Equal(t, []string{"issue_refund"}, graph.Graph.TerminalSteps)
	t.Logf("composed preview: valid=%t entry=%v terminal=%v steps=%d", result.IsValid, graph.Graph.EntryPoints, graph.Graph.TerminalSteps, len(graph.Graph.Nodes))

	// Publishing saves a fully expanded snapshot while retaining the user's source.
	expectIncludeLookup(deps, compiledInclude(t))
	deps.PlanSvc.EXPECT().CheckCreditAvailable(gomock.Any(), "company", service.MeterContractExecution, int64(1)).Return(nil)
	deps.ContractRepo.EXPECT().CreateContractWithVersion(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ *domain.Contract, version *domain.ContractVersion) error {
		require.Equal(t, composingFeature, version.Source)
		var saved validator.Contract
		require.NoError(t, json.Unmarshal([]byte(version.Content), &saved))
		require.Len(t, saved.Steps, 2)
		require.Empty(t, saved.Includes)
		var savedGraph domain.ContractGraph
		require.NoError(t, json.Unmarshal(version.Graph, &savedGraph))
		require.Contains(t, savedGraph.Graph.Nodes, "identity_verified")
		return nil
	})
	deps.PlanSvc.EXPECT().ChargeContract(gomock.Any(), "company").Return(nil)
	status, _ := svc.CreateContract(context.Background(), "owner", "company", "owner", composingFeature)
	require.Equal(t, 200, status)
}

func TestContractIncludeInvalidReferences(t *testing.T) {
	for _, tc := range []struct {
		name         string
		content      string
		lookupError  error
		versionError error
		want         string
	}{
		{name: "missing contract", lookupError: shderrors.ErrContractNotFound, want: "was not found"},
		{name: "missing version", versionError: shderrors.ErrContractNotFound, want: "version 2 was not found"},
		{name: "wrong version content", content: `{"ContractName":"identity_required","Version":1}`, want: "no valid compiled content"},
		{name: "duplicate step", content: `{"ContractName":"identity_required","Version":2,"Steps":[{"ID":"issue_refund","Owner":"processor"}],"Parties":["processor"]}`, want: "Duplicate step ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			if tc.lookupError != nil {
				deps.ContractRepo.EXPECT().GetByNameAndCompany(gomock.Any(), "identity_required", "company").Return(nil, tc.lookupError)
			} else if tc.versionError != nil {
				deps.ContractRepo.EXPECT().GetByNameAndCompany(gomock.Any(), "identity_required", "company").Return(&domain.Contract{ID: "included"}, nil)
				deps.ContractRepo.EXPECT().GetVersion(gomock.Any(), "included", 2).Return(nil, tc.versionError)
			} else {
				expectIncludeLookup(deps, tc.content)
			}
			svc := service.NewContractService(deps.ContractRepo, nil, zap.NewNop())
			_, _, result, err := svc.PreviewContract(context.Background(), "company", composingFeature)
			require.NoError(t, err)
			require.False(t, result.IsValid)
			require.Contains(t, result.Errors[0].Message, tc.want)
		})
	}
}

func TestContractIncludeRejectsNonGherkinSources(t *testing.T) {
	for _, source := range []string{
		"contract_name: composed\nversion: 1\nincludes:\n  - name: identity_required\n    version: 2\n",
		`{"contract_name":"composed","version":1,"includes":[{"name":"identity_required","version":2}]}`,
	} {
		svc := service.NewContractService(nil, nil, zap.NewNop())
		_, _, result, err := svc.PreviewContract(context.Background(), "company", source)
		require.NoError(t, err)
		require.False(t, result.IsValid)
		require.Equal(t, "source", result.Errors[0].Field)
	}
}

func TestContractIncludeRejectsContradictions(t *testing.T) {
	strictSource := `Feature: identity_required
Version: 2

Rule: Start
  When step "identity_verified" is submitted
  Then owner must be "reviewer"
  And this step is an entry point
  And next step must be one of "identity_recorded"

Rule: Record
  When step "identity_recorded" is submitted
  Then owner must be "reviewer"
  And this step is terminal
`
	strictContract, result := validator.NewContractValidator().Validate(strictSource)
	require.True(t, result.IsValid, "%v", result.Errors)
	strictContent, _, err := validator.NewContractValidator().SerializeContract(strictContract)
	require.NoError(t, err)
	contentConflict := `Feature: refund_processing
Version: 1
Include: identity_required:2
Rule: Issue refund
  When step "issue_refund" is submitted
  Then owner must be "processor"
  And step "identity_verified" must have succeeded
  And content "status" must equal "approved"
  And content "status" must equal "rejected"
  And this step is terminal
`
	strictConflict := `Feature: refund_processing
Version: 1
Include: identity_required:2
Rule: Issue refund
  When step "issue_refund" is submitted
  Then owner must be "processor"
  And step "approval" must have succeeded
  And next step must be one of "approval"
  And this step is terminal
Rule: Approve
  When step "approval" is submitted
  Then owner must be "processor"
  And next step must be one of "issue_refund"
`
	for _, tc := range []struct {
		name, source, content, wantField, wantMessage string
	}{
		{"unusable entry", strings.Replace(composingFeature, "  And this step is terminal", "  And this step is an entry point\n  And this step is terminal", 1), compiledInclude(t), "entry_points", "Every entry point requires"},
		{"terminal prerequisite", strings.Replace(composingFeature, "  And this step is terminal\n", "", 1), compiledInclude(t), "steps.issue_refund.depends_on", "depends on terminal step"},
		{"strict transition blocks prerequisite", strictConflict, strictContent, "steps.identity_recorded", "no outgoing transitions"},
		{"impossible content", contentConflict, compiledInclude(t), "steps.issue_refund.content_rules", "no value satisfying"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			expectIncludeLookup(deps, tc.content)
			svc := service.NewContractService(deps.ContractRepo, nil, zap.NewNop())
			_, _, result, err := svc.PreviewContract(context.Background(), "company", tc.source)
			require.NoError(t, err)
			require.False(t, result.IsValid)
			t.Logf("contradiction preview: %s valid=%t errors=%v", tc.name, result.IsValid, result.Errors)
			require.Contains(t, fmt.Sprint(result.Errors), tc.wantMessage)
			require.True(t, hasValidationField(result.Errors, tc.wantField), "%v", result.Errors)
			expectIncludeLookup(deps, tc.content)
			status, _ := svc.CreateContract(context.Background(), "owner", "company", "owner", tc.source)
			require.Equal(t, 400, status)
		})
	}
}

func hasValidationField(errors []validator.ValidationError, field string) bool {
	for _, err := range errors {
		if err.Field == field {
			return true
		}
	}
	return false
}

type compositionReviewStub struct {
	flagged bool
	err     error
	calls   int
}

func (s *compositionReviewStub) ReviewComposition(_ context.Context, source string, expanded *validator.Contract) (bool, error) {
	s.calls++
	if source != composingFeature || len(expanded.Steps) != 2 {
		return false, errors.New("wrong composition supplied to reviewer")
	}
	return s.flagged, s.err
}

func TestContractIncludeSemanticReviewIsAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flagged bool
		err     error
		want    string
	}{
		{"possible conflict", true, nil, "possible semantic conflict"},
		{"review unavailable", false, errors.New("offline"), "review was unavailable"},
		{"no conflict reported", false, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			expectIncludeLookup(deps, compiledInclude(t))
			svc := service.NewContractService(deps.ContractRepo, nil, zap.NewNop())
			reviewer := &compositionReviewStub{flagged: tc.flagged, err: tc.err}
			svc.SetCompositionReviewer(reviewer)
			_, _, result, err := svc.PreviewContract(context.Background(), "company", composingFeature)
			require.NoError(t, err)
			require.True(t, result.IsValid)
			require.Equal(t, 1, reviewer.calls)
			if tc.want == "" {
				require.Empty(t, result.Warnings)
			} else {
				require.Len(t, result.Warnings, 1)
				require.Contains(t, result.Warnings[0], tc.want)
			}
		})
	}
}
