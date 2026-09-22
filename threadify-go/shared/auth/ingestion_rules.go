package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"threadify-go/shared/ingestion"
)

// IngestionRulesHandler shares browser CSRF and fresh API-key authentication with Engine settings.
func (s *BrowserService) IngestionRulesHandler(store ingestion.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/v1/engine/ingestion-rules" && path != "/v1/engine/ingestion-rules/preview" {
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
		if path == "/v1/engine/ingestion-rules/preview" {
			if r.Method != http.MethodPost {
				browserError(w, 405, "method_not_allowed")
				return
			}
			var body struct {
				Filters   *[]string `json:"filters"`
				SpanNames []string  `json:"span_names"`
			}
			if !decodeIngestionBody(w, r, &body) || body.Filters == nil {
				browserError(w, 400, "filters_required")
				return
			}
			result, err := ingestion.Preview(*body.Filters, body.SpanNames)
			if err != nil {
				browserError(w, 400, err.Error())
				return
			}
			browserJSON(w, 200, result)
			return
		}
		switch r.Method {
		case http.MethodGet:
		case http.MethodPut:
			var body struct {
				Filters  *[]string `json:"filters"`
				Revision *string   `json:"revision"`
			}
			if !decodeIngestionBody(w, r, &body) || body.Filters == nil || body.Revision == nil {
				browserError(w, 400, "filters_and_revision_required")
				return
			}
			filters, err := ingestion.Normalize(*body.Filters)
			if err != nil {
				browserError(w, 400, err.Error())
				return
			}
			// Serialize with membership changes and recheck administrator status before saving.
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
			result, err := store.Save(r.Context(), actor.CompanyID, *body.Revision, filters)
			if errors.Is(err, ingestion.ErrConflict) {
				browserError(w, 409, err.Error())
				return
			}
			if err != nil {
				browserError(w, 503, "ingestion_rules_unavailable")
				return
			}
			// The transaction holds an authorization lock only; settings are persisted by Valkey.
			browserJSON(w, 200, struct {
				ingestion.Settings
				CanManage bool `json:"can_manage"`
			}{result, true})
			return
		default:
			browserError(w, 405, "method_not_allowed")
			return
		}
		result, err := store.Load(r.Context(), actor.CompanyID)
		if err != nil {
			browserError(w, 503, "ingestion_rules_unavailable")
			return
		}
		browserJSON(w, 200, struct {
			ingestion.Settings
			CanManage bool `json:"can_manage"`
		}{result, userHasRole(actor, "admin")})
	})
}

// Preview accepts bounded sample names as well as up to 100 filter patterns.
func decodeIngestionBody(w http.ResponseWriter, r *http.Request, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	d.DisallowUnknownFields()
	return d.Decode(out) == nil && errors.Is(d.Decode(&struct{}{}), io.EOF)
}
