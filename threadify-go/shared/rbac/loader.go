package rbac

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Permission represents a single permission from JSON
type Permission struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsWildcard  bool   `json:"is_wildcard"`
}

// Role represents a role with its permissions from JSON
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// PermissionsConfig holds all permissions by scope level
type PermissionsConfig struct {
	AppLevel     []Permission `json:"app_level"`
	RuntimeLevel []Permission `json:"runtime_level"`
}

// RolesConfig holds all roles by scope level
type RolesConfig struct {
	AppLevel     map[string]Role `json:"app_level"`
	APILevel     map[string]Role `json:"api_level"`
	RuntimeLevel map[string]Role `json:"runtime_level"`
}

// Loader loads and caches permissions and roles from JSON files
type Loader struct {
	permissions PermissionsConfig
	roles       RolesConfig
	mu          sync.RWMutex
}

// NewLoader creates a new RBAC loader reading from disk
func NewLoader(permissionsPath, rolesPath string) (*Loader, error) {
	loader := &Loader{}

	// Load permissions
	pData, err := os.ReadFile(permissionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read permissions: %w", err)
	}
	if err := loader.loadPermissions(pData); err != nil {
		return nil, fmt.Errorf("failed to parse permissions: %w", err)
	}

	// Load roles
	rData, err := os.ReadFile(rolesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read roles: %w", err)
	}
	if err := loader.loadRoles(rData); err != nil {
		return nil, fmt.Errorf("failed to parse roles: %w", err)
	}

	return loader, nil
}

// NewLoaderFromFS creates a new RBAC loader reading from an embedded filesystem
func NewLoaderFromFS(fs interface {
	ReadFile(name string) ([]byte, error)
}, permissionsPath, rolesPath string) (*Loader, error) {
	loader := &Loader{}

	// Load permissions
	pData, err := fs.ReadFile(permissionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read permissions from FS: %w", err)
	}
	if err := loader.loadPermissions(pData); err != nil {
		return nil, fmt.Errorf("failed to parse permissions: %w", err)
	}

	// Load roles
	rData, err := fs.ReadFile(rolesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read roles from FS: %w", err)
	}
	if err := loader.loadRoles(rData); err != nil {
		return nil, fmt.Errorf("failed to parse roles: %w", err)
	}

	return loader, nil
}

// loadPermissions unmarshals permissions from JSON data
func (l *Loader) loadPermissions(data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return json.Unmarshal(data, &l.permissions)
}

// loadRoles unmarshals roles from JSON data
func (l *Loader) loadRoles(data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return json.Unmarshal(data, &l.roles)
}

// GetPermissionsForRoles returns all permissions for given role names and scope level
func (l *Loader) GetPermissionsForRoles(roleNames []string, scopeLevel string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var allPermissions []string
	permissionSet := make(map[string]bool) // Deduplicate permissions

	var roles map[string]Role
	switch scopeLevel {
	case "app_level":
		roles = l.roles.AppLevel
	case "api_level":
		roles = l.roles.APILevel
	default:
		roles = l.roles.RuntimeLevel
	}

	for _, roleName := range roleNames {
		if role, exists := roles[roleName]; exists {
			for _, perm := range role.Permissions {
				if !permissionSet[perm] {
					permissionSet[perm] = true
					allPermissions = append(allPermissions, perm)
				}
			}
		}
	}

	return allPermissions
}

// CheckPermission checks if a required permission matches any of the user's permissions
func (l *Loader) CheckPermission(userPermissions []string, required string) bool {
	for _, perm := range userPermissions {
		// Exact match
		if perm == required {
			return true
		}

		// Wildcard match (e.g., "contract.read.*" matches "contract.read.order_contract")
		if strings.HasSuffix(perm, ".*") {
			prefix := strings.TrimSuffix(perm, ".*")
			if strings.HasPrefix(required, prefix+".") || required == prefix {
				return true
			}
		}
	}
	return false
}

// GetAllAppLevelRoles returns all app-level role names
func (l *Loader) GetAllAppLevelRoles() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	roles := make([]string, 0, len(l.roles.AppLevel))
	for roleName := range l.roles.AppLevel {
		roles = append(roles, roleName)
	}
	return roles
}

// GetAllRuntimeLevelRoles returns all runtime-level role names
func (l *Loader) GetAllRuntimeLevelRoles() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	roles := make([]string, 0, len(l.roles.RuntimeLevel))
	for roleName := range l.roles.RuntimeLevel {
		roles = append(roles, roleName)
	}
	return roles
}

// GetAllRoles returns all roles with their details
func (l *Loader) GetAllRoles() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return map[string]interface{}{
		"app_level":     l.roles.AppLevel,
		"api_level":     l.roles.APILevel,
		"runtime_level": l.roles.RuntimeLevel,
	}
}

// GetRolesByLevel returns roles for a specific level
func (l *Loader) GetRolesByLevel(level string) map[string]Role {
	l.mu.RLock()
	defer l.mu.RUnlock()

	switch level {
	case "app_level":
		return l.roles.AppLevel
	case "api_level":
		return l.roles.APILevel
	case "runtime_level":
		return l.roles.RuntimeLevel
	default:
		return nil
	}
}

// Reload reloads permissions and roles from disk
func (l *Loader) Reload(permissionsPath, rolesPath string) error {
	pData, err := os.ReadFile(permissionsPath)
	if err != nil {
		return err
	}
	if err := l.loadPermissions(pData); err != nil {
		return err
	}

	rData, err := os.ReadFile(rolesPath)
	if err != nil {
		return err
	}
	return l.loadRoles(rData)
}
