package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

type successfulContentCache interface {
	HGet(context.Context, string, string) (string, error)
}
type successfulContentDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// SuccessfulContentRepository reads immutable successful values rather than the
// mutable retry state. Callers must first authorize access to the bound thread.
type SuccessfulContentRepository struct {
	cache successfulContentCache
	db    successfulContentDB
}

func NewSuccessfulContentRepository(cache successfulContentCache, db successfulContentDB) *SuccessfulContentRepository {
	return &SuccessfulContentRepository{cache: cache, db: db}
}

// GetSuccessfulContent never falls back to stale disk data on a cache error, and
// never searches an older success when the newest success lacks the requested field.
func (r *SuccessfulContentRepository) GetSuccessfulContent(ctx context.Context, threadID, stepName string) (map[string]string, error) {
	if threadID == "" || stepName == "" || r.cache == nil {
		return nil, fmt.Errorf("successful step lookup is unavailable")
	}
	raw, err := r.cache.HGet(ctx, "thread:"+threadID+":successful_contexts", stepName)
	if err == nil {
		var snapshot struct {
			StepID  string `json:"stepID"`
			Order   string `json:"order"`
			Context string `json:"context"`
		}
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return nil, fmt.Errorf("invalid successful step snapshot")
		}
		order, err := strconv.ParseInt(snapshot.Order, 10, 64)
		if err != nil || order <= 0 || snapshot.StepID == "" {
			return nil, fmt.Errorf("invalid successful step snapshot")
		}
		return decodeSuccessfulContent(snapshot.Context)
	}
	if !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("successful step lookup is unavailable")
	}
	if r.db == nil {
		return nil, fmt.Errorf("step %q has not completed successfully", stepName)
	}
	var content string
	err = r.db.QueryRow(ctx, `SELECT context::text FROM thread_successful_contexts WHERE thread_id=$1 AND step_name=$2`, threadID, stepName).Scan(&content)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("step %q has not completed successfully", stepName)
	}
	if err != nil {
		return nil, fmt.Errorf("successful step lookup is unavailable")
	}
	return decodeSuccessfulContent(content)
}

// Keep original wire strings, including JSON-looking values and numeric precision.
func decodeSuccessfulContent(raw string) (map[string]string, error) {
	var content map[string]string
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return nil, fmt.Errorf("invalid successful step content")
	}
	return content, nil
}
