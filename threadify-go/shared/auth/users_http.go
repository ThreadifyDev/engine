package auth

import (
	"errors"
	"net/http"
	"strings"
)

// managementActor resolves fresh cookie-session or API-key authority for Engine administration.
func (s *BrowserService) managementActor(r *http.Request) (*TokenClaims, error) {
	if key := r.Header.Get("X-API-Key"); key != "" {
		if r.Header.Get("Authorization") != "" {
			return nil, ErrBrowserAuth
		}
		return s.AuthenticateAPIKey(r.Context(), key)
	}
	token, err := ExtractBearerToken(r.Header.Get("Authorization"))
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(token, sessionPrefix) {
		return s.Authenticate(r.Context(), token)
	}
	return s.AuthenticateAPIKey(r.Context(), token)
}

// UserManagement is mounted only by the Engine; browser cookies and integration keys share local authority.
func (s *BrowserService) UserManagement(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/users" && !strings.HasPrefix(r.URL.Path, "/v1/users/") {
			next.ServeHTTP(w, r)
			return
		}
		actor, err := s.managementActor(r)
		if err != nil || actor == nil {
			browserError(w, 401, "authentication_required")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/users")
		switch {
		case id == "" && r.Method == http.MethodGet:
			users, e := s.ListUsers(r.Context(), actor)
			if e != nil {
				s.userError(w, e)
				return
			}
			browserJSON(w, 200, map[string]any{"users": users, "can_manage": userHasRole(actor, "admin"), "login_url": s.requestOrigin(r) + "/login"})
		case id == "" && r.Method == http.MethodPost:
			var body struct {
				Email    string `json:"email"`
				FullName string `json:"full_name"`
				Role     string `json:"role"`
			}
			if !decodeBrowserBody(w, r, &body) {
				browserError(w, 400, "invalid_request")
				return
			}
			user, created, e := s.CreateInvitedUser(r.Context(), actor, body.Email, body.FullName, body.Role)
			if e != nil {
				s.userError(w, e)
				return
			}
			status := 200
			if created {
				status = 201
			}
			browserJSON(w, status, map[string]any{"user": user, "created": created, "login_url": s.requestOrigin(r) + "/login"})
		case strings.HasPrefix(id, "/") && len(id) > 1 && !strings.Contains(id[1:], "/") && r.Method == http.MethodPatch:
			var body UserChange
			if !decodeBrowserBody(w, r, &body) {
				browserError(w, 400, "invalid_request")
				return
			}
			user, e := s.ChangeUser(r.Context(), actor, id[1:], body)
			if e != nil {
				s.userError(w, e)
				return
			}
			browserJSON(w, 200, map[string]any{"user": user})
		default:
			browserError(w, 405, "method_not_allowed")
		}
	})
}
func (s *BrowserService) userError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUserDenied):
		browserError(w, 403, "user_management_denied")
	case errors.Is(err, ErrUserMissing):
		browserError(w, 404, "user_not_found")
	case errors.Is(err, ErrLastAdmin):
		browserError(w, 409, "last_active_admin_required")
	case errors.Is(err, ErrUserConflict):
		browserError(w, 409, "user_state_conflict")
	case errors.Is(err, ErrInvalidUser):
		browserError(w, 400, "invalid_user")
	default:
		browserError(w, 503, "user_management_unavailable")
	}
}
