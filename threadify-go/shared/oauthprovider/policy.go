package oauthprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/Usefused/fused-open-core/oauthserver"
	"github.com/jackc/pgx/v5/pgxpool"
	"threadify-go/shared/rbac"
)

// RoleSource resolves a user's current active membership and roles. It returns
// an error for a suspended, archived, invited, or missing user.
type RoleSource func(context.Context, string) ([]string, error)

// DatabaseRoleSource reads roles at authorization time, constrained to one
// Threadify company. OAuth grants are for human users, never service accounts.
func DatabaseRoleSource(pool *pgxpool.Pool, companyID string) (RoleSource, error) {
	if pool == nil || companyID == "" {
		return nil, errors.New("invalid Threadify role source")
	}
	return func(ctx context.Context, userID string) ([]string, error) {
		if userID == "" {
			return nil, errors.New("active Threadify membership required")
		}
		rows, err := pool.Query(ctx, `SELECT r.role_name FROM users u JOIN user_roles r ON r.principal_id=u.id AND r.principal_type='user' WHERE u.id=$1 AND u.company_id=$2 AND u.status='active'`, userID, companyID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var roles []string
		for rows.Next() {
			var role string
			if err := rows.Scan(&role); err != nil {
				return nil, err
			}
			roles = append(roles, role)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			return nil, errors.New("active Threadify membership required")
		}
		return roles, nil
	}, nil
}

// Policy maps live Threadify role permissions to delegated OAuth scopes. Its
// allowlist is smaller than the full app permission set so admin permissions
// cannot become delegable merely by changing a role.
type Policy struct {
	loader  *rbac.Loader
	roles   RoleSource
	allowed map[string]struct{}
}

func NewPolicy(loader *rbac.Loader, roles RoleSource, allowedScopes []string) (*Policy, error) {
	if loader == nil || roles == nil || len(allowedScopes) == 0 {
		return nil, errors.New("invalid Threadify OAuth policy")
	}
	known := map[string]bool{}
	for _, role := range loader.GetRolesByLevel("app_level") {
		for _, permission := range role.Permissions {
			known[permission] = true
		}
	}
	allowed := make(map[string]struct{}, len(allowedScopes))
	for _, scope := range allowedScopes {
		if !known[scope] || scope == "" {
			return nil, fmt.Errorf("unknown Threadify OAuth scope %q", scope)
		}
		if _, duplicate := allowed[scope]; duplicate {
			return nil, fmt.Errorf("duplicate Threadify OAuth scope %q", scope)
		}
		allowed[scope] = struct{}{}
	}
	return &Policy{loader: loader, roles: roles, allowed: allowed}, nil
}

func (p *Policy) ValidateScope(scope string) error {
	if p == nil {
		return errors.New("OAuth policy unavailable")
	}
	if _, ok := p.allowed[scope]; !ok {
		return fmt.Errorf("Threadify OAuth scope %q is not delegable", scope)
	}
	return nil
}

func (p *Policy) CanGrant(ctx context.Context, userID, scope string) (bool, error) {
	if err := p.ValidateScope(scope); err != nil {
		return false, err
	}
	roles, err := p.roles(ctx, userID)
	if err != nil || len(roles) == 0 {
		return false, errors.New("active Threadify membership required")
	}
	permissions := p.loader.GetPermissionsForRoles(roles, "app_level")
	return p.loader.CheckPermission(permissions, scope), nil
}

// CanUse applies the stored grant ceiling and the user's current roles to a
// concrete permission, such as contract.read.<contract_id>.
func (p *Policy) CanUse(ctx context.Context, userID string, granted []string, required string) bool {
	if p == nil || required == "" {
		return false
	}
	for _, scope := range granted {
		if p.ValidateScope(scope) != nil || !p.loader.CheckPermission([]string{scope}, required) {
			continue
		}
		allowed, err := p.CanGrant(ctx, userID, scope)
		if err == nil && allowed {
			return true
		}
	}
	return false
}

func (p *Policy) Actor(userID, credentialHash string) oauthserver.Actor[string] {
	return oauthserver.Actor[string]{
		SubjectID: userID, CredentialID: credentialHash, IsBrowserSession: true,
		CanGrant: func(ctx context.Context, scope string) (bool, error) {
			return p.CanGrant(ctx, userID, scope)
		},
	}
}
