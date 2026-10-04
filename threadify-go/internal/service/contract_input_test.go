package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	gonats "github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/broker"
	"github.com/threadify/engine/internal/config"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"go.uber.org/zap"
	"testing"
	shareddomain "threadify-go/shared/domain"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"threadify-go/shared/actionmapping"
)

type inputClassifier struct {
	DecisionClassifier
	result string
	err    error
	calls  int
}

func (c *inputClassifier) MatchAction(context.Context, string, []string) (string, error) {
	c.calls++
	return c.result, c.err
}

func TestContractInputMatching(t *testing.T) {
	for _, tc := range []struct {
		name, input, choice, classification, step string
		classifierCalls                           int
		exact                                     bool
		classifierErr                             error
	}{
		{name: "exact mapping", input: "checkout.confirm", classification: "mapped_step", step: "confirm"},
		{name: "longest prefix", input: "checkout.review.open", classification: "mapped_step", step: "review"},
		{name: "prefix", input: "checkout.open", classification: "mapped_step", step: "checkout"},
		{name: "pinned version", input: "old-input", classification: "mapped_step", step: "checkout"},
		{name: "exact step", input: "confirm", classification: "step_candidate", step: "confirm", exact: true},
		{name: "Jev match", input: "confirm my order", choice: "confirm", classification: "step_candidate", step: "confirm", classifierCalls: 1},
		{name: "Jev no match", input: "unrelated", classification: "substep", classifierCalls: 1},
		{name: "Jev invalid choice", input: "unrelated", choice: "not-a-step", classification: "substep", classifierCalls: 1},
		{name: "Jev group choice", input: "unrelated", choice: "group", classification: "substep", classifierCalls: 1},
		{name: "Jev unavailable", input: "unrelated", classification: "substep", classifierCalls: 1, classifierErr: errors.New("offline")},
		{name: "invalid mapping", input: "bad-input", classification: "mapping_rejected", step: "removed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			validator := enginemocks.NewMockContractGraphValidator(ctrl)
			validator.EXPECT().GetContractGraph(gomock.Any(), "orders", 1, "company").Return(&domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
				"checkout": {Type: "step"}, "review": {Type: "step"}, "confirm": {Type: "step"}, "group": {Type: "parallel_group"},
			}}}, nil)
			classifier := &inputClassifier{result: tc.choice, err: tc.classifierErr}
			svc := &ThreadService{contractValidator: validator, browserClassifier: classifier, browserActionMappings: &actionLinkStore{settings: actionmapping.Settings{Rules: []actionmapping.Rule{
				{Contract: "orders", Version: 1, Action: "checkout.*", Step: "checkout"},
				{Contract: "orders", Version: 1, Action: "checkout.confirm", Step: "confirm"},
				{Contract: "orders", Version: 1, Action: "checkout.review.*", Step: "review"},
				{Contract: "orders", Version: 2, Action: "old-input", Step: "confirm"},
				{Contract: "orders", Version: 1, Action: "old-input", Step: "checkout"},
				{Contract: "orders", Version: 1, Action: "bad-input", Step: "removed"},
			}}}}
			version := 1
			match, err := svc.matchContractInput(context.Background(), &domain.Thread{ContractName: "orders", ContractVersion: &version, CompanyID: "company"}, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.classification, match.classification)
			require.Equal(t, tc.step, match.step)
			require.Equal(t, tc.exact, match.exact)
			require.Equal(t, tc.classifierCalls, classifier.calls)
		})
	}
}

func TestDirectSDKEventsBypassInputMappings(t *testing.T) {
	ctrl := gomock.NewController(t)
	connections := enginemocks.NewMockConnectionManager(ctrl)
	connections.EXPECT().IsConnected("owner").Return(true)
	plan := enginemocks.NewMockPlanService(ctrl)
	plan.EXPECT().GetCurrentLimits(gomock.Any(), "company").Return(nil, errors.New("normal write path reached"))
	classifier := &inputClassifier{}
	svc := &ThreadService{connectionMgr: connections, planService: plan, browserClassifier: classifier,
		browserActionMappings: &actionLinkStore{err: errors.New("must not be loaded")}}
	response := svc.HandleRecordEvent(context.Background(), &domain.RecordEventCmd{
		ThreadID: "thread", StepName: "direct_step", Type: "otel_span", Context: map[string]string{},
		Status: "success", StartedAt: "2026-10-04T10:00:00Z", FinishedAt: "2026-10-04T10:00:01Z",
	}, "owner", "company")
	require.Contains(t, response.Message, "normal write path reached")
	require.Zero(t, classifier.calls)
}

