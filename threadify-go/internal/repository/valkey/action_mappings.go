package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/database"
	"threadify-go/shared/actionmapping"
)

type BrowserActionMappings struct{ client *redis.Client }

func NewBrowserActionMappings(v *database.ValkeyService) *BrowserActionMappings {
	return &BrowserActionMappings{client: v.Client}
}

func browserActionMappingsKey() string { return "threadify:browser-action-mappings" }

func (s *BrowserActionMappings) Load(ctx context.Context, _ string) (actionmapping.Settings, error) {
	values, err := s.client.HGetAll(ctx, browserActionMappingsKey()).Result()
	if err != nil {
		return actionmapping.Settings{}, err
	}
	result := actionmapping.Settings{Rules: []actionmapping.Rule{}, Revision: values["revision"]}
	if raw, ok := values["rules"]; ok {
		if err := json.Unmarshal([]byte(raw), &result.Rules); err != nil {
			return result, err
		}
		if result.Rules == nil {
			return result, fmt.Errorf("invalid stored action mappings")
		}
		result.Rules, err = actionmapping.Normalize(result.Rules)
		if err != nil {
			return result, err
		}
	} else if result.Revision != "" {
		return result, fmt.Errorf("missing stored action mappings")
	}
	if raw := values["updated_at"]; raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return result, err
		}
		result.UpdatedAt = &t
	}
	return result, nil
}

const saveBrowserActionMappings = `
local revision = redis.call('HGET', KEYS[1], 'revision') or ''
if revision ~= ARGV[1] then return 0 end
redis.call('HSET', KEYS[1], 'revision', ARGV[2], 'rules', ARGV[3], 'updated_at', ARGV[4])
return 1
`

func (s *BrowserActionMappings) Save(ctx context.Context, _ string, revision string, rules []actionmapping.Rule) (actionmapping.Settings, error) {
	normalized, err := actionmapping.Normalize(rules)
	if err != nil {
		return actionmapping.Settings{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return actionmapping.Settings{}, err
	}
	next, now := uuid.NewString(), time.Now().UTC()
	saved, err := s.client.Eval(ctx, saveBrowserActionMappings, []string{browserActionMappingsKey()}, revision, next, string(payload), now.Format(time.RFC3339Nano)).Int()
	if err != nil {
		return actionmapping.Settings{}, err
	}
	if saved != 1 {
		return actionmapping.Settings{}, actionmapping.ErrConflict
	}
	return actionmapping.Settings{Rules: normalized, Revision: next, UpdatedAt: &now}, nil
}
