package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// ScopeResolver defines the interface for scope resolution
type ScopeResolver interface {
	GetScopePermissions(scope string) []string
	ResolveScope(ctx context.Context, threadID, userID, role string, isCreator bool, explicitScope *string) (string, error)
}

// Publisher handles publishing notifications to NATS
type Publisher struct {
	client        *Client
	scopeResolver ScopeResolver
	accessRepo    interfaces.AccessRepository
}

// NewPublisher creates a new notification publisher
func NewPublisher(
	client *Client,
	scopeResolver ScopeResolver,
	accessRepo interfaces.AccessRepository,
) *Publisher {
	return &Publisher{
		client:        client,
		scopeResolver: scopeResolver,
		accessRepo:    accessRepo,
	}
}

// PublishNotification publishes a notification to appropriate scope subjects
func (p *Publisher) PublishNotification(ctx context.Context, notification models.ValidationNotification) error {
	// Get all users in thread
	allAccess, err := p.accessRepo.GetAllAccess(ctx, notification.ThreadID)
	if err != nil {
		return fmt.Errorf("failed to get thread access: %w", err)
	}

	// Group users by scope
	scopeUsers := make(map[string][]string)
	for userID, access := range allAccess {
		// Check if user has "read" permission (Layer 1: Permission Gatekeeper)
		if !contains(access.Permissions, "read") {
			fmt.Printf("[NATS-PUBLISHER] User %s does not have 'read' permission, skipping notification\n", userID)
			continue
		}

		// Get user's scope (from access.Scope or resolve it)
		scope := p.getUserScope(ctx, notification.ThreadID, userID, access)

		// Check if user should receive this notification (Layer 2: Content Filtering)
		if p.shouldReceiveNotification(notification, scope) {
			scopeUsers[scope] = append(scopeUsers[scope], userID)
		}
	}

	// Publish to each scope subject
	for scope, users := range scopeUsers {
		if err := p.publishToScope(notification, scope, users); err != nil {
			return fmt.Errorf("failed to publish to scope %s: %w", scope, err)
		}
	}

	return nil
}

// getUserScope gets user's scope from access or resolves it
func (p *Publisher) getUserScope(ctx context.Context, threadID, userID string, access *interfaces.UserAccess) string {
	// Get the first role (primary role)
	var role string
	if len(access.Roles) > 0 {
		role = access.Roles[0]
	}

	// Check if user is creator (granted_by is "self" for creator)
	isCreator := access.GrantedBy == "self" || access.GrantedBy == ""

	// Resolve scope using the scope resolver
	scope, err := p.scopeResolver.ResolveScope(ctx, threadID, userID, role, isCreator, nil)
	if err != nil {
		// Fallback to participant if resolution fails
		fmt.Printf("[NATS-PUBLISHER] Failed to resolve scope for user %s in thread %s: %v\n", userID, threadID, err)
		return "participant"
	}
	return scope
}

// shouldReceiveNotification checks if user with given scope should receive notification
func (p *Publisher) shouldReceiveNotification(notification models.ValidationNotification, scope string) bool {
	permissions := p.scopeResolver.GetScopePermissions(scope)

	// Completion notifications (status = "passed")
	if notification.Status == "passed" {
		return contains(permissions, "completions") || contains(permissions, "all_completions")
	}

	// Failed/Error notifications (status = "failed" or "error")
	if notification.Status == "failed" || notification.Status == "error" {
		return contains(permissions, "failures") || contains(permissions, "all_failures") || contains(permissions, "all_violations")
	}

	// Critical violations
	if notification.Severity == string(models.SeverityCritical) {
		return contains(permissions, "critical_violations") || contains(permissions, "all_violations")
	}

	// All violations (for owner scope)
	return contains(permissions, "all_violations")
}

// publishToScope publishes notification to a specific scope subject
func (p *Publisher) publishToScope(notification models.ValidationNotification, scope string, recipients []string) error {
	// Create subject: thread.{threadId}.{scope}
	subject := fmt.Sprintf("thread.%s.%s", notification.ThreadID, scope)

	// Add recipients to notification payload
	notificationWithRecipients := struct {
		models.ValidationNotification
		Recipients []string `json:"recipients"`
	}{
		ValidationNotification: notification,
		Recipients:             recipients,
	}

	// Marshal to JSON
	data, err := json.Marshal(notificationWithRecipients)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Publish to NATS (old subject for backward compatibility)
	_, err = p.client.JetStream().Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}

	fmt.Printf("[NATS-PUBLISH] Published to %s for %d recipients\n", subject, len(recipients))

	// Also publish to WebSocket notification stream (new subject pattern)
	wsSubject := fmt.Sprintf("notifications.thread.%s", notification.ThreadID)
	_, err = p.client.JetStream().Publish(wsSubject, data)
	if err != nil {
		fmt.Printf("[NATS-WARN] Failed to publish to WebSocket stream %s: %v\n", wsSubject, err)
		// Don't fail the whole operation if WebSocket publish fails
	} else {
		fmt.Printf("[NATS-PUBLISH] Published to WebSocket stream %s\n", wsSubject)
	}

	return nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
