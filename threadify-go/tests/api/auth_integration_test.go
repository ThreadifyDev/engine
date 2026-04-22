package api

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"threadify-go/shared/repository"
)

func TestAuth_Signup_Success(t *testing.T) {
	email := uniqueEmail()

	resp := doJSON(t, http.MethodPost, "/api/auth/signup", map[string]any{
		"email":        email,
		"password":     "Password123!@#",
		"full_name":    "Test User",
		"company_name": "Test Company",
	})

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(resp.Body))
	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Signup successful.", body["message"])

	assert.Nil(t, body["token"],
		"signup must not return a token before email verification")
	assert.Nil(t, body["user"],
		"signup must not return user data before email verification")
}

func TestAuth_VerifyOTP_ProvisionsSignupCredits(t *testing.T) {
	email := uniqueEmail()
	password := "Password123!@#"

	signupUser(t, email, password)

	otp := supabase.GetOTP(email)
	require.NotEmpty(t, otp)

	verifyResp := doJSON(t, http.MethodPost, "/api/auth/verify-otp", map[string]any{
		"email": email,
		"token": otp,
	})
	require.Equal(t, http.StatusOK, verifyResp.StatusCode, string(verifyResp.Body))

	body := decodeJSONBody(t, verifyResp)
	userMap := body["user"].(map[string]any)
	companyID := userMap["company_id"].(string)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, env.Postgres.ConnectionString)
	require.NoError(t, err)
	defer pool.Close()

	account, err := repository.NewPlanRepo(pool).GetCreditAccount(ctx, companyID)
	require.NoError(t, err)
	require.NotNil(t, account, "signup verification should create a credit account")
	assert.Equal(t, int64(100_000), account.CreditBalanceMillicents)
	assert.Equal(t, int64(60_000), account.RateLimitTPS)
	assert.Equal(t, int64(1_048_576), account.PayloadLimitBytes)
}

func TestAuth_Signup_DuplicateEmail(t *testing.T) {
	email := uniqueEmail()
	const password = "Password123!@#"

	signupUser(t, email, password)

	second := doJSON(t, http.MethodPost, "/api/auth/signup", map[string]any{
		"email":        email,
		"password":     password,
		"full_name":    "Test User",
		"company_name": "Test Company",
	})
	require.Equal(t, http.StatusConflict, second.StatusCode, string(second.Body))

	body := decodeJSONBody(t, second)
	assert.Equal(t, "user with this email already exists", body["error"])
}

func TestAuth_Signup_JSONDecodingErrors(t *testing.T) {
	tests := []struct {
		name         string
		body         func() []byte
		wantError    string
		errorIsExact bool
	}{
		{
			name:         "missing_body",
			body:         func() []byte { return nil },
			wantError:    "request body",
			errorIsExact: false,
		},
		{
			name: "unknown_fields_rejected",
			body: func() []byte {
				return marshalJSON(t, map[string]any{
					"email":        uniqueEmail(),
					"password":     "Password123!@#",
					"full_name":    "Test User",
					"company_name": "Test Company",
					"unexpected":   "nope",
				})
			},
			wantError:    "request body contains unknown fields",
			errorIsExact: true,
		},
		{
			name:         "multiple_json_objects_rejected",
			body:         func() []byte { return []byte(`{"email":"a@example.com"}{"email":"b@example.com"}`) },
			wantError:    "request body must contain a single JSON object",
			errorIsExact: true,
		},
		{
			name: "wrong_field_type",
			body: func() []byte {
				return []byte(`{"email":"` + uniqueEmail() + `","password":123,"full_name":"Test User","company_name":"Test Company"}`)
			},
			wantError:    "must be of type",
			errorIsExact: false,
		},
		{
			name: "body_too_large",
			body: func() []byte {
				b := make([]byte, 0, (1<<20)+20)
				b = append(b, []byte(`{"email":"`)...)
				b = append(b, bytes.Repeat([]byte("a"), (1<<20)+1)...)
				b = append(b, []byte(`"}`)...)
				return b
			},
			wantError:    "must not exceed",
			errorIsExact: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := doRaw(t, http.MethodPost, "/api/auth/signup", tc.body(), "application/json")
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body := decodeJSONBody(t, resp)
			if tc.errorIsExact {
				assert.Equal(t, tc.wantError, body["error"])
			} else {
				assert.Contains(t, body["error"], tc.wantError)
			}
			assert.Nil(t, body["token"])
			assert.Nil(t, body["user"])
		})
	}
}

