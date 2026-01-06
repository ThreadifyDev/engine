package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
)

// NotificationHandler defines the interface for handling received notifications
type NotificationHandler interface {
	HandleNotification(notification models.ValidationNotification) error
}

// NotificationConsumer consumes notifications from NATS and delivers them to handlers
type NotificationConsumer struct {
	client        *natsrepo.Client
	scopeResolver *ScopeResolver
	handlers      map[string]NotificationHandler // threadID -> handler
	subscriptions map[string]context.CancelFunc  // threadID -> cancel function
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewNotificationConsumer creates a new notification consumer
func NewNotificationConsumer(
	client *natsrepo.Client,
	scopeResolver *ScopeResolver,
) *NotificationConsumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &NotificationConsumer{
		client:        client,
		scopeResolver: scopeResolver,
		handlers:      make(map[string]NotificationHandler),
		subscriptions: make(map[string]context.CancelFunc),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Subscribe subscribes a handler to notifications for a specific thread and user scope
func (nc *NotificationConsumer) Subscribe(threadID, userID, scope string, handler NotificationHandler) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	// Create unique key for this subscription
	subKey := fmt.Sprintf("%s:%s", threadID, userID)

	// Check if already subscribed
	if _, exists := nc.subscriptions[subKey]; exists {
		return fmt.Errorf("already subscribed to thread %s for user %s", threadID, userID)
	}

	// Store handler
	nc.handlers[subKey] = handler

	// Create subject pattern for this thread and scope
	subject := fmt.Sprintf("thread.%s.%s", threadID, scope)

	// Create context for this subscription
	subCtx, subCancel := context.WithCancel(nc.ctx)
	nc.subscriptions[subKey] = subCancel

	// Start consuming in background
	go nc.consumeNotifications(subCtx, subKey, subject, handler)

	fmt.Printf("[CONSUMER] Subscribed to %s for user %s (scope: %s)\n", subject, userID, scope)
	return nil
}

// Unsubscribe removes a subscription for a thread and user
func (nc *NotificationConsumer) Unsubscribe(threadID, userID string) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	subKey := fmt.Sprintf("%s:%s", threadID, userID)

	// Cancel the subscription context
	if cancel, exists := nc.subscriptions[subKey]; exists {
		cancel()
		delete(nc.subscriptions, subKey)
		delete(nc.handlers, subKey)
		fmt.Printf("[CONSUMER] Unsubscribed from thread %s for user %s\n", threadID, userID)
		return nil
	}

	return fmt.Errorf("no subscription found for thread %s and user %s", threadID, userID)
}

// consumeNotifications consumes notifications from NATS for a specific subject
func (nc *NotificationConsumer) consumeNotifications(ctx context.Context, subKey, subject string, handler NotificationHandler) {
	// Subscribe to NATS subject
	msgChan := make(chan []byte, 100)

	// Start NATS subscription
	go func() {
		defer close(msgChan)

		// Use NATS JetStream pull consumer
		consumerName := fmt.Sprintf("consumer-%s", subKey)

		for {
			select {
			case <-ctx.Done():
				fmt.Printf("[CONSUMER] Stopping consumption for %s\n", subject)
				return
			default:
				// Fetch messages from NATS
				// This is a simplified version - in production, use proper JetStream consumer
				msg, err := nc.client.FetchMessage(subject, consumerName, 5*time.Second)
				if err != nil {
					// Timeout or error - continue
					time.Sleep(100 * time.Millisecond)
					continue
				}

				select {
				case msgChan <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Process messages
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}

			// Deserialize notification
			var notification models.ValidationNotification
			if err := json.Unmarshal(msg, &notification); err != nil {
				fmt.Printf("[CONSUMER-ERROR] Failed to unmarshal notification: %v\n", err)
				continue
			}

			// Deliver to handler
			if err := handler.HandleNotification(notification); err != nil {
				fmt.Printf("[CONSUMER-ERROR] Handler failed for %s: %v\n", subKey, err)
			} else {
				fmt.Printf("[CONSUMER] Delivered notification %s to %s\n", notification.NotificationID, subKey)
			}
		}
	}
}

// Stop stops the consumer and all subscriptions
func (nc *NotificationConsumer) Stop() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	// Cancel all subscriptions
	for subKey, cancel := range nc.subscriptions {
		cancel()
		fmt.Printf("[CONSUMER] Stopped subscription: %s\n", subKey)
	}

	// Clear maps
	nc.subscriptions = make(map[string]context.CancelFunc)
	nc.handlers = make(map[string]NotificationHandler)

	// Cancel main context
	nc.cancel()

	fmt.Println("[CONSUMER] Notification consumer stopped")
}

// GetActiveSubscriptions returns the number of active subscriptions
func (nc *NotificationConsumer) GetActiveSubscriptions() int {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return len(nc.subscriptions)
}
