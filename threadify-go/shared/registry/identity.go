package registry

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// IdentityTransaction is the product-specific entry into Registry's shared sign-in flow.
type IdentityTransaction struct {
	TransactionID   string    `json:"transaction_id"`
	VerificationURL string    `json:"verification_url"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// IdentityAssertion is exchanged server-to-server and never returned to browser JavaScript.
type IdentityAssertion struct {
	SchemaVersion   int       `json:"schema_version"`
	TransactionID   string    `json:"transaction_id"`
	AccountID       string    `json:"account_id"`
	InstallationID  string    `json:"installation_id"`
	Purpose         string    `json:"purpose"`
	Provider        string    `json:"provider"`
	Issuer          string    `json:"issuer"`
	ExternalSubject string    `json:"external_subject"`
	VerifiedEmail   string    `json:"verified_email"`
	DisplayName     string    `json:"display_name"`
	AuthMethod      string    `json:"auth_method"`
	EnrollmentRef   string    `json:"enrollment_ref"`
	AuthenticatedAt time.Time `json:"authenticated_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	LogoutToken     string    `json:"logout_token,omitempty"`
	LogoutExpiresAt time.Time `json:"logout_expires_at,omitempty"`
}

var ErrIdentityPending = errors.New("identity transaction pending")
var ErrIdentityUnavailable = errors.New("Registry sign-in unavailable")

// BrowserKey derives a separate encryption/cookie key without persisting the license.
func (r *Runtime) BrowserKey() []byte {
	mac := hmac.New(sha256.New, []byte(r.cfg.LicenseKey))
	mac.Write([]byte("threadify-browser-v1:" + r.cfg.InstallationID))
	return mac.Sum(nil)
}
func (r *Runtime) InstallationID() string { return r.cfg.InstallationID }
func (r *Runtime) BrowserOrigin() string  { return r.cfg.BrowserOrigin }

// IsLicenseKey permits the same explicit administrator bootstrap exchange as Fused.
func (r *Runtime) IsLicenseKey(key string) bool {
	a, b := sha256.Sum256([]byte(key)), sha256.Sum256([]byte(r.cfg.LicenseKey))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// StartIdentity uses the Threadify product boundary, not the Fused entitlement gate.
func (r *Runtime) StartIdentity(ctx context.Context, verifier, enrollment string) (IdentityTransaction, error) {
	var out IdentityTransaction
	err := r.identityRequest(ctx, "/transactions", map[string]string{"purpose": "browser_login", "engine_verifier": verifier, "enrollment_ref": enrollment}, &out)
	return out, err
}
func (r *Runtime) ExchangeIdentity(ctx context.Context, id, verifier string) (IdentityAssertion, error) {
	var out IdentityAssertion
	err := r.identityRequest(ctx, "/transactions/"+id+"/exchange", map[string]string{"engine_verifier": verifier}, &out)
	return out, err
}
func (r *Runtime) LogoutIdentity(ctx context.Context, token, returnURL string) (string, error) {
	var out struct {
		URL string `json:"logout_url"`
	}
	err := r.identityRequest(ctx, "/logout", map[string]string{"logout_token": token, "return_url": returnURL}, &out)
	return out.URL, err
}

// identityRequest disables redirects so license credentials cannot cross Registry origins.
func (r *Runtime) identityRequest(ctx context.Context, path string, body, out any) error {
	if _, err := r.Snapshot(); err != nil {
		return err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.cfg.URL, "/")+"/api/threadify/identity"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.cfg.LicenseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Threadify-Installation-ID", r.cfg.InstallationID)
	client := &http.Client{Timeout: 10 * time.Second, Transport: r.client.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return ErrIdentityUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&failure)
		if failure.Error.Code == "transaction_unavailable" {
			return ErrIdentityPending
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrIdentityUnavailable
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(out); err != nil {
		return ErrIdentityUnavailable
	}
	return nil
}
