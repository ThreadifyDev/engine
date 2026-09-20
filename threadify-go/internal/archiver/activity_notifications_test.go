package archiver

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type notificationArchiveDB struct {
	DBExecer
	exec func(string, []any) (pgconn.CommandTag, error)
}

func (d notificationArchiveDB) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return d.exec(query, args)
}

func TestValidationActivityPopulatesNotificationProjection(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "retry on projection failure"}[fail], func(t *testing.T) {
			event := StreamEvent{Data: map[string]string{
				"type": "validation_result", "threadId": "thread", "stepId": "refund:attempt",
				"notificationStepId": "step-uuid", "notificationId": "notification-uuid", "notificationType": "rule.violated",
				"source": "rule", "status": "violated", "stepStatus": "success", "severity": "critical",
				"message": "Missing approval", "timestamp": "2026-09-20T00:00:00Z", "details": `{"violations":[]}`,
			}}
			failure := errors.New("database unavailable")
			calls := 0
			db := notificationArchiveDB{exec: func(query string, args []any) (pgconn.CommandTag, error) {
				calls++
				if calls == 1 {
					require.Contains(t, query, "INSERT INTO thread_notifications")
					require.Contains(t, query, "ON CONFLICT (notification_id) DO NOTHING")
					require.Equal(t, []any{"notification-uuid", "thread", "step-uuid", "refund", "attempt"}, args[:5])
					require.Equal(t, "rule.violated", args[6])
					require.JSONEq(t, `{"violations":[]}`, args[12].(string))
					if fail {
						return pgconn.CommandTag{}, failure
					}
				} else {
					require.Contains(t, query, "INSERT INTO thread_activities")
				}
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			}}
			err := NewPostgresWriter(db, zap.NewNop()).WriteActivityLog(context.Background(), []StreamEvent{event})
			if fail {
				require.ErrorIs(t, err, failure)
				require.Equal(t, 1, calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 2, calls)
			}
		})
	}
}
