package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"threadify-go/shared/registry"
)

// Authenticate resolves current roles and source-key state on every request, so revocation is immediate.
func (s *BrowserService) Authenticate(ctx context.Context, token string) (*TokenClaims, error) {
	if !strings.HasPrefix(token, sessionPrefix) || len(token) != len(sessionPrefix)+43 {
		return nil, ErrBrowserAuth
	}
	if _, err := s.registry.Snapshot(); err != nil {
		return nil, ErrBrowserAuth
	}
	var id, kind string
	var expiry time.Time
	err := s.pool.QueryRow(ctx, `SELECT b.principal_id,b.principal_type,b.expires_at FROM threadify_browser_sessions b
 WHERE b.token_hash=$1 AND b.company_id=$2 AND b.installation_id=$3 AND b.revoked_at IS NULL AND b.expires_at>$4 AND b.auth_generation=$5
 AND (b.source_key_id IS NULL OR EXISTS(SELECT 1 FROM api_keys k WHERE k.id=b.source_key_id AND k.company_id=b.company_id AND k.is_active AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>($4::timestamptz AT TIME ZONE current_setting('TimeZone')))))`, browserHash(token), s.registry.CompanyID(), s.registry.InstallationID(), s.now().UTC(), s.signed("generation", "v1")).Scan(&id, &kind, &expiry)
	if err != nil {
		return nil, ErrBrowserAuth
	}
	return s.principalClaims(ctx, id, kind, expiry)
}

// principalClaims resolves the current local user status and roles for every session request.
func (s *BrowserService) principalClaims(ctx context.Context, id, kind string, expiry time.Time) (*TokenClaims, error) {
	var err error
	claims := &TokenClaims{UserID: id, Sub: id, CompanyID: s.registry.CompanyID(), ExpiresAt: expiry, PrincipalType: kind}
	if kind == "user" {
		err = s.pool.QueryRow(ctx, `SELECT email,email_verified FROM users WHERE id=$1 AND company_id=$2 AND status='active'`, id, claims.CompanyID).Scan(&claims.Email, &claims.EmailVerified)
	} else {
		var active bool
		err = s.pool.QueryRow(ctx, `SELECT is_active FROM service_accounts WHERE id=$1 AND company_id=$2`, id, claims.CompanyID).Scan(&active)
		if !active {
			return nil, ErrBrowserAuth
		}
	}
	if err != nil {
		return nil, ErrBrowserAuth
	}
	rows, err := s.pool.Query(ctx, `SELECT role_name FROM user_roles WHERE principal_id=$1 AND principal_type=$2`, id, kind)
	if err != nil {
		return nil, ErrBrowserAuth
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if rows.Scan(&role) != nil {
			return nil, ErrBrowserAuth
		}
		claims.Roles = append(claims.Roles, role)
	}
	if rows.Err() != nil || len(claims.Roles) == 0 {
		return nil, ErrBrowserAuth
	}
	return claims, nil
}

