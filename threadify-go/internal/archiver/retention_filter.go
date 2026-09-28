package archiver

import (
	"context"

	"github.com/threadify/engine/internal/retention"
)

func (c *NATSConsumer) excludeRetainedEvents(ctx context.Context, events []StreamEvent) ([]StreamEvent, error) {
	if !c.retentionGuard || len(events) == 0 {
		return events, nil
	}
	ids := make([]string, 0, len(events))
	for _, event := range events {
		id := event.Data["threadId"]
		if id == "" {
			id = event.Data["threadID"]
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	deleted, err := retention.DeletedIDs(ctx, c.db, ids)
	if err != nil {
		return nil, err
	}
	kept := events[:0]
	for _, event := range events {
		id := event.Data["threadId"]
		if id == "" {
			id = event.Data["threadID"]
		}
		if !deleted[id] {
			kept = append(kept, event)
		}
	}
	return kept, nil
}

func (c *NATSConsumer) excludeRetainedSubSteps(ctx context.Context, subSteps []map[string]interface{}) ([]map[string]interface{}, error) {
	if !c.retentionGuard || len(subSteps) == 0 {
		return subSteps, nil
	}
	ids := make([]string, 0, len(subSteps))
	for _, step := range subSteps {
		id, _ := step["threadId"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	deleted, err := retention.DeletedIDs(ctx, c.db, ids)
	if err != nil {
		return nil, err
	}
	kept := subSteps[:0]
	for _, step := range subSteps {
		id, _ := step["threadId"].(string)
		if !deleted[id] {
			kept = append(kept, step)
		}
	}
	return kept, nil
}
