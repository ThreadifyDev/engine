package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	sharedconfig "threadify-go/shared/config"
)

type EngineSettings struct {
	PublicURL       string            `json:"public_url"`
	ConfigPublicURL string            `json:"config_public_url"`
	Source          string            `json:"source"`
	CanManage       bool              `json:"can_manage"`
	Endpoints       map[string]string `json:"endpoints"`
}

// engineSettings reads the installation override on each request so changes apply across replicas.
func (s *BrowserService) engineSettings(ctx context.Context, actor *TokenClaims, configured string) (EngineSettings, error) {
	if actor == nil || actor.CompanyID != s.registry.CompanyID() {
		return EngineSettings{}, ErrUserDenied
	}
	result := EngineSettings{PublicURL: configured, ConfigPublicURL: configured, Source: "config", CanManage: userHasRole(actor, "admin"), Endpoints: map[string]string{}}
	if configured == "" {
		result.Source = "unset"
	}
	var override string
	err := s.pool.QueryRow(ctx, `SELECT public_url FROM threadify_engine_settings WHERE company_id=$1 AND installation_id=$2`, actor.CompanyID, s.registry.InstallationID()).Scan(&override)
	if err == nil {
		result.PublicURL = override
		result.Source = "ui"
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return EngineSettings{}, err
	}
	if result.PublicURL != "" {
		base := result.PublicURL
		ws := strings.Replace(strings.Replace(base, "https://", "wss://", 1), "http://", "ws://", 1)
		result.Endpoints = map[string]string{"http": base, "websocket": ws + "/threads", "graphql": base + "/graphql", "otel": base + "/v1/traces", "mcp": base + "/mcp"}
	}
	return result, nil
}

// changeEngineURL persists an advertised address only; it never redirects credentials or changes the listener.
func (s *BrowserService) changeEngineURL(ctx context.Context, actor *TokenClaims, value string, reset bool) error {
	normalized, err := sharedconfig.NormalizePublicURL(value)
	if err != nil || (!reset && normalized == "") {
		return ErrInvalidUser
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.lockUsers(ctx, tx); err != nil {
		return err
	}
	if err = s.authorizeUserMutation(ctx, tx, actor); err != nil {
		return err
	}
	if reset {
		_, err = tx.Exec(ctx, `DELETE FROM threadify_engine_settings WHERE company_id=$1 AND installation_id=$2`, actor.CompanyID, s.registry.InstallationID())
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO threadify_engine_settings(company_id,installation_id,public_url,updated_by,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(company_id,installation_id) DO UPDATE SET public_url=EXCLUDED.public_url,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at`, actor.CompanyID, s.registry.InstallationID(), normalized, actor.UserID, s.now().UTC())
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EngineSettingsHandler is mounted by the Engine inside browser CSRF and Registry authorization middleware.
func (s *BrowserService) EngineSettingsHandler(configured string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/engine/settings" {
			next.ServeHTTP(w, r)
			return
		}
		actor, err := s.managementActor(r)
		if err != nil || actor == nil {
			browserError(w, 401, "authentication_required")
			return
		}
		switch r.Method {
		case http.MethodGet:
		case http.MethodPut:
			var body struct {
				PublicURL string `json:"public_url"`
			}
			if !decodeBrowserBody(w, r, &body) {
				browserError(w, 400, "invalid_request")
				return
			}
			err = s.changeEngineURL(r.Context(), actor, body.PublicURL, false)
		case http.MethodDelete:
			err = s.changeEngineURL(r.Context(), actor, "", true)
		default:
			browserError(w, 405, "method_not_allowed")
			return
		}
		if errors.Is(err, ErrInvalidUser) {
			browserError(w, 400, "invalid_public_url")
			return
		}
		if err != nil {
			s.userError(w, err)
			return
		}
		result, err := s.engineSettings(r.Context(), actor, configured)
		if err != nil {
			s.userError(w, err)
			return
		}
		browserJSON(w, 200, result)
	})
}
