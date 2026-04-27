package service

import (
	"context"
	"fmt"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/types"
	"go.uber.org/zap"
)

// ScopeResolver handles notification scope resolution for users in threads.
type ScopeResolver struct {
	config            *config.Config
	contractGraphRepo types.ContractGraphRepository
	threadRepo        types.ThreadRepository
	logger            *zap.Logger
}

// NewScopeResolver creates a new scope resolver.
func NewScopeResolver(
	cfg *config.Config,
	contractGraphRepo types.ContractGraphRepository,
	threadRepo types.ThreadRepository,
	logger *zap.Logger,
) *ScopeResolver {
	r := &ScopeResolver{
		config:            cfg,
		contractGraphRepo: contractGraphRepo,
		threadRepo:        threadRepo,
		logger:            logger,
	}

	// Log a warning on misconfiguration but don't fail — defaults will be used.
	if err := r.ValidateConfig(); err != nil {
		r.logger.Warn("configuration validation failed", zap.Error(err))
	}

	return r
}

// ValidateConfig validates the notification system configuration.
func (r *ScopeResolver) ValidateConfig() error {
	if r.config.NotificationSystem.DefaultScope == "" {
		return ErrConfigMissingDefaultScope
	}

	if _, exists := r.config.NotificationSystem.Scopes[r.config.NotificationSystem.DefaultScope]; !exists {
		return fmt.Errorf("default_scope %q does not exist in scopes configuration", r.config.NotificationSystem.DefaultScope)
	}

	for scopeName, scopeConfig := range r.config.NotificationSystem.Scopes {
		if len(scopeConfig.Permissions) == 0 {
			return fmt.Errorf("scope %q has no permissions defined", scopeName)
		}
	}

	return nil
}

// ResolveScope determines the notification scope for a user in a thread.
//
// Resolution order:
//  1. Creator  → always "owner"
//  2. Explicit scope provided (from invitation)
//  3. Contract role_defaults
//  4. Contract default_scope
//  5. System default_scope
func (r *ScopeResolver) ResolveScope(
	ctx context.Context,
	threadID, userID, role string,
	isCreator bool,
	explicitScope *string,
) (string, error) {
	scopeFields := r.scopeLogFields(userID, threadID, role)

	// 1. Creator is always owner.
	if isCreator {
		r.logger.Debug("resolved scope (creator)", append(scopeFields, zap.String("result", "owner"))...)
		return "owner", nil
	}

	// 2. Explicit scope provided (from invitation).
	if explicitScope != nil {
		if *explicitScope == "" {
			r.logger.Debug("resolved scope (explicit none)", scopeFields...)
			return "", nil
		}
		if !r.isValidScope(*explicitScope) {
			r.logger.Error("invalid explicit scope", append(scopeFields, zap.String("scope", *explicitScope))...)
			return "", fmt.Errorf("invalid scope: %q", *explicitScope)
		}
		r.logger.Debug("resolved scope (explicit)", append(scopeFields, zap.String("result", *explicitScope))...)
		return *explicitScope, nil
	}

	systemDefault := r.config.NotificationSystem.DefaultScope

	// 3-4. Fetch thread and contract for role_defaults / contract default_scope.
	thread, err := r.threadRepo.Get(ctx, threadID)
	if err != nil {
		r.logger.Debug("resolved scope (system default, thread not found)", append(scopeFields, zap.String("result", systemDefault))...)
		return systemDefault, nil
	}

	if thread.ContractName == "" || thread.ContractVersion == nil {
		r.logger.Debug("resolved scope (system default, no contract)", append(scopeFields, zap.String("result", systemDefault))...)
		return systemDefault, nil
	}

	contract, err := r.contractGraphRepo.Get(ctx, thread.ContractName, *thread.ContractVersion, thread.CompanyID)
	if err != nil {
		r.logger.Debug("resolved scope (system default, contract not found)", append(scopeFields, zap.String("result", systemDefault))...)
		return systemDefault, nil
	}

	nc := contract.NotificationConfig

	// 3. Contract role_defaults.
	if nc != nil && nc.RoleDefaults != nil {
		if roleDefault, exists := nc.RoleDefaults[role]; exists && r.isValidScope(roleDefault) {
			r.logger.Debug("resolved scope (contract role_default)", append(scopeFields, zap.String("result", roleDefault))...)
			return roleDefault, nil
		}
	}

	// 4. Contract default_scope (checked independently of RoleDefaults).
	if nc != nil && nc.DefaultScope != "" && r.isValidScope(nc.DefaultScope) {
		r.logger.Debug("resolved scope (contract default)", append(scopeFields, zap.String("result", nc.DefaultScope))...)
		return nc.DefaultScope, nil
	}

	// 5. System default.
	r.logger.Debug("resolved scope (system default)", append(scopeFields, zap.String("result", systemDefault))...)
	return systemDefault, nil
}

// isValidScope reports whether scope exists in the system configuration.
// An empty scope is valid and means "no notification access".
func (r *ScopeResolver) isValidScope(scope string) bool {
	if scope == "" {
		return true
	}
	_, exists := r.config.NotificationSystem.Scopes[scope]
	return exists
}

// scopeLogFields returns the common zap fields used in ResolveScope log lines.
func (r *ScopeResolver) scopeLogFields(userID, threadID, role string) []zap.Field {
	return []zap.Field{
		zap.String("user_id", userID),
		zap.String("thread_id", threadID),
		zap.String("role", role),
	}
}
