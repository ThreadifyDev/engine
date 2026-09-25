package oauthprovider

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/Usefused/fused-open-core/oauthserver"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is Threadify's durable, installation-scoped implementation of
// the shared OAuth protocol store. The raw credential values never reach it.
type PostgresStore struct {
	pool           *pgxpool.Pool
	companyID      string
	installationID string
}

var _ oauthserver.Store[string] = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool, companyID, installationID string) (*PostgresStore, error) {
	if pool == nil || companyID == "" || installationID == "" {
		return nil, errors.New("invalid Threadify OAuth store configuration")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('threadify:oauth:schema',0))`); err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
CREATE SEQUENCE IF NOT EXISTS threadify_oauth_revision_seq;
CREATE TABLE IF NOT EXISTS threadify_oauth_clients (
 id uuid PRIMARY KEY, company_id text NOT NULL, installation_id text NOT NULL,
 client_id text NOT NULL UNIQUE, name text NOT NULL, client_type text NOT NULL CHECK(client_type IN ('public','confidential')),
 client_secret_hash text, redirect_uris text[] NOT NULL, allowed_scopes text[] NOT NULL,
 created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz,
 CHECK((client_type='public' AND client_secret_hash IS NULL) OR (client_type='confidential' AND client_secret_hash IS NOT NULL))
);
CREATE TABLE IF NOT EXISTS threadify_oauth_consents (
 client_id uuid NOT NULL REFERENCES threadify_oauth_clients(id) ON DELETE CASCADE,
 subject_id text NOT NULL, granted_scope text[] NOT NULL, granted_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(client_id,subject_id)
);
CREATE TABLE IF NOT EXISTS threadify_oauth_codes (
 id uuid PRIMARY KEY, client_id uuid NOT NULL REFERENCES threadify_oauth_clients(id) ON DELETE CASCADE,
 subject_id text NOT NULL, redirect_uri text NOT NULL, scope text[] NOT NULL,
 code_hash text NOT NULL UNIQUE, code_challenge text NOT NULL,
 expires_at timestamptz NOT NULL, consumed_at timestamptz
);
CREATE INDEX IF NOT EXISTS threadify_oauth_codes_expiry ON threadify_oauth_codes(expires_at);
CREATE TABLE IF NOT EXISTS threadify_oauth_tokens (
 id uuid PRIMARY KEY, client_id uuid NOT NULL REFERENCES threadify_oauth_clients(id) ON DELETE CASCADE,
 subject_id text NOT NULL, family_id uuid NOT NULL, scope text[] NOT NULL,
 access_hash text NOT NULL UNIQUE, refresh_hash text NOT NULL UNIQUE,
 access_expires_at timestamptz NOT NULL, refresh_expires_at timestamptz NOT NULL,
 consumed_at timestamptz, revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS threadify_oauth_tokens_expiry ON threadify_oauth_tokens(refresh_expires_at);
CREATE INDEX IF NOT EXISTS threadify_oauth_tokens_family ON threadify_oauth_tokens(family_id);
`)
	if err != nil {
		return nil, fmt.Errorf("initialize Threadify OAuth schema: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool, companyID: companyID, installationID: installationID}, nil
}

func (s *PostgresStore) revision(ctx context.Context, tx pgx.Tx) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `SELECT nextval('threadify_oauth_revision_seq')`).Scan(&revision)
	return revision, err
}

func scanClient(row pgx.Row) (oauthserver.OAuthClient, *string, error) {
	var client oauthserver.OAuthClient
	var secret *string
	err := row.Scan(&client.ID, &client.ClientID, &client.Name, &client.ClientType, &secret, &client.RedirectURIs, &client.AllowedScopes, &client.CreatedAt, &client.RevokedAt)
	client.HasSecret = secret != nil
	return client, secret, err
}

const clientColumns = `id,client_id,name,client_type,client_secret_hash,redirect_uris,allowed_scopes,created_at,revoked_at`

