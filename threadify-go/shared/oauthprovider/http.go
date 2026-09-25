package oauthprovider

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/Usefused/fused-open-core/oauthserver"
	"github.com/google/uuid"
)

// BrowserActor is resolved by Threadify's existing, revocable browser session.
// OAuth authorization and administration never accept an OAuth bearer token.
type BrowserActor struct {
	UserID         string
	CredentialHash string
	Admin          bool
}

type HTTPServer struct {
	Service      *oauthserver.Service[string]
	Store        *PostgresStore
	Policy       *Policy
	Browser      func(*http.Request) (BrowserActor, error)
	SameOrigin   func(*http.Request) bool
	ValidCSRF    func(*http.Request) bool
	PublicURL    string
	AllowRequest func(*http.Request) bool
}

func (h *HTTPServer) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h == nil || h.Service == nil || h.Policy == nil || h.Browser == nil || h.SameOrigin == nil || h.ValidCSRF == nil {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if h.AllowRequest != nil && (strings.HasPrefix(r.URL.Path, "/oauth/") || strings.HasPrefix(r.URL.Path, "/v1/oauth/")) && !h.AllowRequest(r) {
			w.Header().Set("Retry-After", "60")
			oauthError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}
		switch {
		case r.URL.Path == "/oauth/authorize" && r.Method == http.MethodGet:
			h.authorize(w, r)
		case r.URL.Path == "/oauth/authorize/consent" && r.Method == http.MethodPost:
			h.consent(w, r)
		case r.URL.Path == "/oauth/token" && r.Method == http.MethodPost:
			h.token(w, r)
		case r.URL.Path == "/oauth/revoke" && r.Method == http.MethodPost:
			h.revoke(w, r)
		case r.URL.Path == "/.well-known/oauth-authorization-server" && r.Method == http.MethodGet:
			h.metadata(w, r)
		case r.URL.Path == "/v1/oauth/clients" && r.Method == http.MethodPost:
			h.createClient(w, r)
		case r.URL.Path == "/v1/oauth/clients" && r.Method == http.MethodGet:
			h.clients(w, r)
		case strings.HasPrefix(r.URL.Path, "/v1/oauth/clients/") && r.Method == http.MethodDelete:
			h.revokeClient(w, r)
		case r.URL.Path == "/oauth/connected-apps" && r.Method == http.MethodGet:
			h.connectedApps(w, r)
		case strings.HasPrefix(r.URL.Path, "/oauth/connected-apps/") && r.Method == http.MethodDelete:
			h.disconnect(w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (h *HTTPServer) browserActor(r *http.Request) (oauthserver.Actor[string], bool, error) {
	principal, err := h.Browser(r)
	if err != nil || principal.UserID == "" {
		return oauthserver.Actor[string]{}, false, oauthserver.ErrAuthenticationRequired
	}
	return h.Policy.Actor(principal.UserID, principal.CredentialHash), principal.Admin, nil
}
func jsonReply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func oauthError(w http.ResponseWriter, status int, code string) {
	jsonReply(w, status, map[string]string{"error": code})
}
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}
func (h *HTTPServer) form(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request")
		return false
	}
	return true
}
func parseAuthorize(v url.Values) oauthserver.AuthorizeRequest {
	return oauthserver.AuthorizeRequest{ClientID: v.Get("client_id"), RedirectURI: v.Get("redirect_uri"), ResponseType: v.Get("response_type"), Scope: strings.Fields(v.Get("scope")), State: v.Get("state"), CodeChallenge: v.Get("code_challenge"), CodeChallengeMethod: v.Get("code_challenge_method")}
}

var consentTemplate = template.Must(template.New("threadify-oauth-consent").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect to Threadify</title><style>
:root{font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#172b38;background:#f4f7f8}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px}
main{width:min(100%,520px);background:#fff;border:1px solid #dce5e9;border-radius:18px;padding:36px;box-shadow:0 20px 60px rgba(17,39,51,.09)}
.brand{color:#126b73;font-size:14px;font-weight:800;letter-spacing:.13em;text-transform:uppercase}.eyebrow{margin:28px 0 8px;color:#526a77;font-size:14px;font-weight:700}
h1{font-size:29px;letter-spacing:-.035em;line-height:1.2;margin:0 0 14px}.lead{color:#526a77;line-height:1.55;margin:0 0 26px}
.permission{display:flex;gap:14px;padding:18px;border:1px solid #dce5e9;border-radius:12px;background:#f8fbfb}.icon{display:grid;place-items:center;flex:none;width:28px;height:28px;border-radius:50%;background:#d9f2ed;color:#086754;font-weight:800}
.permission strong{display:block;margin-bottom:5px}.permission span.detail{color:#526a77;font-size:14px;line-height:1.45}.return{color:#526a77;font-size:13px;line-height:1.5;margin:20px 0 26px;overflow-wrap:anywhere}
.actions{display:flex;gap:10px}.actions button{font:inherit;font-weight:700;border-radius:9px;padding:12px 18px;cursor:pointer}.allow{background:#126b73;border:1px solid #126b73;color:#fff;flex:1}.deny{background:#fff;border:1px solid #b8c9d0;color:#172b38;flex:1}
.actions button:focus-visible{outline:3px solid #7ecbce;outline-offset:2px}@media(max-width:560px){main{padding:24px}.actions{flex-direction:column}}
</style></head><body><main><div class="brand">Threadify</div><p class="eyebrow">Connect an application</p>
<h1>{{.Client}} wants to access your account</h1><p class="lead">Review the access this application is requesting.</p>
<div class="permission"><span class="icon" aria-hidden="true">✓</span><div><strong>{{.ScopeLabel}}</strong><span class="detail">{{.ScopeDescription}}</span></div></div>
<p class="return">After you choose Allow, you will return to {{.RedirectHost}}.</p>
<form method="post" action="/oauth/authorize/consent"><input type="hidden" name="client_id" value="{{.ClientID}}"><input type="hidden" name="redirect_uri" value="{{.RedirectURI}}"><input type="hidden" name="scope" value="{{.Scope}}"><input type="hidden" name="state" value="{{.State}}"><input type="hidden" name="code_challenge" value="{{.Challenge}}"><input type="hidden" name="code_challenge_method" value="S256"><div class="actions"><button class="deny" name="decision" value="deny" type="submit">Deny</button><button class="allow" name="decision" value="allow" type="submit">Allow</button></div></form>
</main></body></html>`))

func (h *HTTPServer) authorize(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	actor, _, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	req := parseAuthorize(r.URL.Query())
	result, err := h.Service.Authorize(r.Context(), actor, req)
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	if !result.RequiresConsent {
		http.Redirect(w, r, result.RedirectURL, http.StatusFound)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	redirect, _ := url.Parse(req.RedirectURI)
	scopeLabel, scopeDescription := "Requested access", strings.Join(result.Scope, ", ")
	if len(result.Scope) == 1 && result.Scope[0] == ScopeContractReadAll {
		scopeLabel = "View contracts"
		scopeDescription = "Read your contracts. This application cannot create, edit, or delete them."
	}
	_ = consentTemplate.Execute(w, struct{ Client, ClientID, RedirectURI, RedirectHost, Scope, ScopeLabel, ScopeDescription, State, Challenge string }{
		result.Client.Name, req.ClientID, req.RedirectURI, redirect.Hostname(), strings.Join(result.Scope, " "), scopeLabel, scopeDescription, req.State, req.CodeChallenge,
	})
}
func (h *HTTPServer) consent(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	actor, _, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	if !h.SameOrigin(r) {
		oauthError(w, 403, "access_denied")
		return
	}
	if !h.form(w, r) {
		return
	}
	if r.PostForm.Get("decision") != "allow" {
		if h.Store == nil {
			oauthError(w, 403, "access_denied")
			return
		}
		client, _, lookupErr := h.Store.GetOAuthClientByPublicID(r.Context(), r.PostForm.Get("client_id"))
		redirectURI := r.PostForm.Get("redirect_uri")
		valid := lookupErr == nil && r.PostForm.Get("code_challenge_method") == "S256" && len(r.PostForm.Get("code_challenge")) >= 43
		matched := false
		for _, candidate := range client.RedirectURIs {
			if candidate == redirectURI {
				matched = true
				break
			}
		}
		if !valid || !matched {
			oauthError(w, 400, "invalid_request")
			return
		}
		parsed, _ := url.Parse(redirectURI)
		query := parsed.Query()
		query.Set("error", "access_denied")
		if state := r.PostForm.Get("state"); state != "" {
			query.Set("state", state)
		}
		parsed.RawQuery = query.Encode()
		http.Redirect(w, r, parsed.String(), http.StatusFound)
		return
	}
	redirect, err := h.Service.Consent(r.Context(), actor, oauthserver.ConsentRequest{ClientID: r.PostForm.Get("client_id"), RedirectURI: r.PostForm.Get("redirect_uri"), Scope: strings.Fields(r.PostForm.Get("scope")), State: r.PostForm.Get("state"), CodeChallenge: r.PostForm.Get("code_challenge"), CodeChallengeMethod: r.PostForm.Get("code_challenge_method")})
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}
func clientCredentials(r *http.Request) (string, string) {
	if id, secret, ok := r.BasicAuth(); ok {
		return id, secret
	}
	return r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
}
func protocolCode(err error) string {
	switch {
	case errors.Is(err, oauthserver.ErrInvalidGrant):
		return "invalid_grant"
	case errors.Is(err, oauthserver.ErrUnauthorizedClient):
		return "invalid_client"
	case errors.Is(err, oauthserver.ErrUnsupportedGrantType):
		return "unsupported_grant_type"
	default:
		return "invalid_request"
	}
}
func (h *HTTPServer) token(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if !h.form(w, r) {
		return
	}
	id, secret := clientCredentials(r)
	response, err := h.Service.Token(r.Context(), oauthserver.TokenRequest{GrantType: r.PostForm.Get("grant_type"), Code: r.PostForm.Get("code"), RedirectURI: r.PostForm.Get("redirect_uri"), CodeVerifier: r.PostForm.Get("code_verifier"), RefreshToken: r.PostForm.Get("refresh_token"), ClientID: id, ClientSecret: secret})
	if err != nil {
		oauthError(w, 400, protocolCode(err))
		return
	}
	jsonReply(w, 200, map[string]any{"access_token": response.AccessToken, "refresh_token": response.RefreshToken, "token_type": response.TokenType, "expires_in": response.ExpiresIn, "scope": strings.Join(response.Scope, " ")})
}
func (h *HTTPServer) revoke(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if !h.form(w, r) {
		return
	}
	id, secret := clientCredentials(r)
	err := h.Service.Revoke(r.Context(), oauthserver.RevokeRequest{Token: r.PostForm.Get("token"), ClientID: id, ClientSecret: secret})
	if err != nil {
		oauthError(w, 400, protocolCode(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
func (h *HTTPServer) metadata(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(h.PublicURL, "/")
	jsonReply(w, 200, map[string]any{"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token", "revocation_endpoint": base + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"}})
}
func (h *HTTPServer) createClient(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	actor, admin, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	if !admin {
		oauthError(w, 403, "access_denied")
		return
	}
	if !h.ValidCSRF(r) {
		oauthError(w, 403, "access_denied")
		return
	}
	var body struct {
		Name          string                      `json:"name"`
		ClientType    oauthserver.OAuthClientType `json:"client_type"`
		RedirectURIs  []string                    `json:"redirect_uris"`
		AllowedScopes []string                    `json:"allowed_scopes"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	result, err := h.Service.CreateClient(r.Context(), actor, oauthserver.CreateClientInput{Name: body.Name, ClientType: body.ClientType, RedirectURIs: body.RedirectURIs, AllowedScopes: body.AllowedScopes})
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	jsonReply(w, 201, map[string]any{"client": result.Client, "client_secret": result.ClientSecret})
}
func (h *HTTPServer) clients(w http.ResponseWriter, r *http.Request) {
	_, admin, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	if !admin {
		oauthError(w, 403, "access_denied")
		return
	}
	list, err := h.Service.ListClients(r.Context())
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	jsonReply(w, 200, list)
}
func (h *HTTPServer) connectedApps(w http.ResponseWriter, r *http.Request) {
	actor, _, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	list, err := h.Service.ListConnectedApps(r.Context(), actor)
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	jsonReply(w, 200, list)
}
func (h *HTTPServer) disconnect(w http.ResponseWriter, r *http.Request) {
	actor, _, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	if !h.ValidCSRF(r) {
		oauthError(w, 403, "access_denied")
		return
	}
	id, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, "/oauth/connected-apps/"))
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	if err := h.Service.RevokeConnectedApp(r.Context(), actor, id); err != nil {
		oauthError(w, 404, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPServer) revokeClient(w http.ResponseWriter, r *http.Request) {
	actor, admin, err := h.browserActor(r)
	if err != nil {
		oauthError(w, 401, "authentication_required")
		return
	}
	if !admin || !h.ValidCSRF(r) {
		oauthError(w, 403, "access_denied")
		return
	}
	id, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, "/v1/oauth/clients/"))
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	if err := h.Service.RevokeClient(r.Context(), actor, id); err != nil {
		oauthError(w, 404, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
