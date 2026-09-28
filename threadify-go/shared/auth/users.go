package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrUserConflict = errors.New("user_state_conflict")
var ErrUserDenied = errors.New("user_management_denied")
var ErrUserMissing = errors.New("user_not_found")
var ErrLastAdmin = errors.New("last_active_admin_required")
var ErrInvalidUser = errors.New("invalid_user")

type EngineUser struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	Status   string   `json:"status"`
	Roles    []string `json:"roles"`
}
type UserChange struct {
	FullName *string `json:"full_name,omitempty"`
	Role     *string `json:"role,omitempty"`
	Status   *string `json:"status,omitempty"`
}

func userRoleValid(role string) bool { return role == "admin" || role == "member" || role == "viewer" }
func userHasRole(actor *TokenClaims, role string) bool {
	for _, r := range actor.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// lockUsers serializes grants and status transitions across Engine replicas, including first sign-in.
func (s *BrowserService) lockUsers(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, s.registry.CompanyID()+":browser-users")
	return err
}

// authorizeUserMutation rechecks actor authority after taking the lifecycle lock.
func (s *BrowserService) authorizeUserMutation(ctx context.Context, tx pgx.Tx, actor *TokenClaims) error {
	if actor == nil || actor.CompanyID != s.registry.CompanyID() {
		return ErrUserDenied
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles r WHERE r.principal_id=$1 AND r.principal_type=$2 AND r.role_name='admin'
 AND (($2='user' AND EXISTS(SELECT 1 FROM users WHERE id=$1 AND company_id=$3 AND status='active')) OR ($2='service_account' AND EXISTS(SELECT 1 FROM service_accounts WHERE id=$1 AND company_id=$3 AND is_active))))`, actor.UserID, actor.PrincipalType, actor.CompanyID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrUserDenied
	}
	return nil
}

// ListUsers exposes the installation's local principals and their actual lifecycle states.
func (s *BrowserService) ListUsers(ctx context.Context, actor *TokenClaims) ([]EngineUser, error) {
	if actor == nil || actor.CompanyID != s.registry.CompanyID() || !(userHasRole(actor, "admin") || actor.PrincipalType == "user" && (userHasRole(actor, "member") || userHasRole(actor, "viewer"))) {
		return nil, ErrUserDenied
	}
	rows, err := s.pool.Query(ctx, `SELECT u.id,u.email,COALESCE(u.full_name,''),u.status,COALESCE(array_agg(r.role_name ORDER BY r.role_name) FILTER(WHERE r.role_name IS NOT NULL),ARRAY[]::varchar[]) FROM users u LEFT JOIN user_roles r ON r.principal_id=u.id AND r.principal_type='user' WHERE u.company_id=$1 GROUP BY u.id ORDER BY lower(u.email),u.id`, actor.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []EngineUser{}
	for rows.Next() {
		var u EngineUser
		if err = rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.Roles); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// CreateInvitedUser reserves an email as a local invited principal; repeated identical invitations are idempotent.
func (s *BrowserService) CreateInvitedUser(ctx context.Context, actor *TokenClaims, email, name, role string) (EngineUser, bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 255 || len(name) > 255 || !userRoleValid(role) {
		return EngineUser{}, false, ErrInvalidUser
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EngineUser{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = s.lockUsers(ctx, tx); err != nil {
		return EngineUser{}, false, err
	}
	if err = s.authorizeUserMutation(ctx, tx, actor); err != nil {
		return EngineUser{}, false, err
	}
	var u EngineUser
	var company string
	err = tx.QueryRow(ctx, `SELECT id,email,COALESCE(full_name,''),status,COALESCE(company_id,'') FROM users WHERE lower(email)=$1 FOR UPDATE`, email).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &company)
	if err == nil {
		if company != actor.CompanyID || u.Status != "invited" {
			return EngineUser{}, false, ErrUserConflict
		}
		var existingRole string
		err = tx.QueryRow(ctx, `SELECT role_name FROM user_roles WHERE principal_id=$1 AND principal_type='user'`, u.ID).Scan(&existingRole)
		if err != nil {
			return EngineUser{}, false, err
		}
		if existingRole != role || u.FullName != name {
			return EngineUser{}, false, ErrUserConflict
		}
		u.Roles = []string{existingRole}
		return u, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EngineUser{}, false, err
	}
	u = EngineUser{ID: newUserID(), Email: email, FullName: name, Status: "invited", Roles: []string{role}}
	_, err = tx.Exec(ctx, `INSERT INTO users(id,company_id,email,full_name,status,email_verified,onboarding_completed,first_instrumentation_done) VALUES($1,$2,$3,$4,'invited',false,true,true)`, u.ID, actor.CompanyID, u.Email, u.FullName)
	if err != nil {
		return EngineUser{}, false, ErrUserConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'user',$2,$3)`, u.ID, role, actor.UserID)
	if err != nil {
		return EngineUser{}, false, err
	}
	if err = s.auditUser(ctx, tx, actor.UserID, u.ID, "user.invited"); err != nil {
		return EngineUser{}, false, err
	}
	return u, true, tx.Commit(ctx)
}

