package auth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/middleware"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/service/tests/common"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/management/domain"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

// Exercise real HTTP handlers, OTP exchange, signature verification, middleware
// and profile loading. Only the external auth server and repositories are fixtures.
func TestSignupOTPThenProfile(t *testing.T) {
	for _, mode := range []string{"no_jwks", "symmetric_signing", "asymmetric_signing"} {
		t.Run(mode, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			authID := "supabase-new-user"
			user := &domain.User{ID: "local-user", CompanyID: "local-company", Email: "signup@example.invalid", AuthUserID: &authID}
			secret := []byte("test-only-signing-secret-never-used-outside-this-test")
			ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			var signingKey any = secret
			var verificationKey any = secret
			var method jwt.SigningMethod = jwt.SigningMethodHS256
			if mode == "asymmetric_signing" {
				method, signingKey, verificationKey = jwt.SigningMethodES256, ecKey, &ecKey.PublicKey
			}
			var sessionToken, issuer string
			var providerChecks atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/v1/verify":
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["email"] != user.Email || body["token"] != "12345678" || body["type"] != "magiclink" {
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"error_code":"bad_token"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"access_token": sessionToken, "user": map[string]string{"id": authID, "email": user.Email}})
				case "/auth/v1/user":
					providerChecks.Add(1)
					if r.Header.Get("apikey") != "test-publishable-key" {
						w.WriteHeader(401)
						return
					}
					token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					parsed, err := jwt.Parse(token, func(*jwt.Token) (any, error) { return verificationKey, nil }, jwt.WithValidMethods([]string{method.Alg()}), jwt.WithIssuer(issuer), jwt.WithAudience("authenticated"))
					if err != nil || !parsed.Valid {
						w.WriteHeader(401)
						_, _ = w.Write([]byte(`{"error_code":"bad_jwt"}`))
						return
					}
					sub, _ := parsed.Claims.GetSubject()
					_ = json.NewEncoder(w).Encode(map[string]string{"id": sub, "email": user.Email, "email_confirmed_at": time.Now().UTC().Format(time.RFC3339)})
				case "/auth/v1/.well-known/jwks.json":
					keys := []any{}
					if mode == "asymmetric_signing" {
						keys = append(keys, map[string]string{"kty": "EC", "kid": "test-key", "crv": "P-256", "alg": "ES256", "x": base64.RawURLEncoding.EncodeToString(ecKey.X.Bytes()), "y": base64.RawURLEncoding.EncodeToString(ecKey.Y.Bytes())})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
				default:
					w.WriteHeader(404)
				}
			}))
			defer provider.Close()
			issuer = provider.URL + "/auth/v1"
			sign := func(exp time.Time, subject string) string {
				token := jwt.NewWithClaims(method, jwt.MapClaims{
					"sub": subject, "email": user.Email, "iss": issuer, "aud": "authenticated", "exp": exp.Unix(),
					// Identity must come from the DB, not stale/editable metadata.
					"user_metadata": map[string]string{"threadify_user_id": "stale-user", "threadify_company_id": "other-company"},
				})
				token.Header["kid"] = "test-key"
				value, err := token.SignedString(signingKey)
				require.NoError(t, err)
				return value
			}
			sessionToken = sign(time.Now().Add(time.Hour), authID)
			client, err := sharedauth.NewSupabaseClient(sharedauth.SupabaseAuthConfig{URL: provider.URL, PublishableKey: "test-publishable-key", SecretKey: "test-secret-key"})
			require.NoError(t, err)
			var verifier sharedauth.TokenVerifier
			if mode != "no_jwks" {
				verifier = sharedauth.NewJWKSVerifier(issuer+"/.well-known/jwks.json", "authenticated", issuer)
			}
			svc := deps.NewAuthService(nil, client, verifier, nil, []byte(testEncryptionKey))
			// Provisioned accounts verify mail without legacy credit grants.
			svc.ConfigureLicensedAccount(user.CompanyID, user.Email)
			deps.UserRepo.EXPECT().FindByAuthUserID(gomock.Any(), authID).Return(user, nil).AnyTimes()
			deps.UserRepo.EXPECT().FindByAuthUserID(gomock.Any(), "unknown-user").Return(nil, nil).AnyTimes()
			deps.UserRepo.EXPECT().UpdateEmailVerified(gomock.Any(), user.ID, true).Return(nil)
			deps.UserRepo.EXPECT().UpdateLastLogin(gomock.Any(), user.ID).Return(nil)
			welcome := make(chan struct{}, 1)
			deps.EmailSvc.EXPECT().SendWelcomeEmail(gomock.Any(), user.Email, "").DoAndReturn(func(_ any, _, _ string) error { welcome <- struct{}{}; return nil })
			deps.UserRoleRepo.EXPECT().GetUserRoles(gomock.Any(), user.ID).Return([]string{"admin"}, nil).AnyTimes()
			deps.UserRepo.EXPECT().FindByID(gomock.Any(), user.ID).Return(user, nil).AnyTimes()
			deps.CompanyRepo.EXPECT().FindByID(gomock.Any(), user.CompanyID).Return(&domain.Company{ID: user.CompanyID}, nil).AnyTimes()
			deps.APIKeySvc.EXPECT().ValidateAPIKey(gomock.Any(), gomock.Any()).Return(nil, sharedauth.ErrAuthInvalidToken).AnyTimes()
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/api/auth/verify-otp", handlers.NewAuthHandler(svc).VerifyEmail)
			users := service.NewUserService(deps.UserRepo, deps.CompanyRepo, deps.UserRoleRepo, deps.OutboxRepo, nil, nil, deps.Logger)
			router.GET("/api/user/profile", middleware.AuthAccessTokenAuth(svc, deps.APIKeySvc), handlers.NewUserHandler(users).GetProfile)
			otpResponse := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/api/auth/verify-otp", bytes.NewBufferString(`{"email":"signup@example.invalid","token":"12345678"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(otpResponse, request)
			require.Equal(t, 200, otpResponse.Code)
			select {
			case <-welcome:
			case <-time.After(time.Second):
				t.Fatal("welcome email not queued")
			}
			var response struct {
				Token string `json:"token"`
			}
			require.NoError(t, json.Unmarshal(otpResponse.Body.Bytes(), &response))
			require.NotEmpty(t, response.Token)
			for _, tc := range []struct {
				name, token string
				status      int
			}{
				{"new_signup_session", response.Token, 200},
				{"tampered", response.Token + "bad", 401},
				{"expired", sign(time.Now().Add(-time.Hour), authID), 401},
				{"unlinked_identity", sign(time.Now().Add(time.Hour), "unknown-user"), 401},
				{"api_key_not_forwarded", "th_test_only_key", 401},
			} {
				t.Run(tc.name, func(t *testing.T) {
					checksBefore := providerChecks.Load()
					request := httptest.NewRequest("GET", "/api/user/profile?minimal=true", nil)
					request.Header.Set("Authorization", "Bearer "+tc.token)
					result := httptest.NewRecorder()
					router.ServeHTTP(result, request)
					require.Equal(t, tc.status, result.Code, result.Body.String())
					if tc.name == "api_key_not_forwarded" {
						require.Equal(t, checksBefore, providerChecks.Load())
					}
					if tc.status == 200 {
						require.Contains(t, result.Body.String(), user.ID)
						require.NotContains(t, result.Body.String(), "other-company")
					}
				})
			}
		})
	}
}
