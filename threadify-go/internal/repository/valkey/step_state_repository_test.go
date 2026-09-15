package valkey

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

type companyScopedStepReader struct {
	stepStatePostgresReader
	companyID string
	steps     []*domain.StepStateInfo
	calls     int
}

func (r *companyScopedStepReader) GetStepsWithPermissionCheck(_ context.Context, threadID, companyID string, stepName, idempotencyKey, status *string) ([]*domain.StepStateInfo, error) {
	r.calls++
	if companyID != r.companyID {
		return nil, nil
	}
	var steps []*domain.StepStateInfo
	for _, step := range r.steps {
		if threadID != step.ThreadID || stepName != nil && *stepName != step.StepName ||
			idempotencyKey != nil && *idempotencyKey != step.IdempotencyKey ||
			status != nil && *status != step.Status {
			continue
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func TestStepsCacheMissUsesCompanyForPostgresAuthorization(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(map[bool]string{true: "own company", false: "other company"}[allowed], func(t *testing.T) {
			client := enginemocks.NewMockValkeyClient(gomock.NewController(t))
			client.EXPECT().Keys(gomock.Any(), "thread:thread-1:steps:*").Return([]string{}, nil)
			step := &domain.StepStateInfo{
				ThreadID: "thread-1", StepName: "checkout", Status: "success",
				IdempotencyKey: "otel:trace-1:span-1", Actor: "service-account-1",
			}
			reader := &companyScopedStepReader{companyID: "company-1", steps: []*domain.StepStateInfo{step}}
			repo := &StepStateRepository{client: client, postgresRepo: reader, logger: zap.NewNop()}
			companyID := "company-1"
			if !allowed {
				companyID = "company-2"
			}

			steps, err := repo.GetStepsWithPermissionCheck(context.Background(), step.ThreadID, step.Actor, companyID,
				&domain.PermissionCheckResult{HasAccess: true, HasFullRead: true},
				&step.StepName, &step.IdempotencyKey, &step.Status)

			require.NoError(t, err)
			require.Equal(t, 1, reader.calls)
			if allowed {
				require.Equal(t, []*domain.StepStateInfo{step}, steps)
			} else {
				require.Empty(t, steps)
			}
		})
	}
}

func TestStepsCacheMissPreservesOwnReadRestriction(t *testing.T) {
	for _, fullRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "own only", true: "full read takes precedence"}[fullRead], func(t *testing.T) {
			client := enginemocks.NewMockValkeyClient(gomock.NewController(t))
			client.EXPECT().Keys(gomock.Any(), "thread:thread-1:steps:*").Return([]string{}, nil)
			own := &domain.StepStateInfo{ThreadID: "thread-1", Actor: "service-account-1"}
			reader := &companyScopedStepReader{
				companyID: "company-1",
				steps: []*domain.StepStateInfo{
					{ThreadID: "thread-1", Actor: "service-account-2"},
					own,
					{ThreadID: "thread-1", Actor: ""},
				},
			}
			repo := &StepStateRepository{client: client, postgresRepo: reader, logger: zap.NewNop()}

			steps, err := repo.GetStepsWithPermissionCheck(context.Background(), "thread-1", "service-account-1", "company-1",
				&domain.PermissionCheckResult{HasAccess: true, HasFullRead: fullRead, HasOwnRead: true}, nil, nil, nil)

			require.NoError(t, err)
			require.Equal(t, 1, reader.calls)
			if fullRead {
				require.Equal(t, reader.steps, steps)
			} else {
				require.Equal(t, []*domain.StepStateInfo{own}, steps)
			}
		})
	}
}

func TestStepsWithoutPermissionDoNotReadCacheOrPostgres(t *testing.T) {
	client := enginemocks.NewMockValkeyClient(gomock.NewController(t))
	repo := NewStepStateRepositoryWithPostgres(60, client, nil, zap.NewNop())
	steps, err := repo.GetStepsWithPermissionCheck(context.Background(), "thread-1", "service-account-1", "company-1",
		&domain.PermissionCheckResult{}, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, steps)
}

func TestCachedStepsPreserveOTelIdempotencyKey(t *testing.T) {
	for _, permissionCheck := range []bool{false, true} {
		for _, filtered := range []bool{false, true} {
			name := map[bool]string{false: "list", true: "permission checked"}[permissionCheck] + "/" +
				map[bool]string{false: "unfiltered", true: "filtered"}[filtered]
			t.Run(name, func(t *testing.T) {
				client := enginemocks.NewMockValkeyClient(gomock.NewController(t))
				key := "thread:thread-1:steps:checkout:otel:trace-1:span-1"
				client.EXPECT().Keys(gomock.Any(), "thread:thread-1:steps:*").Return([]string{key}, nil)
				client.EXPECT().HGetAll(gomock.Any(), key).Return(map[string]string{
					"status": "success", "actor": "service-account-1",
				}, nil)
				repo := NewStepStateRepositoryWithPostgres(60, client, nil, zap.NewNop())
				idempotencyKey := "otel:trace-1:span-1"
				var filter *string
				if filtered {
					filter = &idempotencyKey
				}
				var steps []*domain.StepStateInfo
				var err error
				if permissionCheck {
					steps, err = repo.GetStepsWithPermissionCheck(context.Background(), "thread-1", "service-account-1", "company-1",
						&domain.PermissionCheckResult{HasAccess: true, HasOwnRead: true}, nil, filter, nil)
				} else {
					steps, err = repo.ListSteps(context.Background(), "thread-1", nil, filter, nil)
				}
				require.NoError(t, err)
				require.Len(t, steps, 1)
				require.Equal(t, idempotencyKey, steps[0].IdempotencyKey)
				require.Equal(t, "checkout", steps[0].StepName)
			})
		}
	}
}