func (s *PostgresStore) CreateOAuthClient(ctx context.Context, input oauthserver.OAuthClientRegistration[string]) (oauthserver.OAuthClientMutationResult, error) {
	if strings.TrimSpace(input.Name) == "" || len(input.RedirectURIs) == 0 || len(input.AllowedScopes) == 0 || input.Actor.SubjectID == "" ||
		(input.ClientType != oauthserver.OAuthClientPublic && input.ClientType != oauthserver.OAuthClientConfidential) {
		return oauthserver.OAuthClientMutationResult{}, oauthserver.ErrInvalidOAuthClient
	}
	if (input.ClientType == oauthserver.OAuthClientConfidential) != (input.ClientSecretHash != "") {
		return oauthserver.OAuthClientMutationResult{}, oauthserver.ErrInvalidOAuthClient
	}
	if len(input.Name) > 120 || !strings.HasPrefix(input.ClientID, Prefixes.ClientID) || !validRedirects(input.RedirectURIs) {
		return oauthserver.OAuthClientMutationResult{}, oauthserver.ErrInvalidOAuthClient
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return oauthserver.OAuthClientMutationResult{}, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	client, _, err := scanClient(tx.QueryRow(ctx, `INSERT INTO threadify_oauth_clients(id,company_id,installation_id,client_id,name,client_type,client_secret_hash,redirect_uris,allowed_scopes,created_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+clientColumns,
		id, s.companyID, s.installationID, input.ClientID, input.Name, input.ClientType, nullable(input.ClientSecretHash), input.RedirectURIs, input.AllowedScopes, input.Actor.SubjectID))
	if err != nil {
		return oauthserver.OAuthClientMutationResult{}, err
	}
	rev, err := s.revision(ctx, tx)
	if err != nil {
		return oauthserver.OAuthClientMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return oauthserver.OAuthClientMutationResult{}, err
	}
	return oauthserver.OAuthClientMutationResult{Client: client, AuthorizationRevision: rev}, nil
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *PostgresStore) ListOAuthClients(ctx context.Context) ([]oauthserver.OAuthClient, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+clientColumns+` FROM threadify_oauth_clients WHERE company_id=$1 AND installation_id=$2 ORDER BY created_at DESC`, s.companyID, s.installationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clients := []oauthserver.OAuthClient{}
	for rows.Next() {
		c, _, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

func (s *PostgresStore) GetOAuthClientByPublicID(ctx context.Context, publicID string) (oauthserver.OAuthClient, string, error) {
	c, hash, err := scanClient(s.pool.QueryRow(ctx, `SELECT `+clientColumns+` FROM threadify_oauth_clients WHERE client_id=$1 AND company_id=$2 AND installation_id=$3 AND revoked_at IS NULL`, publicID, s.companyID, s.installationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return oauthserver.OAuthClient{}, "", oauthserver.ErrOAuthClientNotFound
	}
	if err != nil {
		return oauthserver.OAuthClient{}, "", err
	}
	if hash == nil {
		return c, "", nil
	}
	return c, *hash, nil
}

func (s *PostgresStore) RevokeOAuthClient(ctx context.Context, id uuid.UUID, _ oauthserver.MutationActor[string]) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `UPDATE threadify_oauth_clients SET revoked_at=now() WHERE id=$1 AND company_id=$2 AND installation_id=$3 AND revoked_at IS NULL`, id, s.companyID, s.installationID)
	if err != nil {
		return 0, err
	}
	if cmd.RowsAffected() != 1 {
		return 0, oauthserver.ErrOAuthClientNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE threadify_oauth_tokens SET revoked_at=now() WHERE client_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return 0, err
	}
	rev, err := s.revision(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return rev, nil
}

func (s *PostgresStore) GetOAuthUserConsent(ctx context.Context, clientID uuid.UUID, subjectID string) (oauthserver.OAuthConsent[string], bool, error) {
	var c oauthserver.OAuthConsent[string]
	err := s.pool.QueryRow(ctx, `SELECT x.client_id,x.subject_id,x.granted_scope,x.granted_at FROM threadify_oauth_consents x JOIN threadify_oauth_clients client ON client.id=x.client_id WHERE x.client_id=$1 AND x.subject_id=$2 AND client.company_id=$3 AND client.installation_id=$4 AND client.revoked_at IS NULL`, clientID, subjectID, s.companyID, s.installationID).Scan(&c.ClientID, &c.SubjectID, &c.GrantedScope, &c.GrantedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return oauthserver.OAuthConsent[string]{}, false, nil
	}
	return c, err == nil, err
}

func (s *PostgresStore) RecordOAuthConsentAndIssueCode(ctx context.Context, consent oauthserver.OAuthConsentGrant[string], code oauthserver.OAuthAuthorizationCodeIssue[string]) error {
	if consent.ClientID != code.ClientID || consent.SubjectID != code.SubjectID || code.CodeHash == "" || code.CodeChallenge == "" {
		return oauthserver.ErrOAuthGrantDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, `SELECT revoked_at IS NULL FROM threadify_oauth_clients WHERE id=$1 AND company_id=$2 AND installation_id=$3 FOR SHARE`, code.ClientID, s.companyID, s.installationID).Scan(&active)
	if err != nil || !active {
		return oauthserver.ErrOAuthGrantDenied
	}
	_, err = tx.Exec(ctx, `INSERT INTO threadify_oauth_consents(client_id,subject_id,granted_scope) VALUES($1,$2,$3) ON CONFLICT(client_id,subject_id) DO UPDATE SET granted_scope=excluded.granted_scope,granted_at=now()`, consent.ClientID, consent.SubjectID, consent.GrantedScope)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO threadify_oauth_codes(id,client_id,subject_id,redirect_uri,scope,code_hash,code_challenge,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, code.ID, code.ClientID, code.SubjectID, code.RedirectURI, code.Scope, code.CodeHash, code.CodeChallenge, code.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validVerifier(challenge, verifier string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	digest := sha256.Sum256([]byte(verifier))
	encoded := base64.RawURLEncoding.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(challenge), []byte(encoded)) == 1
}

func secretHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func insertToken(ctx context.Context, tx pgx.Tx, clientID uuid.UUID, subjectID string, familyID uuid.UUID, scope []string, issue oauthserver.OAuthTokenIssue) (oauthserver.OAuthTokenMetadata[string], error) {
	var m oauthserver.OAuthTokenMetadata[string]
	err := tx.QueryRow(ctx, `INSERT INTO threadify_oauth_tokens(id,client_id,subject_id,family_id,scope,access_hash,refresh_hash,access_expires_at,refresh_expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,client_id,subject_id,family_id,scope,access_expires_at,refresh_expires_at`, uuid.New(), clientID, subjectID, familyID, scope, issue.AccessTokenHash, issue.RefreshTokenHash, issue.AccessExpiresAt, issue.RefreshExpiresAt).Scan(&m.ID, &m.ClientID, &m.SubjectID, &m.TokenFamilyID, &m.Scope, &m.AccessExpiresAt, &m.RefreshExpiresAt)
	return m, err
}

func (s *PostgresStore) ExchangeOAuthAuthorizationCode(ctx context.Context, input oauthserver.OAuthCodeExchange, at time.Time) (oauthserver.OAuthTokenMetadata[string], error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	defer tx.Rollback(ctx)
	var id, clientID uuid.UUID
	var subjectID, redirectURI, challenge string
	var scope []string
	var expires time.Time
	var consumed *time.Time
	err = tx.QueryRow(ctx, `SELECT code.id,code.client_id,code.subject_id,code.redirect_uri,code.scope,code.code_challenge,code.expires_at,code.consumed_at
 FROM threadify_oauth_codes code JOIN threadify_oauth_clients client ON client.id=code.client_id AND client.revoked_at IS NULL
 WHERE code.code_hash=$1 AND client.company_id=$2 AND client.installation_id=$3 FOR UPDATE OF code`, input.CodeHash, s.companyID, s.installationID).Scan(&id, &clientID, &subjectID, &redirectURI, &scope, &challenge, &expires, &consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	if consumed != nil || !expires.After(at) || clientID != input.ClientID || redirectURI != input.RedirectURI || !validVerifier(challenge, input.CodeVerifier) {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	cmd, err := tx.Exec(ctx, `UPDATE threadify_oauth_codes SET consumed_at=$2 WHERE id=$1 AND consumed_at IS NULL`, id, at)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	if cmd.RowsAffected() != 1 {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	m, err := insertToken(ctx, tx, clientID, subjectID, uuid.New(), scope, input.Issue)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	m.AuthorizationRevision, err = s.revision(ctx, tx)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	return m, nil
}

func (s *PostgresStore) RotateOAuthRefreshToken(ctx context.Context, input oauthserver.OAuthRefreshExchange, at time.Time) (oauthserver.OAuthTokenMetadata[string], error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	defer tx.Rollback(ctx)
	var id, clientID, familyID uuid.UUID
	var subjectID string
	var scope []string
	var consumed, revoked *time.Time
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT token.id,token.client_id,token.subject_id,token.family_id,token.scope,token.consumed_at,token.revoked_at,token.refresh_expires_at
 FROM threadify_oauth_tokens token JOIN threadify_oauth_clients client ON client.id=token.client_id AND client.revoked_at IS NULL
 WHERE token.refresh_hash=$1 AND client.company_id=$2 AND client.installation_id=$3 FOR UPDATE OF token`, input.RefreshTokenHash, s.companyID, s.installationID).Scan(&id, &clientID, &subjectID, &familyID, &scope, &consumed, &revoked, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	if clientID != input.ClientID || revoked != nil || !expires.After(at) {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	if consumed != nil {
		if _, err := tx.Exec(ctx, `UPDATE threadify_oauth_tokens SET revoked_at=$2 WHERE family_id=$1 AND revoked_at IS NULL`, familyID, at); err != nil {
			return oauthserver.OAuthTokenMetadata[string]{}, err
		}
		if _, err := s.revision(ctx, tx); err != nil {
			return oauthserver.OAuthTokenMetadata[string]{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return oauthserver.OAuthTokenMetadata[string]{}, err
		}
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantReuseDetected
	}
	cmd, err := tx.Exec(ctx, `UPDATE threadify_oauth_tokens SET consumed_at=$2 WHERE id=$1 AND consumed_at IS NULL`, id, at)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	if cmd.RowsAffected() != 1 {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	m, err := insertToken(ctx, tx, clientID, subjectID, familyID, scope, input.Issue)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	m.AuthorizationRevision, err = s.revision(ctx, tx)
	if err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return oauthserver.OAuthTokenMetadata[string]{}, err
	}
	return m, nil
}

func (s *PostgresStore) RevokeOAuthTokenByHash(ctx context.Context, clientID uuid.UUID, hash string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var familyID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT token.family_id FROM threadify_oauth_tokens token JOIN threadify_oauth_clients client ON client.id=token.client_id WHERE token.client_id=$1 AND client.company_id=$2 AND client.installation_id=$3 AND (token.access_hash=$4 OR token.refresh_hash=$4) AND token.revoked_at IS NULL FOR UPDATE OF token`, clientID, s.companyID, s.installationID, hash).Scan(&familyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE threadify_oauth_tokens SET revoked_at=now() WHERE family_id=$1 AND revoked_at IS NULL`, familyID); err != nil {
		return false, err
	}
	if _, err := s.revision(ctx, tx); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PostgresStore) ListOAuthConnectedApps(ctx context.Context, subjectID string) ([]oauthserver.OAuthConnectedApp, error) {
	rows, err := s.pool.Query(ctx, `SELECT consent.client_id,client.name,consent.granted_scope,consent.granted_at FROM threadify_oauth_consents consent JOIN threadify_oauth_clients client ON client.id=consent.client_id AND client.revoked_at IS NULL WHERE consent.subject_id=$1 AND client.company_id=$2 AND client.installation_id=$3 ORDER BY consent.granted_at DESC`, subjectID, s.companyID, s.installationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	apps := []oauthserver.OAuthConnectedApp{}
	for rows.Next() {
		var app oauthserver.OAuthConnectedApp
		if err := rows.Scan(&app.ClientID, &app.ClientName, &app.GrantedScope, &app.GrantedAt); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (s *PostgresStore) RevokeOAuthConnectedApp(ctx context.Context, clientID uuid.UUID, subjectID string, _ oauthserver.MutationActor[string]) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `DELETE FROM threadify_oauth_consents consent USING threadify_oauth_clients client WHERE consent.client_id=client.id AND client.id=$1 AND consent.subject_id=$2 AND client.company_id=$3 AND client.installation_id=$4`, clientID, subjectID, s.companyID, s.installationID)
	if err != nil {
		return 0, err
	}
	if cmd.RowsAffected() != 1 {
		return 0, oauthserver.ErrOAuthConsentNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE threadify_oauth_tokens SET revoked_at=now() WHERE client_id=$1 AND subject_id=$2 AND revoked_at IS NULL`, clientID, subjectID); err != nil {
		return 0, err
	}
	rev, err := s.revision(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return rev, nil
}

func (s *PostgresStore) ExpireOAuthArtifacts(ctx context.Context, at time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, errors.New("positive cleanup limit required")
	}
	var count int
	err := s.pool.QueryRow(ctx, `WITH due_codes AS(SELECT code.id FROM threadify_oauth_codes code JOIN threadify_oauth_clients client ON client.id=code.client_id WHERE client.company_id=$1 AND client.installation_id=$2 AND code.expires_at<=$3 LIMIT $4 FOR UPDATE OF code SKIP LOCKED), deleted_codes AS(DELETE FROM threadify_oauth_codes code USING due_codes d WHERE code.id=d.id RETURNING code.id), due_tokens AS(SELECT token.id FROM threadify_oauth_tokens token JOIN threadify_oauth_clients client ON client.id=token.client_id WHERE client.company_id=$1 AND client.installation_id=$2 AND token.refresh_expires_at<=$3 LIMIT $4 FOR UPDATE OF token SKIP LOCKED), deleted_tokens AS(DELETE FROM threadify_oauth_tokens token USING due_tokens d WHERE token.id=d.id RETURNING token.id) SELECT (SELECT count(*) FROM deleted_codes)+(SELECT count(*) FROM deleted_tokens)`, s.companyID, s.installationID, at, limit).Scan(&count)
	return count, err
}

// AccessGrant checks a presented access token against the live row. Callers
// must then apply Policy.CanUse for the requested API permission.
func (s *PostgresStore) AccessGrant(ctx context.Context, rawToken string) (string, []string, error) {
	if !strings.HasPrefix(rawToken, Prefixes.AccessToken) || len(rawToken) > 256 {
		return "", nil, oauthserver.ErrInvalidGrant
	}
	var userID string
	var scope []string
	err := s.pool.QueryRow(ctx, `SELECT token.subject_id,token.scope FROM threadify_oauth_tokens token JOIN threadify_oauth_clients client ON client.id=token.client_id AND client.revoked_at IS NULL WHERE token.access_hash=$1 AND token.revoked_at IS NULL AND token.access_expires_at>now() AND client.company_id=$2 AND client.installation_id=$3`, secretHash(rawToken), s.companyID, s.installationID).Scan(&userID, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, oauthserver.ErrInvalidGrant
	}
	return userID, scope, err
}

func validRedirects(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, raw := range values {
		parsed, err := url.Parse(raw)
		if err != nil || raw == "" || strings.TrimSpace(raw) != raw || parsed.Host == "" || parsed.Opaque != "" || parsed.User != nil || parsed.Fragment != "" || seen[raw] {
			return false
		}
		loopback := parsed.Hostname() == "localhost"
		if ip := net.ParseIP(parsed.Hostname()); ip != nil {
			loopback = ip.IsLoopback()
		}
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
			return false
		}
		seen[raw] = true
	}
	return len(seen) > 0
}
