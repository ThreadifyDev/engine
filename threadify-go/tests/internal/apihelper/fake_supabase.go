package apihelper

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type FakeSupabase struct {
	URL    string
	server *httptest.Server

	mu            sync.Mutex
	users         map[string]fakeUser
	revokedTokens map[string]struct{}
	resetTokens   map[string]string

	privateKey *rsa.PrivateKey
	publicJWK  jwkKey
}

type fakeUser struct {
	ID                 string
	Email              string
	Password           string
	ThreadifyUserID    string
	ThreadifyCompanyID string
}

func StartFakeSupabase() *FakeSupabase {
	f := &FakeSupabase{
		users:         map[string]fakeUser{},
		revokedTokens: map[string]struct{}{},
		resetTokens:   map[string]string{},
	}

	if err := f.initSigningKey(); err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/v1/admin/users", f.handleAdminUsers)
	mux.HandleFunc("/auth/v1/token", f.handleToken)
	mux.HandleFunc("/auth/v1/admin/generate_link", f.handleGenerateLink)
	mux.HandleFunc("/auth/v1/verify", f.handleVerify)
	mux.HandleFunc("/auth/v1/logout", f.handleLogout)
	mux.HandleFunc("/auth/v1/recover", f.handleOK)
	mux.HandleFunc("/auth/v1/admin/users/", f.handleAdminUserUpdate)
	mux.HandleFunc("/auth/v1/.well-known/jwks.json", f.handleJWKS)

	srv := httptest.NewServer(mux)
	f.server = srv
	f.URL = srv.URL
	return f
}

func (f *FakeSupabase) Close() {
	if f != nil && f.server != nil {
		f.server.Close()
	}
}

func (f *FakeSupabase) HasUser(email string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.users[strings.ToLower(strings.TrimSpace(email))]
	return ok
}

func (f *FakeSupabase) GetOTP(email string) string {
	return "123456" // Always return a successful mock OTP
}

type AuthToken struct {
	AccessToken string
	UserID      string
	CompanyID   string
}

func (f *FakeSupabase) MintToken(userID, companyID, email string) (string, error) {
	return f.mintAccessToken(fakeUser{
		ID:                 userID,
		Email:              email,
		ThreadifyUserID:    userID,
		ThreadifyCompanyID: companyID,
	})
}

func (f *FakeSupabase) GetResetToken(email string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for token, recipient := range f.resetTokens {
		if recipient == email {
			return token
		}
	}
	return ""
}

func (f *FakeSupabase) handleOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (f *FakeSupabase) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Email        string                 `json:"email"`
			Password     string                 `json:"password"`
			UserMetadata map[string]interface{} `json:"user_metadata"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		email := strings.ToLower(strings.TrimSpace(req.Email))

		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.users[email]; ok {
			http.Error(w, `{"error_code":"email_exists"}`, http.StatusConflict)
			return
		}
		id := uuid.NewString()
		f.users[email] = fakeUser{
			ID:                 id,
			Email:              email,
			Password:           req.Password,
			ThreadifyUserID:    claimStr(req.UserMetadata, "threadify_user_id"),
			ThreadifyCompanyID: claimStr(req.UserMetadata, "threadify_company_id"),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "email": email})
		return
	case http.MethodGet:
		u, _ := url.Parse(r.URL.String())
		email := strings.ToLower(strings.TrimSpace(u.Query().Get("email")))

		f.mu.Lock()
		defer f.mu.Unlock()
		var users []map[string]any
		if email != "" {
			if user, ok := f.users[email]; ok {
				users = append(users, map[string]any{"id": user.ID, "email": user.Email})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (f *FakeSupabase) handleToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	email := strings.ToLower(strings.TrimSpace(req.Email))

	f.mu.Lock()
	user, ok := f.users[email]
	f.mu.Unlock()

	if !ok || user.Password != req.Password {
		http.Error(w, `{"error_code":"invalid_credentials"}`, http.StatusUnauthorized)
		return
	}

	token, err := f.mintAccessToken(user)
	if err != nil {
		http.Error(w, `{"error_code":"server_error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": token,
		"user": map[string]any{
			"id":                 user.ID,
			"email":              user.Email,
			"email_confirmed_at": "",
		},
	})
}

func (f *FakeSupabase) handleGenerateLink(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Type  string `json:"type"`
		Email string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	resetToken := "mock-reset-token"
	if request.Type == "recovery" {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.users[request.Email]; !ok {
			http.Error(w, `{"error_code":"user_not_found"}`, http.StatusNotFound)
			return
		}
		resetToken = uuid.NewString()
		f.resetTokens[resetToken] = request.Email
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"email_otp":     "123456",
		"hashed_token":  resetToken,
		"action_link":   "http://example.com",
		"verification":  true,
		"expires_at":    "",
		"redirect_to":   "",
		"user_metadata": map[string]any{},
	})
}