// issueSession stores only a token hash and encrypted provider logout capability.
func (s *BrowserService) issueSession(ctx context.Context, tx pgx.Tx, id, kind, source, keyID, logout string, logoutExpiry *time.Time) (string, time.Time, error) {
	token := sessionPrefix + randomBrowserToken()
	expiry := s.now().UTC().Add(8 * time.Hour)
	encrypted := ""
	var err error
	if logout != "" {
		encrypted, err = s.encrypt(logout)
		if err != nil {
			return "", time.Time{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO threadify_browser_sessions(token_hash,company_id,installation_id,principal_id,principal_type,source,source_key_id,expires_at,logout_ciphertext,logout_expires_at,auth_generation) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11)`, browserHash(token), s.registry.CompanyID(), s.registry.InstallationID(), id, kind, source, keyID, expiry, encrypted, logoutExpiry, s.signed("generation", "v1"))
	return token, expiry, err
}

// ExchangeKey preserves the key's principal; service keys never become account administrators.
func (s *BrowserService) ExchangeKey(ctx context.Context, key string) (string, time.Time, error) {
	if len(key) > 8192 || key == "" {
		return "", time.Time{}, ErrBrowserAuth
	}
	if _, err := s.registry.Snapshot(); err != nil {
		return "", time.Time{}, ErrBrowserAuth
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.lockUsers(ctx, tx); err != nil {
		return "", time.Time{}, err
	}
	var id, kind, keyID, source string
	if s.registry.IsLicenseKey(key) {
		id, err = s.resolveUser(ctx, tx, s.registry.OwnerEmail(), "", true)
		kind = "user"
		source = "license_exchange"
	} else {
		err = tx.QueryRow(ctx, `SELECT k.id,COALESCE(k.service_account_id,k.user_id),CASE WHEN k.service_account_id IS NOT NULL THEN 'service_account' ELSE 'user' END FROM api_keys k WHERE k.key_hash=$1 AND k.company_id=$2 AND k.is_active AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>($3::timestamptz AT TIME ZONE current_setting('TimeZone'))) FOR SHARE`, browserHash(key), s.registry.CompanyID(), s.now().UTC()).Scan(&keyID, &id, &kind)
		source = "api_key_exchange"
		if err == nil && kind == "service_account" {
			var active bool
			err = tx.QueryRow(ctx, `SELECT is_active FROM service_accounts WHERE id=$1 AND company_id=$2`, id, s.registry.CompanyID()).Scan(&active)
			if !active {
				err = ErrBrowserAuth
			}
		}
	}
	if err != nil {
		return "", time.Time{}, ErrBrowserAuth
	}
	if kind == "user" {
		var active bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND company_id=$2 AND status='active')`, id, s.registry.CompanyID()).Scan(&active)
		if err != nil || !active {
			return "", time.Time{}, ErrBrowserAuth
		}
	}
	var member bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles WHERE principal_id=$1 AND principal_type=$2)`, id, kind).Scan(&member)
	if err != nil || !member {
		return "", time.Time{}, ErrBrowserAuth
	}
	token, expiry, err := s.issueSession(ctx, tx, id, kind, source, keyID, "", nil)
	if err != nil {
		return "", time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", time.Time{}, err
	}
	return token, expiry, nil
}

// resolveUser activates a pre-created invitation only after Registry has verified the matching identity.
func (s *BrowserService) resolveUser(ctx context.Context, tx pgx.Tx, email, name string, bootstrap bool) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", ErrBrowserAuth
	}
	if err := s.lockUsers(ctx, tx); err != nil {
		return "", err
	}
	var id, company, status string
	err := tx.QueryRow(ctx, `SELECT id,COALESCE(company_id,''),status FROM users WHERE lower(email)=$1 FOR UPDATE`, email).Scan(&id, &company, &status)
	owner := strings.EqualFold(email, s.registry.OwnerEmail())
	if errors.Is(err, pgx.ErrNoRows) {
		if !owner {
			return "", ErrBrowserAuth
		}
		id = newUserID()
		_, err = tx.Exec(ctx, `INSERT INTO users(id,company_id,email,full_name,status,email_verified,onboarding_completed,first_instrumentation_done) VALUES($1,$2,$3,$4,'invited',false,true,true)`, id, s.registry.CompanyID(), email, name)
		if err != nil {
			return "", err
		}
		company = s.registry.CompanyID()
		status = "invited"
	} else if err != nil {
		return "", err
	}
	if company != s.registry.CompanyID() || status == "suspended" || status == "archived" {
		return "", ErrBrowserAuth
	}
	if !owner && (bootstrap || status != "invited") {
		return "", ErrBrowserAuth
	}
	if owner {
		_, err = tx.Exec(ctx, `INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'user','admin',$1) ON CONFLICT DO NOTHING`, id)
		if err != nil {
			return "", err
		}
	}
	var member bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles WHERE principal_id=$1 AND principal_type='user')`, id).Scan(&member)
	if err != nil || !member {
		return "", ErrBrowserAuth
	}
	if status == "invited" {
		_, err = tx.Exec(ctx, `UPDATE users SET status='active',email_verified=true WHERE id=$1`, id)
		if err != nil {
			return "", err
		}
		if err = s.auditUser(ctx, tx, id, id, "user.activated"); err != nil {
			return "", err
		}
	}
	return id, nil
}