func TestOTelJevCandidateRecordsEvidenceWithoutCompletingStep(t *testing.T) {
	ctx := context.Background()
	runtime, err := broker.Start(ctx, broker.Options{Mode: "embedded", StoreDir: t.TempDir(), MaxMemoryBytes: 64 << 20, MaxStoreBytes: 8 << 30})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close(ctx)) })
	client, err := natsrepo.NewClient(&config.NATSConfig{URL: gonats.DefaultURL, StreamName: "NOTIFICATIONS"}, zap.NewNop(), runtime.ClientOptions()...)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	require.NoError(t, client.InitStreams(ctx))

	ctrl := gomock.NewController(t)
	repo := enginemocks.NewMockThreadRepository(ctrl)
	version := 1
	thread := &domain.Thread{ID: "thread", OwnerID: "owner", CompanyID: "company", ContractName: "orders", ContractVersion: &version, Status: domain.ThreadStatusActive}
	repo.EXPECT().Get(gomock.Any(), "thread", gomock.Any()).Return(thread, nil).AnyTimes()
	repo.EXPECT().Get(gomock.Any(), "thread").Return(thread, nil).AnyTimes()
	plan := enginemocks.NewMockPlanService(ctrl)
	plan.EXPECT().GetCurrentLimits(gomock.Any(), "company").Return(&shareddomain.CreditAccount{}, nil).AnyTimes()
	plan.EXPECT().CheckPayloadSize(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	plan.EXPECT().DecrementIngress(gomock.Any(), "company", gomock.Any()).Return(nil)
	validator := enginemocks.NewMockContractGraphValidator(ctrl)
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{"confirm": {Type: "step"}}}}
	validator.EXPECT().GetContractGraph(gomock.Any(), "orders", 1, "company").Return(graph, nil).AnyTimes()
	classifier := &inputClassifier{result: "confirm"}
	threadService := &ThreadService{repo: repo, planService: plan, contractValidator: validator,
		accessService: &ThreadAccessService{}, browserClassifier: classifier,
		natsArchivalPublisher: natsrepo.NewArchivalPublisher(client, zap.NewNop()), logger: zap.NewNop()}
	svc := NewOTelTraceService(threadService, newFakeOTelCorrelationRepository(), zap.NewNop())
	span := validOTelSpan()
	span.Name = "confirm my order"
	batch := otelRequest(span, []*commonpb.KeyValue{otelKV("threadify.thread_id", otelString("thread"))})
	for i := 0; i < 2; i++ {
		response, err := svc.Ingest(ctx, batch, "owner", "company")
		require.NoError(t, err)
		require.Nil(t, response.PartialSuccess)
	}
	require.Equal(t, 1, classifier.calls)
	stream, err := client.JetStream().Stream(ctx, natsrepo.StreamActivityLog)
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, info.State.Msgs)
	message, err := stream.GetMsg(ctx, info.State.LastSeq)
	require.NoError(t, err)
	var event map[string]interface{}
	require.NoError(t, json.Unmarshal(message.Data, &event))
	require.Equal(t, "trace_input", event["type"])
	require.Equal(t, "step_candidate", event["classification"])
	require.Equal(t, "confirm", event["matchedStep"])
	require.Equal(t, "confirm my order", event["name"])
	require.Equal(t, "", event["stepId"])
	require.Equal(t, otelTimestamp(span.StartTimeUnixNano), event["startedAt"])

	// An explicit step directive bypasses inference, including when it is invalid.
	span.SpanId[0]++
	span.Attributes = []*commonpb.KeyValue{otelKV("threadify.step_name", otelString("explicit_step"))}
	response, err := svc.Ingest(ctx, batch, "owner", "company")
	require.NoError(t, err)
	require.EqualValues(t, 1, response.PartialSuccess.RejectedSpans)
	require.Contains(t, response.PartialSuccess.ErrorMessage, "Step 'explicit_step' not found")
	require.Equal(t, 1, classifier.calls)

	// An authored mapping uses the normal step path with the original span key.
	threadService.browserActionMappings = &actionLinkStore{settings: actionmapping.Settings{Rules: []actionmapping.Rule{
		{Contract: "orders", Version: 1, Action: "confirm my order", Step: "confirm"},
	}}}
	span.SpanId[0]++
	span.Attributes = nil
	repo.EXPECT().GetStepStatus(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, query domain.StepStatusQuery, _ ...domain.ThreadReadOptions) (string, error) {
			require.Equal(t, "confirm", query.StepName)
			require.Equal(t, "otel:"+hex.EncodeToString(span.TraceId)+":"+hex.EncodeToString(span.SpanId), query.IdempotencyKey)
			require.Equal(t, StepStatusSuccess, query.ExpectedStatus)
			return StepStatusSuccess, nil
		})
	response, err = svc.Ingest(ctx, batch, "owner", "company")
	require.NoError(t, err)
	require.Nil(t, response.PartialSuccess)
	require.Equal(t, 1, classifier.calls)
	info, err = stream.Info(ctx)
	require.NoError(t, err)
	message, err = stream.GetMsg(ctx, info.State.LastSeq)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(message.Data, &event))
	require.Equal(t, "mapped_step", event["classification"])
	require.Equal(t, "confirm", event["mappedStep"])
}
