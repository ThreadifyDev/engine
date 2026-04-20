package archiver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)


var baseCycle = time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)

func spendEvt(id string, amount int64) UsageSyncEvent {
	return UsageSyncEvent{
		EventID: id, CompanyID: "c1",
		Meter:             "credit_spend",
		Amount:            amount,
		BillingCycleStart: baseCycle,
		OccurredAt:        baseCycle,
	}
}

func topupEvt(id string, amount int64) UsageSyncEvent {
	return UsageSyncEvent{
		EventID: id, CompanyID: "c1",
		Meter:             "credit_topup",
		Amount:            amount,
		BillingCycleStart: baseCycle,
		OccurredAt:        baseCycle,
	}
}

func newPW(t *testing.T, db DBExecer) *PostgresWriter {
	t.Helper()
	return NewPostgresWriter(db, zap.NewNop())
}

func TestPostgresWriter_SyncUsageMeters_SuccessAndDuplicate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)

	// First event: new row
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).
		Times(1)

	// Second event: duplicate
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(pgconn.NewCommandTag("INSERT 0 0"), nil).
		Times(1)

	err := newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
		spendEvt("e1", -100),
		spendEvt("e1-dup", -200),
	})

	assert.NoError(t, err)
}

func TestPostgresWriter_SyncUsageMeters_SkipsInvalidEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	// Exec must NOT be called — gomock enforces this.

	err := newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
		{EventID: "e1", CompanyID: "c1", Meter: "unknown_meter", Amount: -1,
			BillingCycleStart: baseCycle, OccurredAt: baseCycle},
		spendEvt("e2", 100),  // wrong sign — spend must be negative
		topupEvt("e3", -100), // wrong sign — topup must be positive
	})

	assert.NoError(t, err)
}

func TestPostgresWriter_SyncUsageMeters_EmptyInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	// Exec must NOT be called.

	err := newPW(t, db).SyncUsageMeters(context.Background(), nil)
	assert.NoError(t, err)
}

func TestPostgresWriter_SyncUsageMeters_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)

	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(pgconn.CommandTag{}, errors.New("connection reset by peer")).
		Times(1)

	err := newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
		spendEvt("e1", -50),
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset by peer")
}

func TestPostgresWriter_SyncUsageMeters_CreditTopup_ZeroAmountSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)

	// Only the positive topup should reach DB
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).
		Times(1)

	err := newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
		topupEvt("t1", 500), // valid positive
		{EventID: "t2", CompanyID: "c1", Meter: "credit_topup", Amount: 0, // zero — skipped
			BillingCycleStart: baseCycle, OccurredAt: baseCycle},
	})

	assert.NoError(t, err)
}

func TestPostgresWriter_SyncUsageMeters_MixedBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)

	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).
		Times(2) // spend + topup

	err := newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
		spendEvt("s1", -100),
		{EventID: "bad", CompanyID: "c1", Meter: "nope", Amount: -1,
			BillingCycleStart: baseCycle, OccurredAt: baseCycle},
		topupEvt("t1", 200),
	})

	assert.NoError(t, err)
}

func TestPostgresWriter_SyncUsageMeters_DeterministicID_IsStable(t *testing.T) {
	var capturedIDs []interface{}

	captureID := func() interface{} {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		db := NewMockDBExecer(ctrl)

		db.EXPECT().
			Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
				// creditUpsertQuery parameter order:
				//   $1 = amount, $2 = company_id, $3 = billing_cycle_start,
				//   $4 = last_sync_event_id, $5 = chargedDelta, $6 = id
				capturedIDs = append(capturedIDs, args[5])
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			}).
			Times(1)

		_ = newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
			spendEvt("ev-stable", -10),
		})
		return capturedIDs[len(capturedIDs)-1]
	}

	id1, id2 := captureID(), captureID()
	assert.Equal(t, id1, id2, "deterministicID must return the same value for identical inputs")
	assert.NotEmpty(t, id1, "deterministicID must return a non-empty string")
}

func TestPostgresWriter_SyncUsageMeters_ChargedDelta_SpendVsTopup(t *testing.T) {
	t.Run("spend produces positive chargedDelta", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		db := NewMockDBExecer(ctrl)
		db.EXPECT().
			Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
				assert.Equal(t, int64(250), args[4])
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			}).
			Times(1)

		_ = newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
			spendEvt("s1", -250),
		})
	})

	t.Run("topup produces zero chargedDelta", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		db := NewMockDBExecer(ctrl)
		db.EXPECT().
			Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
				// $5 = chargedDelta = 0 for topup
				assert.Equal(t, int64(0), args[4])
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			}).
			Times(1)

		_ = newPW(t, db).SyncUsageMeters(context.Background(), []UsageSyncEvent{
			topupEvt("t1", 500),
		})
	})
}
