package auth

import (
	"context"
	"errors"
	"net/http"

	"threadify-go/shared/actionmapping"
)

type ActionMappingValidator func(context.Context, string, actionmapping.Rule) error

// ActionMappingsHandler manages OTel span and auto-captured browser action mappings for this Engine.
func (s *BrowserService) ActionMappingsHandler(store actionmapping.Store, validate ActionMappingValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/v1/engine/browser-action-mappings" && path != "/v1/engine/browser-action-mappings/observed" {
			next.ServeHTTP(w, r)
			return
		}
		actor, err := s.managementActor(r)
		if err != nil || actor == nil {
			browserError(w, 401, "authentication_required")
			return
		}
		if actor.CompanyID != s.registry.CompanyID() {
			browserError(w, 403, "forbidden")
			return
		}
		if path == "/v1/engine/browser-action-mappings/observed" {
			if r.Method != http.MethodGet {
				browserError(w, 405, "method_not_allowed")
				return
			}
			if !userHasRole(actor, "admin") {
				browserError(w, 403, "forbidden")
				return
			}
			rows, err := s.pool.Query(r.Context(), `WITH recent AS (
				SELECT a.payload->>'name' AS name, COALESCE(t.contract_name,'') AS contract,
					COALESCE(t.contract_version,0) AS version, a.recorded_at
				FROM thread_activities a JOIN threads t ON t.id=a.thread_id
				WHERE a.activity_type IN ('browser_action', 'trace_input') AND t.company_id=$1
				ORDER BY a.recorded_at DESC LIMIT 2000
			)
			SELECT name,contract,version,COUNT(*) FROM recent WHERE name IS NOT NULL AND name<>''
			GROUP BY name,contract,version ORDER BY MAX(recorded_at) DESC LIMIT 100`, actor.CompanyID)
			if err != nil {
				browserError(w, 503, "observed_actions_unavailable")
				return
			}
			defer rows.Close()
			actions := make([]struct {
				Name     string `json:"name"`
				Contract string `json:"contract"`
				Version  int    `json:"version"`
				Count    int64  `json:"count"`
			}, 0)
			for rows.Next() {
				var item struct {
					Name     string `json:"name"`
					Contract string `json:"contract"`
					Version  int    `json:"version"`
					Count    int64  `json:"count"`
				}
				if err := rows.Scan(&item.Name, &item.Contract, &item.Version, &item.Count); err != nil {
					browserError(w, 503, "observed_actions_unavailable")
					return
				}
				actions = append(actions, item)
			}
			if rows.Err() != nil {
				browserError(w, 503, "observed_actions_unavailable")
				return
			}
			browserJSON(w, 200, map[string]any{"actions": actions})
			return
		}
		switch r.Method {
		case http.MethodGet:
			result, err := store.Load(r.Context(), actor.CompanyID)
			if err != nil {
				browserError(w, 503, "action_mappings_unavailable")
				return
			}
			browserJSON(w, 200, struct {
				actionmapping.Settings
				CanManage bool `json:"can_manage"`
			}{result, userHasRole(actor, "admin")})
		case http.MethodPut:
			if !userHasRole(actor, "admin") {
				browserError(w, 403, "forbidden")
				return
			}
			var body struct {
				Rules    *[]actionmapping.Rule `json:"rules"`
				Revision *string               `json:"revision"`
			}
			if !decodeIngestionBody(w, r, &body) || body.Rules == nil || body.Revision == nil {
				browserError(w, 400, "rules_and_revision_required")
				return
			}
			rules, err := actionmapping.Normalize(*body.Rules)
			if err != nil {
				browserError(w, 400, err.Error())
				return
			}
			for _, rule := range rules {
				if err := validate(r.Context(), actor.CompanyID, rule); err != nil {
					browserError(w, 400, err.Error())
					return
				}
			}
			tx, err := s.pool.Begin(r.Context())
			if err != nil {
				s.userError(w, err)
				return
			}
			defer tx.Rollback(r.Context())
			if err = s.lockUsers(r.Context(), tx); err == nil {
				err = s.authorizeUserMutation(r.Context(), tx, actor)
			}
			if err != nil {
				s.userError(w, err)
				return
			}
			result, err := store.Save(r.Context(), actor.CompanyID, *body.Revision, rules)
			if errors.Is(err, actionmapping.ErrConflict) {
				browserError(w, 409, err.Error())
				return
			}
			if err != nil {
				browserError(w, 503, "action_mappings_unavailable")
				return
			}
			browserJSON(w, 200, struct {
				actionmapping.Settings
				CanManage bool `json:"can_manage"`
			}{result, true})
		default:
			browserError(w, 405, "method_not_allowed")
		}
	})
}
