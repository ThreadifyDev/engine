package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// writeSuccessfulContexts retains the newest validated success independently of
// retry-state deduplication. Late delivery and replay cannot roll a reference back.
func (w *PostgresWriter) writeSuccessfulContexts(ctx context.Context, events []StreamEvent) error {
	type key struct{ thread, step string }
	type success struct {
		event StreamEvent
		order int64
	}
	newest := map[key]success{}
	for _, event := range events {
		if event.Data["successOrder"] == "" {
			continue
		}
		order, err := strconv.ParseInt(event.Data["successOrder"], 10, 64)
		if err != nil || order <= 0 || event.Data["status"] != "success" {
			return fmt.Errorf("invalid successful context order/status")
		}
		var context map[string]string
		if err := json.Unmarshal([]byte(event.Data["successContext"]), &context); err != nil {
			return fmt.Errorf("invalid successful context: %w", err)
		}
		k := key{event.Data["threadId"], event.Data["stepName"]}
		prior, ok := newest[k]
		if !ok || order > prior.order || (order == prior.order && event.Data["stepId"] > prior.event.Data["stepId"]) {
			newest[k] = success{event, order}
		}
	}
	if len(newest) == 0 {
		return nil
	}
	keys := make([]key, 0, len(newest))
	for k := range newest {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].thread != keys[j].thread {
			return keys[i].thread < keys[j].thread
		}
		return keys[i].step < keys[j].step
	})
	rb := newRowBuilder(5)
	for _, k := range keys {
		s := newest[k]
		rb.add(k.thread, k.step, s.event.Data["stepId"], s.order, s.event.Data["successContext"])
	}
	return w.batchExec(ctx, "write successful step contexts", `INSERT INTO thread_successful_contexts(thread_id,step_name,step_id,validation_order,context) VALUES `+rb.placeholders()+`
 ON CONFLICT(thread_id,step_name) DO UPDATE SET step_id=EXCLUDED.step_id, validation_order=EXCLUDED.validation_order, context=EXCLUDED.context
 WHERE (EXCLUDED.validation_order,EXCLUDED.step_id) > (thread_successful_contexts.validation_order,thread_successful_contexts.step_id)`, rb.Values)
}
