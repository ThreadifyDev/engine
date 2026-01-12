package service

import (
	"context"
	"fmt"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
)

// ScopeResolver handles notification scope resolution for users in threads
type ScopeResolver struct {
	config            *config.Config
	contractGraphRepo interfaces.ContractGraphRepository
	threadRepo        interfaces.ThreadRepository
}

// NewScopeResolver creates a new scope resolver
func NewScopeResolver(
	cfg *config.Config,
	contractGraphRepo interfaces.ContractGraphRepository,
	threadRepo interfaces.ThreadRepository,
) *ScopeResolver {
	resolver := &ScopeResolver{
		config:            cfg,
		contractGraphRepo: contractGraphRepo,
		threadRepo:        threadRepo,
	}

	// Validate configuration on initialization
	if err := resolver.ValidateConfig(); err != nil {
		// Log warning but don't fail - use defaults
		fmt.Printf("[SCOPE-RESOLVER-WARN] Configuration validation failed: %v\n", err)
	}

	return resolver
}

// ValidateConfig validates the notification system configuration
func (r *ScopeResolver) ValidateConfig() error {
	if r.config.NotificationSystem.DefaultScope == "" {
		return fmt.Errorf("notification_system.default_scope is not set")
	}

	// Validate default scope exists in scopes map
	if _, exists := r.config.NotificationSystem.Scopes[r.config.NotificationSystem.DefaultScope]; !exists {
		return fmt.Errorf("default_scope '%s' does not exist in scopes configuration", r.config.NotificationSystem.DefaultScope)
	}

	// Validate all scopes have permissions
	for scopeName, scopeConfig := range r.config.NotificationSystem.Scopes {
		if len(scopeConfig.Permissions) == 0 {
			return fmt.Errorf("scope '%s' has no permissions defined", scopeName)
		}
	}

	return nil
}

// ResolveScope determines the notification scope for a user in a thread
// Resolution order:
// 1. Creator is always "owner"
// 2. Explicit scope provided (from invitation)
// 3. Contract role_defaults
// 4. Contract default_scope
// 5. System default_scope
func (r *ScopeResolver) ResolveScope(
	ctx context.Context,
	threadID string,
	userID string,
	role string,
	isCreator bool,
	explicitScope *string,
) (string, error) {
	// 1. Creator is always owner
	if isCreator {
		fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → owner (creator)\n", userID, threadID, role)
		return "owner", nil
	}

	// 2. Explicit scope provided (from invitation)
	if explicitScope != nil {
		if *explicitScope == "" {
			// Explicitly no scope - no notification access
			fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → none (explicit)\n", userID, threadID, role)
			return "", nil
		}

		// Validate scope exists in system config
		if r.isValidScope(*explicitScope) {
			fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (explicit)\n", userID, threadID, role, *explicitScope)
			return *explicitScope, nil
		}

		fmt.Printf("[SCOPE-RESOLVE-ERROR] User=%s, Thread=%s, Role=%s → invalid scope: %s\n", userID, threadID, role, *explicitScope)
		return "", fmt.Errorf("invalid scope: %s", *explicitScope)
	}

	// 3-4. Get contract for role_defaults and default_scope
	thread, err := r.threadRepo.Get(ctx, threadID)
	if err != nil {
		// If can't get thread, use system default
		fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (system default, thread not found)\n",
			userID, threadID, role, r.config.NotificationSystem.DefaultScope)
		return r.config.NotificationSystem.DefaultScope, nil
	}

	// Handle nil contract name or version (contract name is used for graph lookups)
	if thread.ContractName == "" || thread.ContractVersion == nil {
		fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (system default, no contract)\n",
			userID, threadID, role, r.config.NotificationSystem.DefaultScope)
		return r.config.NotificationSystem.DefaultScope, nil
	}

	// Use contract name (not UUID) for graph repository lookups
	contract, err := r.contractGraphRepo.Get(ctx, thread.ContractName, *thread.ContractVersion, thread.CompanyID)
	if err != nil {
		// If can't get contract, use system default
		fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (system default, contract not found)\n",
			userID, threadID, role, r.config.NotificationSystem.DefaultScope)
		return r.config.NotificationSystem.DefaultScope, nil
	}

	// 3. Check contract role_defaults
	if contract.NotificationConfig != nil && contract.NotificationConfig.RoleDefaults != nil {
		if roleDefault, exists := contract.NotificationConfig.RoleDefaults[role]; exists {
			if r.isValidScope(roleDefault) {
				fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (contract role_default)\n",
					userID, threadID, role, roleDefault)
				return roleDefault, nil
			}
		}

		// 4. Check contract default_scope
		if contract.NotificationConfig.DefaultScope != "" {
			if r.isValidScope(contract.NotificationConfig.DefaultScope) {
				fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (contract default)\n",
					userID, threadID, role, contract.NotificationConfig.DefaultScope)
				return contract.NotificationConfig.DefaultScope, nil
			}
		}
	}

	// 5. Use system default
	fmt.Printf("[SCOPE-RESOLVE] User=%s, Thread=%s, Role=%s → %s (system default)\n",
		userID, threadID, role, r.config.NotificationSystem.DefaultScope)
	return r.config.NotificationSystem.DefaultScope, nil
}

// isValidScope checks if a scope exists in system configuration
func (r *ScopeResolver) isValidScope(scope string) bool {
	if scope == "" {
		return true // Empty scope is valid (means no notification access)
	}

	_, exists := r.config.NotificationSystem.Scopes[scope]
	return exists
}

// GetScopePermissions returns the permissions for a given scope
func (r *ScopeResolver) GetScopePermissions(scope string) []string {
	if scope == "" {
		return []string{} // No permissions
	}

	scopeConfig, exists := r.config.NotificationSystem.Scopes[scope]
	if !exists {
		return []string{}
	}

	return scopeConfig.Permissions
}

// ShouldReceiveNotification determines if a user with given scope should receive a notification
func (r *ScopeResolver) ShouldReceiveNotification(
	scope string,
	notificationType string, // "violation" or "completion"
	stepOwner string, // Role that owns the step
	userRole string, // User's role
	severity string, // "critical", "warning", "minor", "info"
) bool {
	if scope == "" {
		return false // No scope = no notifications
	}

	permissions := r.GetScopePermissions(scope)

	switch scope {
	case "owner":
		// Owner sees everything
		return true

	case "participant":
		// Participant sees:
		// - All completions
		// - Critical violations anywhere
		// - Own step violations

		if notificationType == "completion" {
			return contains(permissions, "completions")
		}

		if notificationType == "violation" {
			// Critical violations anywhere
			if severity == "critical" && contains(permissions, "critical_violations") {
				return true
			}

			// Own step violations
			if stepOwner == userRole && contains(permissions, "own_step_violations") {
				return true
			}
		}

		return false

	case "observer":
		// Observer only sees completions
		if notificationType == "completion" {
			return contains(permissions, "completions")
		}
		return false

	default:
		return false
	}
}
