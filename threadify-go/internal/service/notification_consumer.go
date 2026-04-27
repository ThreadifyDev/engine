package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/threadify/engine/internal/types"
	"github.com/threadify/engine/internal/models"
	"go.uber.org/zap"
)

// NotificationHandler defines the interface for handling received notifications.
type NotificationHandler interface {
	HandleNotification(notification models.ValidationNotification) error
}

type NotificationConsumer struct {
	client        types.NATSClient
	scopeResolver *ScopeResolver
	handlers      map[string]NotificationHandler
	subscriptions map[string]context.CancelFunc
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *zap.Logger
}

// NewNotificationConsumer creates a new notification consumer.
func NewNotificationConsumer(
	client types.NATSClient,
	scopeResolver *ScopeResolver,
	logger *zap.Logger,
) *NotificationConsumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &NotificationConsumer{
		client:        client,
		scopeResolver: scopeResolver,
		handlers:      make(map[string]NotificationHandler),
		subscriptions: make(map[string]context.CancelFunc),
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
	}
}

// Subscribe registers a handler for notifications on a specific thread and user scope.
// Returns an error if a subscription for this thread+user already exists.
func (nc *NotificationConsumer) Subscribe(threadID, userID, scope string, handler NotificationHandler) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	subKey := subKey(threadID, userID)

	if _, exists := nc.subscriptions[subKey]; exists {
		return fmt.Errorf("already subscribed to thread %s for user %s", threadID, userID)
	}

	subject := fmt.Sprintf("thread.%s.%s", threadID, scope)
	subCtx, subCancel := context.WithCancel(nc.ctx)

	nc.handlers[subKey] = handler
	nc.subscriptions[subKey] = subCancel

	go nc.consumeNotifications(subCtx, subKey, subject, handler)

	nc.logger.Info("subscribed to notifications",
		zap.String("thread_id", threadID),
		zap.String("user_id", userID),
		zap.String("subject", subject),
	)
	return nil
}

// Unsubscribe cancels the subscription for a thread and user.
func (nc *NotificationConsumer) Unsubscribe(threadID, userID string) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	key := subKey(threadID, userID)
	cancel, exists := nc.subscriptions[key]
	if !exists {
		return fmt.Errorf("no subscription found for thread %s and user %s", threadID, userID)
	}

	cancel()
	delete(nc.subscriptions, key)
	delete(nc.handlers, key)

	nc.logger.Info("unsubscribed from notifications",
		zap.String("thread_id", threadID),
		zap.String("user_id", userID),
	)
	return nil
}

// consumeNotifications fetches and dispatches messages for a single subscription.
// Runs in its own goroutine; exits when ctx is cancelled.
func (nc *NotificationConsumer) consumeNotifications(ctx context.Context, key, subject string, handler NotificationHandler) {
	// NATS consumer names must not contain ':' — sanitise the subKey.
	consumerName := "consumer-" + strings.ReplaceAll(key, ":", "-")

	for {
		select {
		case <-ctx.Done():
			nc.logger.Debug("stopping consumption", zap.String("subject", subject))
			return
		default:
		}

		msg, err := nc.client.FetchMessage(subject, consumerName, 5*time.Second)
		if err != nil {
			// Timeout or transient error — back off and retry.
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		var notification models.ValidationNotification
		if err := json.Unmarshal(msg.Data, &notification); err != nil {
			nc.logger.Error("failed to unmarshal notification",
				zap.String("sub_key", key),
				zap.Error(err),
			)
			continue
		}

		if err := handler.HandleNotification(notification); err != nil {
			nc.logger.Error("handler failed",
				zap.String("sub_key", key),
				zap.Error(err),
			)
		} else {
			nc.logger.Debug("delivered notification",
				zap.String("notification_id", notification.NotificationID),
				zap.String("sub_key", key),
			)
		}
	}
}

// Stop cancels all subscriptions and shuts down the consumer.
func (nc *NotificationConsumer) Stop() {
	// Cancelling nc.ctx propagates to all subCtx children, stopping their goroutines.
	nc.cancel()

	nc.mu.Lock()
	defer nc.mu.Unlock()

	nc.subscriptions = make(map[string]context.CancelFunc)
	nc.handlers = make(map[string]NotificationHandler)

	nc.logger.Info("notification consumer stopped")
}

// GetActiveSubscriptions returns the number of active subscriptions.
func (nc *NotificationConsumer) GetActiveSubscriptions() int {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return len(nc.subscriptions)
}

// subKey returns a consistent map key for a thread+user pair.
func subKey(threadID, userID string) string {
	return threadID + ":" + userID
}