// ChangeUser never activates an invitation or resurrects an archived identity; sign-in owns first activation.
func (s *BrowserService) ChangeUser(ctx context.Context, actor *TokenClaims, id string, change UserChange) (EngineUser, error) {
	if change.FullName == nil && change.Role == nil && change.Status == nil {
		return EngineUser{}, ErrInvalidUser
	}
	if change.FullName != nil && len(strings.TrimSpace(*change.FullName)) > 255 {
		return EngineUser{}, ErrInvalidUser
	}
	if change.Role != nil && !userRoleValid(*change.Role) {
		return EngineUser{}, ErrInvalidUser
	}
	if change.Status != nil && *change.Status != "active" && *change.Status != "suspended" && *change.Status != "archived" {
		return EngineUser{}, ErrInvalidUser
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EngineUser{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.lockUsers(ctx, tx); err != nil {
		return EngineUser{}, err
	}
	if err = s.authorizeUserMutation(ctx, tx, actor); err != nil {
		return EngineUser{}, err
	}
	var u EngineUser
	err = tx.QueryRow(ctx, `SELECT id,email,COALESCE(full_name,''),status FROM users WHERE id=$1 AND company_id=$2 FOR UPDATE`, id, actor.CompanyID).Scan(&u.ID, &u.Email, &u.FullName, &u.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return EngineUser{}, ErrUserMissing
	}
	if err != nil {
		return EngineUser{}, err
	}
	if u.Status == "archived" {
		return EngineUser{}, ErrUserConflict
	}
	if change.Status != nil {
		if *change.Status == "active" && u.Status != "suspended" && u.Status != "active" {
			return EngineUser{}, ErrUserConflict
		}
		if *change.Status == "suspended" && u.Status != "active" && u.Status != "suspended" {
			return EngineUser{}, ErrUserConflict
		}
		u.Status = *change.Status
	}
	if change.FullName != nil {
		u.FullName = strings.TrimSpace(*change.FullName)
	}
	_, err = tx.Exec(ctx, `UPDATE users SET full_name=$2,status=$3 WHERE id=$1`, id, u.FullName, u.Status)
	if err != nil {
		return EngineUser{}, err
	}
	if change.Role != nil {
		_, err = tx.Exec(ctx, `DELETE FROM user_roles WHERE principal_id=$1 AND principal_type='user'`, id)
		if err != nil {
			return EngineUser{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'user',$2,$3)`, id, *change.Role, actor.UserID)
		if err != nil {
			return EngineUser{}, err
		}
	}
	var admins int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM users u JOIN user_roles r ON r.principal_id=u.id AND r.principal_type='user' AND r.role_name='admin' WHERE u.company_id=$1 AND u.status='active'`, actor.CompanyID).Scan(&admins)
	if err != nil {
		return EngineUser{}, err
	}
	if admins == 0 {
		return EngineUser{}, ErrLastAdmin
	}
	// Status and role changes invalidate existing sessions; reactivation requires a new session.
	if change.Status != nil || change.Role != nil {
		_, err = tx.Exec(ctx, `UPDATE threadify_browser_sessions SET revoked_at=$2 WHERE principal_id=$1 AND principal_type='user' AND revoked_at IS NULL`, id, s.now().UTC())
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE threadify_cli_credentials SET revoked_at=$2 WHERE principal_id=$1 AND principal_type='user' AND revoked_at IS NULL`, id, s.now().UTC())
		}
		if err != nil {
			return EngineUser{}, err
		}
	}
	if u.Status == "archived" {
		_, err = tx.Exec(ctx, `UPDATE api_keys SET is_active=false,revoked_at=$2 WHERE user_id=$1 AND service_account_id IS NULL`, id, s.now().UTC())
		if err != nil {
			return EngineUser{}, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT role_name FROM user_roles WHERE principal_id=$1 AND principal_type='user' ORDER BY role_name`, id)
	if err != nil {
		return EngineUser{}, err
	}
	u.Roles = []string{}
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			rows.Close()
			return EngineUser{}, err
		}
		u.Roles = append(u.Roles, role)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return EngineUser{}, err
	}
	if err = s.auditUser(ctx, tx, actor.UserID, id, "user.updated:"+u.Status); err != nil {
		return EngineUser{}, err
	}
	return u, tx.Commit(ctx)
}
func (s *BrowserService) auditUser(ctx context.Context, tx pgx.Tx, actor, id, action string) error {
	_, err := tx.Exec(ctx, `INSERT INTO threadify_user_audit(company_id,actor_id,user_id,action,created_at) VALUES($1,$2,$3,$4,$5)`, s.registry.CompanyID(), actor, id, action, s.now().UTC())
	return err
}

// RequireActiveUser is shared by the external API so suspension cannot be bypassed with a raw user key.
func RequireActiveUser(ctx context.Context, id, company string) error {
	s := currentBrowser.Load()
	if s == nil {
		return nil
	}
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND company_id=$2 AND status='active')`, id, company).Scan(&active)
	if err != nil || !active {
		return ErrBrowserAuth
	}
	return nil
}

// AuthenticateAPIKey resolves fresh key and principal state for Engine user management.
func (s *BrowserService) AuthenticateAPIKey(ctx context.Context, key string) (*TokenClaims, error) {
	if strings.HasPrefix(key, CLICredentialPrefix) {
		return s.authenticateCLI(ctx, key)
	}
	if len(key) == 0 || len(key) > 8192 {
		return nil, ErrBrowserAuth
	}
	if _, err := s.registry.Snapshot(); err != nil {
		return nil, ErrBrowserAuth
	}
	var id, kind string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(service_account_id,user_id),CASE WHEN service_account_id IS NOT NULL THEN 'service_account' ELSE 'user' END FROM api_keys WHERE key_hash=$1 AND company_id=$2 AND is_active AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>($3::timestamptz AT TIME ZONE current_setting('TimeZone')))`, browserHash(key), s.registry.CompanyID(), s.now().UTC()).Scan(&id, &kind)
	if err != nil {
		return nil, ErrBrowserAuth
	}
	return s.principalClaims(ctx, id, kind, s.now().Add(time.Hour))
}