func TestAuth_Signup_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "invalid_email",
			payload: map[string]any{
				"email": "not-an-email", "password": "Password123!@#",
				"full_name": "Test User", "company_name": "Test Company",
			},
		},
		{
			name: "company_name_required_without_invitation_token",
			payload: map[string]any{
				"email": uniqueEmail(), "password": "Password123!@#",
				"full_name": "Test User",
			},
		},
		{
			name: "password_too_short",
			payload: map[string]any{
				"email": uniqueEmail(), "password": "short",
				"full_name": "Test User", "company_name": "Test Company",
			},
		},
		{
			name: "password_contains_whitespace",
			payload: map[string]any{
				"email": uniqueEmail(), "password": "Password 123!@#",
				"full_name": "Test User", "company_name": "Test Company",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, "/api/auth/signup", tc.payload)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body := decodeJSONBody(t, resp)
			assert.Equal(t, "Validation failed", body["error"])
			assert.Nil(t, body["token"])
			assert.Nil(t, body["user"])
		})
	}
}

func TestAuth_Login_Success(t *testing.T) {
	email := uniqueEmail()
	const password = "Password123!@#"

	signupUser(t, email, password)
	body := loginUser(t, email, password)

	token, ok := body["token"].(string)
	require.True(t, ok && token != "", "login must return a non-empty token")
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3, "token must be a three-part JWT")

	user, ok := body["user"].(map[string]any)
	require.True(t, ok, "login must return a user object")
	assert.Equal(t, email, user["email"],
		"returned user email must match the login email")
	assert.NotEmpty(t, user["id"],
		"returned user must have an id")
	assert.NotEmpty(t, user["company_id"],
		"returned user must have a company_id")
}

func TestAuth_Login_InvalidCredentials(t *testing.T) {
	const wantError = "invalid email or password"

	tests := []struct {
		name  string
		setup func(t *testing.T) (string, string)
	}{
		{
			name: "unknown_email",
			setup: func(t *testing.T) (string, string) {
				return uniqueEmail(), "DoesNotMatter123!@#"
			},
		},
		{
			name: "wrong_password_for_existing_user",
			setup: func(t *testing.T) (string, string) {
				email := uniqueEmail()
				signupUser(t, email, "Password123!@#")
				// Verify the email to get a verified user
				otp := supabase.GetOTP(email)
				require.NotEmpty(t, otp)
				verifyResp := doJSON(t, http.MethodPost, "/api/auth/verify-otp", map[string]any{
					"email": email,
					"token": otp,
				})
				require.Equal(t, http.StatusOK, verifyResp.StatusCode)
				return email, "WrongPassword123!@#"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			email, password := tc.setup(t)
			resp := doJSON(t, http.MethodPost, "/api/auth/login", map[string]any{
				"email":    email,
				"password": password,
			})
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			body := decodeJSONBody(t, resp)
			assert.Equal(t, wantError, body["error"],
				"error message must be identical for both cases to prevent user enumeration")
			assert.Nil(t, body["token"],
				"failed login must not return a token")
			assert.Nil(t, body["user"],
				"failed login must not return user data")
		})
	}
}

func TestAuth_Login_ValidationErrors(t *testing.T) {
	resp := doJSON(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "not-an-email",
		"password": "Password123!@#",
	})

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Validation failed", body["error"])
	assert.Nil(t, body["token"])
}

func TestAuth_Logout_WithValidToken(t *testing.T) {
	email := uniqueEmail()
	const password = "Password123!@#"

	signupUser(t, email, password)
	loginBody := loginUser(t, email, password)

	token, ok := loginBody["token"].(string)
	require.True(t, ok && token != "", "expected a string token from login")

	resp := doRawWithAuth(t, http.MethodPost, "/api/auth/logout", []byte(`{}`), "application/json", token)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, strings.TrimSpace(string(resp.Body)),
		"logout must return an empty body")
}
func TestAuth_Logout_TokenCannotBeReusedAfterLogout(t *testing.T) {
	email := uniqueEmail()
	const password = "Password123!@#"

	signupUser(t, email, password)
	loginBody := loginUser(t, email, password)

	token, ok := loginBody["token"].(string)
	require.True(t, ok && token != "")

	first := doRawWithAuth(t, http.MethodPost, "/api/auth/logout", []byte(`{}`), "application/json", token)
	require.Equal(t, http.StatusNoContent, first.StatusCode)

	second := doRawWithAuth(t, http.MethodPost, "/api/auth/logout", []byte(`{}`), "application/json", token)
	assert.Equal(t, http.StatusUnauthorized, second.StatusCode,
		"revoked token must not be accepted on subsequent logout requests")

	body := decodeJSONBody(t, second)
	assert.NotEmpty(t, body["error"],
		"revoked token response must include an error message")
}

