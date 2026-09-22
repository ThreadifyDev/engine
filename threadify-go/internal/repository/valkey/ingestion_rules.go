package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/database"
	"threadify-go/shared/ingestion"
)

// Rules live without a TTL in the installation's persistent, shared Valkey.
// Reading once per OTLP batch avoids stale replica caches and PostgreSQL on ingestion.
type IngestionRules struct{ client *redis.Client }

func NewIngestionRules(v *database.ValkeyService) *IngestionRules {
	return &IngestionRules{client: v.Client}
}
func ingestionRulesKey(company string) string { return "threadify:ingestion-rules:" + company }
func (s *IngestionRules) Load(ctx context.Context, company string) (ingestion.Settings, error) {
	values, err := s.client.HGetAll(ctx, ingestionRulesKey(company)).Result()
	if err != nil {
		return ingestion.Settings{}, err
	}
	result := ingestion.Settings{Filters: []string{}, Revision: values["revision"]}
	if raw, ok := values["filters"]; ok {
		if err = json.Unmarshal([]byte(raw), &result.Filters); err != nil {
			return result, err
		}
		// A malformed stored policy must never silently admit all spans.
		if result.Filters == nil {
			return result, fmt.Errorf("invalid stored ingestion filters")
		}
		result.Filters, err = ingestion.Normalize(result.Filters)
		if err != nil {
			return result, err
		}
	} else if result.Revision != "" {
		return result, fmt.Errorf("missing stored ingestion filters")
	}
	if raw := values["updated_at"]; raw != "" {
		t, e := time.Parse(time.RFC3339Nano, raw)
		if e != nil {
			return result, e
		}
		result.UpdatedAt = &t
	}
	for key, dest := range map[string]*int64{"evaluated": &result.EvaluatedSpans, "dropped": &result.DroppedSpans} {
		if raw := values[key]; raw != "" {
			*dest, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

const saveIngestionRules = `
local revision = redis.call('HGET', KEYS[1], 'revision') or ''
if revision ~= ARGV[1] then return {0, 0, 0} end
redis.call('HSET', KEYS[1], 'revision', ARGV[2], 'filters', ARGV[3], 'updated_at', ARGV[4])
return {1, tonumber(redis.call('HGET', KEYS[1], 'evaluated') or '0'), tonumber(redis.call('HGET', KEYS[1], 'dropped') or '0')}
`

// Compare-and-set prevents a stale UI tab or CLI file from overwriting another admin's changes.
func (s *IngestionRules) Save(ctx context.Context, company, revision string, filters []string) (ingestion.Settings, error) {
	normalized, err := ingestion.Normalize(filters)
	if err != nil {
		return ingestion.Settings{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return ingestion.Settings{}, err
	}
	next := uuid.NewString()
	now := time.Now().UTC()
	saved, err := s.client.Eval(ctx, saveIngestionRules, []string{ingestionRulesKey(company)}, revision, next, string(payload), now.Format(time.RFC3339Nano)).Int64Slice()
	if err != nil {
		return ingestion.Settings{}, err
	}
	if len(saved) != 3 {
		return ingestion.Settings{}, fmt.Errorf("invalid ingestion rules save response")
	}
	if saved[0] == 0 {
		return ingestion.Settings{}, ingestion.ErrConflict
	}
	// Return this write's revision, even if another administrator saves immediately afterward.
	return ingestion.Settings{Filters: normalized, Revision: next, UpdatedAt: &now, EvaluatedSpans: saved[1], DroppedSpans: saved[2]}, nil
}

// Counters describe evaluated delivery attempts, including retries, rather than unique archived spans.
func (s *IngestionRules) Record(ctx context.Context, company string, evaluated, dropped int) error {
	pipe := s.client.TxPipeline()
	pipe.HIncrBy(ctx, ingestionRulesKey(company), "evaluated", int64(evaluated))
	pipe.HIncrBy(ctx, ingestionRulesKey(company), "dropped", int64(dropped))
	_, err := pipe.Exec(ctx)
	return err
}
