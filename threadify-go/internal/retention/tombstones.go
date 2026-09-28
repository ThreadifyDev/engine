package retention

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type tombstoneQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// DeletedIDs lets persistence consumers discard delayed events for purged threads.
func DeletedIDs(ctx context.Context, db tombstoneQuerier, ids []string) (map[string]bool, error) {
	deleted := make(map[string]bool)
	if len(ids) == 0 {
		return deleted, nil
	}
	rows, err := db.Query(ctx, `SELECT thread_id FROM thread_retention_tombstones WHERE thread_id=ANY($1::text[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("check retained thread deletions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		deleted[id] = true
	}
	return deleted, rows.Err()
}