func (f *FakeSupabase) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token     string `json:"token"`
		TokenHash string `json:"token_hash"`
		Email     string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Token == "000000" || req.Token == "not-a-real-token" || req.TokenHash == "not-a-real-token" || req.TokenHash == "aaaaaaaabbbbbbbbccccccccdddddddd" {
		http.Error(w, `{"error_code":"bad_token","message":"Token is invalid or has expired"}`, http.StatusUnprocessableEntity)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	var user fakeUser
	if req.TokenHash != "" {
		email, ok := f.resetTokens[req.TokenHash]
		if !ok {
			http.Error(w, `{"error_code":"bad_token"}`, http.StatusUnprocessableEntity)
			return
		}
		user = f.users[email]
		delete(f.resetTokens, req.TokenHash)
	} else if req.Email != "" {
		u, ok := f.users[strings.ToLower(strings.TrimSpace(req.Email))]
		if ok {
			user = u
		} else {
			http.Error(w, `{"error_code":"bad_token"}`, http.StatusUnprocessableEntity)
			return
		}
	} else {
		for _, u := range f.users {
			user = u
			break
		}
	}

	if user.ID == "" {
		http.Error(w, `{"error_code":"bad_token"}`, http.StatusUnprocessableEntity)
		return
	}

	token, err := f.mintAccessToken(user)
	if err != nil {
		http.Error(w, `{"error_code":"server_error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": token,
		"user": map[string]any{
			"id":    user.ID,
			"email": user.Email,
		},
	})
}

func (f *FakeSupabase) handleLogout(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		http.Error(w, `{"error_code":"bad_jwt"}`, http.StatusUnauthorized)
		return
	}
	token := parts[1]

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, revoked := f.revokedTokens[token]; revoked {
		http.Error(w, `{"error_code":"bad_jwt","message":"Token has been revoked"}`, http.StatusUnauthorized)
		return
	}
	f.revokedTokens[token] = struct{}{}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (f *FakeSupabase) handleAdminUserUpdate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/auth/v1/admin/users/")
	var update struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&update)
	f.mu.Lock()
	defer f.mu.Unlock()
	for email, user := range f.users {
		if user.ID != id {
			continue
		}
		if update.Password != "" {
			user.Password = update.Password
		}
		if update.Email != "" {
			delete(f.users, email)
			user.Email = update.Email
		}
		f.users[user.Email] = user
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "email": user.Email})
		return
	}
	http.Error(w, `{"error_code":"user_not_found"}`, http.StatusNotFound)
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (f *FakeSupabase) initSigningKey() error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate rsa key: %w", err)
	}

	// `kid` is arbitrary; keep stable for the server lifetime.
	kid := "test-kid-" + uuid.NewString()

	n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	eBytes := []byte(strconv.FormatInt(int64(key.PublicKey.E), 10))
	// Convert exponent integer into big-endian bytes per JWK spec.
	// For common exponent 65537, this results in "AQAB".
	if key.PublicKey.E == 65537 {
		eBytes = []byte{0x01, 0x00, 0x01}
	} else {
		// Fallback: encode as big-endian bytes (rarely used in practice).
		eBytes = intToBigEndianBytes(key.PublicKey.E)
	}
	e := base64.RawURLEncoding.EncodeToString(eBytes)

	f.privateKey = key
	f.publicJWK = jwkKey{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		Use: "sig",
		N:   n,
		E:   e,
	}
	return nil
}

func intToBigEndianBytes(v int) []byte {
	if v == 0 {
		return []byte{0}
	}
	out := []byte{}
	for v > 0 {
		out = append([]byte{byte(v & 0xff)}, out...)
		v >>= 8
	}
	return out
}

func (f *FakeSupabase) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jwksResponse{Keys: []jwkKey{f.publicJWK}})
}

func (f *FakeSupabase) mintAccessToken(user fakeUser) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"iat":   now.Unix(),
		"exp":   now.Add(30 * time.Minute).Unix(),
		"iss":   f.URL,
		"aud":   "authenticated",
		"role":  "owner",
		"user_metadata": map[string]any{
			"threadify_user_id":    user.ThreadifyUserID,
			"threadify_company_id": user.ThreadifyCompanyID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = f.publicJWK.Kid
	return token.SignedString(f.privateKey)
}

func claimStr(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