func newUserID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6], b[8] = (b[6]&15)|64, (b[8]&63)|128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// StartLogin persists the verifier encrypted and returns only the browser polling capability.
func (s *BrowserService) StartLogin(ctx context.Context) (map[string]any, error) {
	id, poll, verifier := randomBrowserToken(), randomBrowserToken(), randomBrowserToken()
	remote, err := s.registry.StartIdentity(ctx, verifier, id)
	if err != nil {
		return nil, err
	}
	if remote.TransactionID == "" || strings.ContainsAny(remote.TransactionID, "/\\?#") || !validBrowserURL(remote.VerificationURL, false) || !remote.ExpiresAt.After(s.now().UTC()) || remote.ExpiresAt.After(s.now().UTC().Add(15*time.Minute)) {
		return nil, ErrBrowserAuth
	}
	ciphertext, err := s.encrypt(verifier)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO threadify_browser_logins(id,registry_id,poll_hash,verifier_ciphertext,company_id,installation_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, remote.TransactionID, browserHash(poll), ciphertext, s.registry.CompanyID(), s.registry.InstallationID(), remote.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{"transaction_id": id, "poll_token": poll, "verification_url": remote.VerificationURL, "expires_at": remote.ExpiresAt}, nil
}

// PollLogin consumes a browser-bound assertion once, including across concurrent Engine replicas.
func (s *BrowserService) PollLogin(ctx context.Context, id, poll string) (tokenResult string, expiryResult time.Time, resultErr error) {
	stage := "transaction"
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, registry.ErrIdentityPending) {
			resultErr = &managedLoginError{stage: stage, cause: resultErr}
		}
	}()
	if len(id) != 43 || len(poll) != 43 {
		return "", time.Time{}, ErrBrowserAuth
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	var remote, ciphertext string
	var expiry time.Time
	err = tx.QueryRow(ctx, `SELECT registry_id,verifier_ciphertext,expires_at FROM threadify_browser_logins WHERE id=$1 AND poll_hash=$2 AND company_id=$3 AND installation_id=$4 AND expires_at>$5 AND consumed_at IS NULL FOR UPDATE`, id, browserHash(poll), s.registry.CompanyID(), s.registry.InstallationID(), s.now().UTC()).Scan(&remote, &ciphertext, &expiry)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", time.Time{}, ErrBrowserAuth
		}
		return "", time.Time{}, err
	}
	stage = "verifier"
	verifier, err := s.decrypt(ciphertext)
	if err != nil {
		return "", time.Time{}, err
	}
	stage = "registry_exchange"
	a, err := s.registry.ExchangeIdentity(ctx, remote, verifier)
	if err != nil {
		return "", time.Time{}, err
	}
	stage = "assertion"
	if !s.validAssertion(a, remote, id, expiry) {
		return "", time.Time{}, ErrBrowserAuth
	}
	stage = "membership"
	if err = s.lockUsers(ctx, tx); err != nil {
		return "", time.Time{}, err
	}
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM threadify_managed_identities WHERE company_id=$1 AND issuer=$2 AND subject=$3`, s.registry.CompanyID(), a.Issuer, a.ExternalSubject).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		userID, err = s.resolveUser(ctx, tx, a.VerifiedEmail, a.DisplayName, false)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO threadify_managed_identities(company_id,issuer,subject,user_id) VALUES($1,$2,$3,$4)`, s.registry.CompanyID(), a.Issuer, a.ExternalSubject, userID)
		}
	}
	if err != nil {
		return "", time.Time{}, err
	}
	// A known provider identity still needs current membership; email alone never restores a removed role.
	var member bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles r ON r.principal_id=u.id AND r.principal_type='user' WHERE u.id=$1 AND u.company_id=$2 AND u.status='active')`, userID, s.registry.CompanyID()).Scan(&member)
	if err != nil {
		return "", time.Time{}, err
	}
	if !member {
		return "", time.Time{}, ErrBrowserAuth
	}
	stage = "session"
	token, sessionExpiry, err := s.issueSession(ctx, tx, userID, "user", "managed_login", "", a.LogoutToken, &a.LogoutExpiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE threadify_browser_logins SET consumed_at=$2,verifier_ciphertext='' WHERE id=$1`, id, s.now().UTC())
	if err != nil {
		return "", time.Time{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET last_login_at=$2,email_verified=true WHERE id=$1`, userID, s.now().UTC())
	if err != nil {
		return "", time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", time.Time{}, err
	}
	return token, sessionExpiry, nil
}

// validAssertion checks both installation binding and the Registry's bounded identity contract.
func (s *BrowserService) validAssertion(a registry.IdentityAssertion, remote, local string, expiry time.Time) bool {
	now := s.now().UTC()
	bounded := func(v string, max int) bool { return v != "" && len(v) <= max && strings.TrimSpace(v) == v }
	validMethod := false
	switch a.AuthMethod {
	case "email_code", "google", "microsoft", "enterprise_sso", "oidc":
		validMethod = true
	}
	logoutValid := (a.LogoutToken == "") == a.LogoutExpiresAt.IsZero()
	if a.LogoutToken != "" {
		logoutValid = bounded(a.LogoutToken, 256) && a.LogoutExpiresAt.After(now) && !a.LogoutExpiresAt.After(now.Add(24*time.Hour))
	}
	return a.SchemaVersion == 1 && a.TransactionID == remote && a.EnrollmentRef == local && a.AccountID == s.registry.AccountID() && a.InstallationID == s.registry.InstallationID() && a.Purpose == "browser_login" && a.Provider == "logto" && validBrowserURL(a.Issuer, false) && bounded(a.Issuer, 512) && bounded(a.ExternalSubject, 512) && bounded(a.VerifiedEmail, 320) && strings.Contains(a.VerifiedEmail, "@") && len(a.DisplayName) <= 256 && validMethod && !a.AuthenticatedAt.IsZero() && !a.AuthenticatedAt.After(now.Add(time.Minute)) && a.AuthenticatedAt.After(now.Add(-15*time.Minute)) && a.ExpiresAt.After(now) && !a.ExpiresAt.After(expiry.Add(time.Second)) && logoutValid
}