func TestAuth_Logout_NoBearerToken_IsNoContent(t *testing.T) {
	resp := doRaw(t, http.MethodPost, "/api/auth/logout", []byte(`{}`), "application/json")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, strings.TrimSpace(string(resp.Body)),
		"logout without token must return an empty body")
}

func TestAuth_ForgotPassword_DoesNotLeakUserExistence(t *testing.T) {
	const wantMessage = "If an account exists"

	unknownResp := doJSON(t, http.MethodPost, "/api/auth/forgot-password", map[string]any{
		"email": uniqueEmail(),
	})
	require.Equal(t, http.StatusOK, unknownResp.StatusCode)
	unknownBody := decodeJSONBody(t, unknownResp)

	email := uniqueEmail()
	signupUser(t, email, "Password123!@#")
	knownResp := doJSON(t, http.MethodPost, "/api/auth/forgot-password", map[string]any{
		"email": email,
	})
	require.Equal(t, http.StatusOK, knownResp.StatusCode)
	knownBody := decodeJSONBody(t, knownResp)

	assert.Contains(t, unknownBody["message"], wantMessage)
	assert.Equal(t, unknownBody["message"], knownBody["message"],
		"response must be identical for known and unknown addresses to prevent user enumeration")

	// Neither response must leak user existence through other fields
	assert.Nil(t, unknownBody["user"])
	assert.Nil(t, knownBody["user"])
	assert.Nil(t, unknownBody["token"])
	assert.Nil(t, knownBody["token"])
}

func TestAuth_ResetPassword_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "empty_token",
			payload: map[string]any{"token": "", "password": "Password123!@#"},
		},
		{
			name:    "missing_token_field",
			payload: map[string]any{"password": "Password123!@#"},
		},
		{
			name:    "missing_password_field",
			payload: map[string]any{"token": "sometoken"},
		},
		{
			name:    "password_too_short",
			payload: map[string]any{"token": "sometoken", "password": "short"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, "/api/auth/reset-password", tc.payload)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body := decodeJSONBody(t, resp)
			assert.Equal(t, "Validation failed", body["error"])
			assert.Nil(t, body["token"])
		})
	}
}

func TestAuth_ResetPassword_InvalidOrExpiredToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "random_invalid_token", token: "not-a-real-token"},
		{name: "looks_like_a_token_but_wrong", token: "aaaaaaaabbbbbbbbccccccccdddddddd"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, "/api/auth/reset-password", map[string]any{
				"token":    tc.token,
				"password": "NewPassword123!@#",
			})

			require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
				"invalid reset token must return 422")
			body := decodeJSONBody(t, resp)
			assert.Equal(t, "invalid or already used token", body["error"])
			assert.Nil(t, body["token"],
				"invalid reset must not return a token")
		})
	}
}

func TestAuth_VerifyOTP_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "empty_token",
			payload: map[string]any{"email": uniqueEmail(), "token": ""},
		},
		{
			name:    "missing_token_field",
			payload: map[string]any{"email": uniqueEmail()},
		},
		{
			name:    "missing_email_field",
			payload: map[string]any{"token": "123456"},
		},
		{
			name:    "invalid_email_format",
			payload: map[string]any{"email": "not-an-email", "token": "123456"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, "/api/auth/verify-otp", tc.payload)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body := decodeJSONBody(t, resp)
			assert.Equal(t, "Validation failed", body["error"])
			assert.Nil(t, body["token"],
				"validation failure must not return a token")
		})
	}
}

func TestAuth_VerifyOTP_InvalidToken(t *testing.T) {
	email := uniqueEmail()
	signupUser(t, email, "Password123!@#")

	resp := doJSON(t, http.MethodPost, "/api/auth/verify-otp", map[string]any{
		"email": email,
		"token": "000000",
	})

	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"invalid OTP must return 422")
	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid or already used token", body["error"])
	assert.Nil(t, body["token"])
}
