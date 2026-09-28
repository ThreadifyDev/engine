package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/archiver"
	"github.com/threadify/engine/internal/retention"
	"github.com/threadify/engine/tests/internal/testenv"
	"go.uber.org/zap"
	managementrepo "threadify-go/shared/management/repository"
)

func TestThreadRetentionKeepsActiveAndRecentData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pg, err := testenv.StartPostgresContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		require.NoError(t, pg.Terminate(cleanupCtx))
	})
	vk, err := testenv.StartValkeyContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		require.NoError(t, vk.Terminate(cleanupCtx))
	})
	require.NoError(t, pg.InitSchema(ctx))
	options, err := redis.ParseURL(vk.URI)
	require.NoError(t, err)
	cache := redis.NewClient(options)
	defer cache.Close()
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-31 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)
	_, err = pg.Pool.Exec(ctx, `INSERT INTO companies(id,name) VALUES ('retention-a','A')`)
	require.NoError(t, err)
	for _, row := range []struct {
		id, company, status string
		updated             time.Time
	}{
		{"old-completed", "retention-a", "completed", old},
		{"old-cancelled", "retention-a", "cancelled", old},
		{"old-closed", "retention-a", "closed", old},
		{"old-failed", "retention-a", "failed", old},
		{"recent-completed", "retention-a", "completed", recent},
		{"old-active", "retention-a", "active", old},
	} {
		_, err = pg.Pool.Exec(ctx, `INSERT INTO threads(id,company_id,status,created_at,updated_at,terminal_archived_at) VALUES($1,$2,$3,$4,$4,$4)`, row.id, row.company, row.status, row.updated)
		require.NoError(t, err)
		require.NoError(t, cache.Set(ctx, "thread:"+row.id, "cached", 0).Err())
		require.NoError(t, cache.Set(ctx, "thread:"+row.id+":grant:invocation", "cached", 0).Err())
	}
	for _, id := range []string{"old-completed", "old-closed", "old-failed", "recent-completed", "old-active"} {
		_, err = pg.Pool.Exec(ctx, `INSERT INTO thread_activities(thread_id,activity_type,actor,actor_service,payload) VALUES($1,'step_recorded','actor','service','{}')`, id)
		require.NoError(t, err)
		_, err = pg.Pool.Exec(ctx, `INSERT INTO thread_refs(thread_id,ref_key,ref_value) VALUES($1,'order','O-1')`, id)
		require.NoError(t, err)
		_, err = pg.Pool.Exec(ctx, `INSERT INTO thread_access(thread_id,user_id,roles) VALUES($1,'actor','[]')`, id)
		require.NoError(t, err)
		_, err = pg.Pool.Exec(ctx, `INSERT INTO thread_validations(validation_id,thread_id,step_id,step_name,timestamp,validations,overall_status,has_critical_violation,critical_count,warning_count,minor_count,info_count,total_validations) VALUES($1,$2,'step:key','step',$3,'[]','passed',false,0,0,0,0,0)`, fmt.Sprintf("validation-%s", id), id, old)
		require.NoError(t, err)
		_, err = pg.Pool.Exec(ctx, `INSERT INTO thread_step_states(id,thread_id,step_name,idempotency_key,status,first_seen_at,last_updated_at) VALUES($1,$2,'step','key','success',$3,$3)`, fmt.Sprintf("state-%s", id), id, old)
		require.NoError(t, err)
		_, err = pg.Pool.Exec(ctx, `INSERT INTO thread_notifications(notification_id,thread_id,step_id,step_name,source,notification_type,message,timestamp) VALUES($1,$2,'step:key','step','validation','step.success','ok',$3)`, fmt.Sprintf("notification-%s", id), id, old)
		require.NoError(t, err)
		_, err = pg.Pool.Exec(ctx, `INSERT INTO step_substeps(id,thread_id,step_id,name,status,recorded_at) VALUES($1,$2,'step:key','operation','success',$3)`, fmt.Sprintf("substep-%s", id), id, old)
		require.NoError(t, err)
	}
	writer := archiver.NewPostgresWriter(pg.Pool, zap.NewNop())
	oldEventTime := old.Format(time.RFC3339)
	_, err = writer.WriteThreadMetadata(ctx, []archiver.StreamEvent{
		{Data: map[string]string{"threadId": "late-completed", "companyId": "retention-a", "status": "active", "createdAt": oldEventTime}},
		{Data: map[string]string{"threadId": "late-completed", "companyId": "retention-a", "status": "completed", "completedAt": oldEventTime}},
	})
	require.NoError(t, err)
	require.NoError(t, cache.Set(ctx, "thread:late-completed", "cached", 0).Err())
	require.NoError(t, cache.Set(ctx, "thread:late-completed:grant:invocation", "cached", 0).Err())
	disabled := retention.NewCleaner(pg.Pool, cache, "retention-a", zap.NewNop())
	deleted, err := disabled.Sweep(ctx, now)
	require.NoError(t, err)
	require.Zero(t, deleted)
	var stillPresent int
	require.NoError(t, pg.Pool.QueryRow(ctx, `SELECT count(*) FROM threads WHERE id='old-completed'`).Scan(&stillPresent))
	require.Equal(t, 1, stillPresent)
	companies := managementrepo.NewCompanyRepository(pg.Pool)
	require.NoError(t, companies.UpdateRetention(ctx, "retention-a", 30))
	company, err := companies.FindByID(ctx, "retention-a")
	require.NoError(t, err)
	require.Equal(t, 30, company.ThreadRetentionDays)
	_, err = pg.Pool.Exec(ctx, `UPDATE companies SET thread_retention_days=-1 WHERE id='retention-a'`)
	require.Error(t, err)
	cleaner := retention.NewCleaner(pg.Pool, cache, "retention-a", zap.NewNop())
	deleted, err = cleaner.Sweep(ctx, now)
	require.NoError(t, err)
	require.Equal(t, 4, deleted)
	deletedIDs, err := retention.DeletedIDs(ctx, pg.Pool, []string{"old-completed", "old-cancelled", "old-closed", "old-failed", "recent-completed"})
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"old-completed": true, "old-cancelled": true, "old-closed": true, "old-failed": true}, deletedIDs)
	for _, id := range []string{"old-completed", "old-cancelled", "old-closed", "old-failed"} {
		var tombstones int
		require.NoError(t, pg.Pool.QueryRow(ctx, `SELECT count(*) FROM thread_retention_tombstones WHERE thread_id=$1 AND company_id='retention-a'`, id).Scan(&tombstones))
		require.Equal(t, 1, tombstones)
		for _, table := range []string{"threads", "thread_activities", "thread_refs", "thread_access", "thread_validations", "thread_step_states", "thread_notifications", "step_substeps"} {
			var count int
			require.NoError(t, pg.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+map[string]string{"threads": "id", "thread_activities": "thread_id", "thread_refs": "thread_id", "thread_access": "thread_id", "thread_validations": "thread_id", "thread_step_states": "thread_id", "thread_notifications": "thread_id", "step_substeps": "thread_id"}[table]+"=$1", id).Scan(&count))
			require.Zero(t, count, table)
		}
		require.EqualValues(t, 0, cache.Exists(ctx, "thread:"+id, "thread:"+id+":grant:invocation").Val())
	}
	for _, id := range []string{"recent-completed", "old-active", "late-completed"} {
		var count int
		require.NoError(t, pg.Pool.QueryRow(ctx, `SELECT count(*) FROM threads WHERE id=$1`, id).Scan(&count))
		require.Equal(t, 1, count)
		require.EqualValues(t, 2, cache.Exists(ctx, "thread:"+id, "thread:"+id+":grant:invocation").Val())
	}
	_, err = pg.Pool.Exec(ctx, `INSERT INTO threads(id,company_id,status,created_at,updated_at,terminal_archived_at)
		VALUES('policy-disabled','retention-a','completed',$1,$1,$1)`, old)
	require.NoError(t, err)
	require.NoError(t, companies.UpdateRetention(ctx, "retention-a", 0))
	deleted, err = cleaner.Sweep(ctx, now)
	require.NoError(t, err)
	require.Zero(t, deleted)
	var preserved int
	require.NoError(t, pg.Pool.QueryRow(ctx, `SELECT count(*) FROM threads WHERE id='policy-disabled'`).Scan(&preserved))
	require.Equal(t, 1, preserved)
	require.NoError(t, companies.UpdateRetention(ctx, "retention-a", 30))
	_, err = pg.Pool.Exec(ctx, `INSERT INTO threads(id,company_id,status,created_at,updated_at,terminal_archived_at)
		VALUES('scheduled-old','retention-a','completed',$1,$1,$1)`, old)
	require.NoError(t, err)
	cleaner.Start()
	require.Eventually(t, func() bool {
		var count int
		return pg.Pool.QueryRow(ctx, `SELECT count(*) FROM threads WHERE id='scheduled-old'`).Scan(&count) == nil && count == 0
	}, 5*time.Second, 50*time.Millisecond)
	require.NoError(t, cleaner.Stop(ctx))
}
