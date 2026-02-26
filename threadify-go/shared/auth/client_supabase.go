package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeoutSeconds = 10

type SupabaseAuthConfig struct {
	URL                   string
	PublishableKey        string
	SecretKey             string
	RequestTimeoutSeconds int
}

type supabaseClient struct {
	baseURL        string
	publishableKey string
	secretKey      string
	timeout        time.Duration
	httpCli        *http.Client
}

func NewSupabaseClient(cfg SupabaseAuthConfig) (AuthClient, error) {
	url := strings.TrimSpace(cfg.URL)
	pubKey := strings.TrimSpace(cfg.PublishableKey)
	secKey := strings.TrimSpace(cfg.SecretKey)

	switch {
	case url == "":
		return nil, errors.New("supabase: url is required")
	case pubKey == "":
		return nil, errors.New("supabase: publishable_key is required")
	case secKey == "":
		return nil, errors.New("supabase: secret_key is required")
	}

	timeoutSec := cfg.RequestTimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSeconds
	}
	timeout := time.Duration(timeoutSec) * time.Second

	return &supabaseClient{
		baseURL:        strings.TrimRight(url, "/"),
		publishableKey: pubKey,
		secretKey:      secKey,
		timeout:        timeout,
		httpCli:        &http.Client{Timeout: timeout},
	}, nil
}

func (s *supabaseClient) RegisterUser(ctx context.Context, email, password, fullName, localUserID, companyID string) (string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	body := map[string]interface{}{
		"email":         email,
		"password":      password,
		"email_confirm": false,
	}
	if meta := buildUserMetadata(fullName, localUserID, companyID); len(meta) > 0 {
		body["user_metadata"] = meta
	}

	var resp supabaseUserResponse
	if err := s.adminPost(ctx, "/auth/v1/admin/users", body, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			switch {
			case isSupabaseConflict(err):
				id, lookupErr := s.FindUserIDByEmail(ctx, email)
				if lookupErr != nil {
					return "", fmt.Errorf("user exists in auth provider but ID lookup failed: %w", lookupErr)
				}
				if id == "" {
					return "", fmt.Errorf("user exists in auth provider but could not resolve ID for email: %s", email)
				}
				return id, nil
			case isSupabaseInvalidEmail(err):
				return "", ErrAuthInvalidEmail
			default:
				return "", httpErr
			}
		}
		return "", fmt.Errorf("register user: %w", err)
	}

	if resp.ID == "" {
		return "", errors.New("admin create user returned no user id")
	}
	return resp.ID, nil
}

func (s *supabaseClient) LoginWithPassword(ctx context.Context, email, password, _ string) (string, *AuthUserInfo, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	body := map[string]interface{}{"email": email, "password": password}

	var resp supabaseLoginResponse
	if err := s.anonPost(ctx, "/auth/v1/token?grant_type=password", body, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			if isSupabaseUnauthorized(err) {
				return "", nil, ErrAuthInvalidCredentials
			}
			return "", nil, httpErr
		}
		return "", nil, fmt.Errorf("login: %w", err)
	}

	token := strings.TrimSpace(resp.AccessToken)
	if token == "" {
		return "", nil, ErrAuthInvalidCredentials
	}

	info := &AuthUserInfo{
		Sub:           strings.TrimSpace(resp.User.ID),
		Email:         strings.TrimSpace(resp.User.Email),
		EmailVerified: resp.User.EmailConfirmedAt != "",
	}
	return token, info, nil
}

func (s *supabaseClient) Authenticate(ctx context.Context, email, password, clientIP string) error {
	_, _, err := s.LoginWithPassword(ctx, email, password, clientIP)
	return err
}

func (s *supabaseClient) RequestPasswordReset(ctx context.Context, email string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var resp struct{ Message string }
	if err := s.anonPost(ctx, "/auth/v1/recover", map[string]interface{}{"email": email}, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			if isSupabaseInvalidEmail(err) {
				return ErrAuthInvalidEmail
			}
			return httpErr
		}
		return fmt.Errorf("request password reset: %w", err)
	}
	return nil
}

func (s *supabaseClient) GeneratePasswordResetLink(ctx context.Context, email string) (string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	body := map[string]interface{}{"type": "recovery", "email": email}

	var resp supabaseGenerateLinkResponse
	if err := s.adminPost(ctx, "/auth/v1/admin/generate_link", body, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			switch {
			case isSupabaseInvalidEmail(err):
				return "", ErrAuthInvalidEmail
			case isSupabaseNotFound(err):
				return "", ErrAuthUserNotFound
			default:
				return "", httpErr
			}
		}
		return "", fmt.Errorf("generate password reset link: %w", err)
	}

	if resp.HashedToken == "" {
		return "", errors.New("supabase did not return a hashed token")
	}
	return resp.HashedToken, nil
}

func (s *supabaseClient) GenerateSignupLink(ctx context.Context, email string) (string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	body := map[string]interface{}{
		"type":  "signup",
		"email": email,
	}

	var resp supabaseGenerateLinkResponse
	if err := s.adminPost(ctx, "/auth/v1/admin/generate_link", body, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			switch {
			case isSupabaseInvalidEmail(err):
				return "", ErrAuthInvalidEmail
			case isSupabaseConflict(err):
				return "", ErrAuthUserAlreadyExists
			default:
				return "", httpErr
			}
		}
		return "", fmt.Errorf("generate signup link: %w", err)
	}

	if resp.HashedToken == "" {
		return "", errors.New("supabase did not return a hashed token")
	}
	return resp.HashedToken, nil
}

func (s *supabaseClient) ResetPasswordWithToken(ctx context.Context, token, newPassword string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	verifyBody := map[string]interface{}{
		"type":       "recovery",
		"token_hash": token,
	}

	var verifyResp supabaseVerifyResponse
	if err := s.anonPost(ctx, "/auth/v1/verify", verifyBody, &verifyResp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			switch {
			case httpErr.hasCode(supabaseCodeBadToken, supabaseCodeOTPDisabled):
				return ErrAuthInvalidToken
			case httpErr.hasCode(supabaseCodeOTPExpired):
				return ErrAuthExpiredToken
			case isSupabaseRateLimit(err):
				return ErrAuthRateLimit
			}
			return httpErr
		}
		return fmt.Errorf("verify reset token: %w", err)
	}
	if verifyResp.User.ID == "" {
		return errors.New("failed to identify user from reset token")
	}

	_, err := s.UpdatePassword(ctx, verifyResp.User.ID, "", newPassword)
	return err
}

func (s *supabaseClient) VerifyEmailWithToken(ctx context.Context, token string) (string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	verifyBody := map[string]interface{}{
		"type":       "signup",
		"token_hash": token,
	}

	var verifyResp supabaseVerifyResponse
	if err := s.anonPost(ctx, "/auth/v1/verify", verifyBody, &verifyResp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			switch {
			case httpErr.hasCode(supabaseCodeBadToken, supabaseCodeOTPDisabled):
				return "", ErrAuthInvalidToken
			case httpErr.hasCode(supabaseCodeOTPExpired):
				return "", ErrAuthExpiredToken
			case isSupabaseRateLimit(err):
				return "", ErrAuthRateLimit
			}
			return "", httpErr
		}
		return "", fmt.Errorf("verify signup token: %w", err)
	}
	if verifyResp.User.ID == "" {
		return "", errors.New("failed to identify user from verification token")
	}

	return verifyResp.User.ID, nil
}

func (s *supabaseClient) UpdatePassword(ctx context.Context, authUserID, email, newPassword string) (string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	id := strings.TrimSpace(authUserID)
	if id == "" {
		var err error
		if id, err = s.FindUserIDByEmail(ctx, email); err != nil {
			return "", err
		}
	}

	var resp supabaseUserResponse
	if err := s.adminPut(ctx, "/auth/v1/admin/users/"+id, map[string]interface{}{"password": newPassword}, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			return "", httpErr
		}
		return "", fmt.Errorf("update password: %w", err)
	}
	return id, nil
}

func (s *supabaseClient) FindUserIDByEmail(ctx context.Context, email string) (string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	endpoint := fmt.Sprintf("/auth/v1/admin/users?email=%s&page=1&per_page=10", strings.TrimSpace(email))

	var resp supabaseListUsersResponse
	if err := s.adminGet(ctx, endpoint, &resp); err != nil {
		var httpErr *supabaseHTTPError
		if errors.As(err, &httpErr) {
			return "", httpErr
		}
		return "", fmt.Errorf("find user by email: %w", err)
	}

	normalized := strings.TrimSpace(email)
	for _, u := range resp.Users {
		if strings.EqualFold(strings.TrimSpace(u.Email), normalized) && u.ID != "" {
			return u.ID, nil
		}
	}
	return "", ErrAuthUserNotFound
}

func (s *supabaseClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, s.timeout)
}

func (s *supabaseClient) anonPost(ctx context.Context, path string, body, out interface{}) error {
	return s.do(ctx, http.MethodPost, path, s.publishableKey, "", body, out)
}

func (s *supabaseClient) adminPost(ctx context.Context, path string, body, out interface{}) error {
	return s.do(ctx, http.MethodPost, path, s.secretKey, "", body, out)
}

func (s *supabaseClient) adminPut(ctx context.Context, path string, body, out interface{}) error {
	return s.do(ctx, http.MethodPut, path, s.secretKey, "", body, out)
}

func (s *supabaseClient) adminGet(ctx context.Context, path string, out interface{}) error {
	return s.do(ctx, http.MethodGet, path, s.secretKey, "", nil, out)
}

func (s *supabaseClient) bearerGet(ctx context.Context, path, token string, out interface{}) error {
	return s.do(ctx, http.MethodGet, path, s.publishableKey, token, nil, out)
}

func (s *supabaseClient) do(ctx context.Context, method, path, authKey, bearerToken string, body, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("apikey", authKey)
	req.Header.Set("Authorization", "Bearer "+authKey)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return newSupabaseHTTPError(resp.StatusCode, string(respBytes))
	}
	if out != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

type supabaseUserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type supabaseLoginResponse struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID               string `json:"id"`
		Email            string `json:"email"`
		EmailConfirmedAt string `json:"email_confirmed_at"`
	} `json:"user"`
}

type supabaseVerifyResponse struct {
	User struct {
		ID string `json:"id"`
	} `json:"user"`
}

type supabaseGenerateLinkResponse struct {
	ActionLink  string `json:"action_link"`
	HashedToken string `json:"hashed_token"`
}

type supabaseListUsersResponse struct {
	Users []struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"users"`
}

// -----------------------------------------------------------------------------
// Error codes & helpers
// -----------------------------------------------------------------------------

const (
	supabaseCodeInvalidCredentials     = "invalid_credentials"
	supabaseCodeBadJWT                 = "bad_jwt"
	supabaseCodeEmailExists            = "email_exists"
	supabaseCodeUserAlreadyExists      = "user_already_exists"
	supabaseCodeConflict               = "conflict"
	supabaseCodeUserNotFound           = "user_not_found"
	supabaseCodeEmailAddressInvalid    = "email_address_invalid"
	supabaseCodeInvalidEmail           = "invalid_email"
	supabaseCodeOverEmailSendRateLimit = "over_email_send_rate_limit"
	supabaseCodeBadToken               = "bad_token"
	supabaseCodeOTPExpired             = "otp_expired"  // token used after expiry
	supabaseCodeOTPDisabled            = "otp_disabled" // token already consumed / invalid
)

type supabaseHTTPError struct {
	StatusCode int
	Body       string
	Code       string
}

func (e *supabaseHTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("supabase HTTP %d (%s): %s", e.StatusCode, e.Code, e.Body)
	}
	return fmt.Sprintf("supabase HTTP %d: %s", e.StatusCode, e.Body)
}

// newSupabaseHTTPError parses the stable "error_code" field from Supabase's
// JSON error body. Note: Supabase uses "error_code" (string) for the error
// identifier and "code" (integer) for the HTTP status — do not confuse them.
func newSupabaseHTTPError(statusCode int, body string) *supabaseHTTPError {
	e := &supabaseHTTPError{StatusCode: statusCode, Body: body}
	var parsed struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err == nil && parsed.ErrorCode != "" {
		e.Code = parsed.ErrorCode
	}
	return e
}

func (e *supabaseHTTPError) hasCode(codes ...string) bool {
	for _, c := range codes {
		if e.Code == c {
			return true
		}
	}
	return false
}

func isSupabaseUnauthorized(err error) bool {
	var e *supabaseHTTPError
	if !errors.As(err, &e) {
		return false
	}
	return e.hasCode(supabaseCodeInvalidCredentials, supabaseCodeBadJWT) ||
		e.StatusCode == http.StatusUnauthorized ||
		e.StatusCode == http.StatusForbidden
}

func isSupabaseConflict(err error) bool {
	var e *supabaseHTTPError
	if !errors.As(err, &e) {
		return false
	}
	return e.hasCode(supabaseCodeEmailExists, supabaseCodeUserAlreadyExists, supabaseCodeConflict) ||
		e.StatusCode == http.StatusConflict ||
		e.StatusCode == http.StatusUnprocessableEntity
}

func isSupabaseNotFound(err error) bool {
	var e *supabaseHTTPError
	if !errors.As(err, &e) {
		return false
	}
	return e.hasCode(supabaseCodeUserNotFound) || e.StatusCode == http.StatusNotFound
}

func isSupabaseInvalidEmail(err error) bool {
	var e *supabaseHTTPError
	if !errors.As(err, &e) {
		return false
	}
	return e.hasCode(supabaseCodeEmailAddressInvalid, supabaseCodeInvalidEmail)
}

func isSupabaseRateLimit(err error) bool {
	var e *supabaseHTTPError
	if !errors.As(err, &e) {
		return false
	}
	return e.hasCode(supabaseCodeOverEmailSendRateLimit) || e.StatusCode == http.StatusTooManyRequests
}

func buildUserMetadata(fullName, localUserID, companyID string) map[string]interface{} {
	meta := make(map[string]interface{})
	if name := strings.TrimSpace(fullName); name != "" {
		meta["full_name"] = name
	}
	if localUserID != "" {
		meta["threadify_user_id"] = localUserID
	}
	if companyID != "" {
		meta["threadify_company_id"] = companyID
	}
	return meta
}
