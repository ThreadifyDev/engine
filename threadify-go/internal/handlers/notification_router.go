package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/models"
)

// PendingNotification tracks a notification waiting for client ACK
type PendingNotification struct {
	natsMsg        jetstream.Msg
	notificationID string
	targetClients  map[string]bool // Clients that should ACK
	ackedClients   map[string]bool // Clients that have ACKed
	sentAt         time.Time
	mu             sync.Mutex
}

// ClientSubscription represents a client's subscription to step notifications
type ClientSubscription struct {
	StepName     string   // The step name to subscribe to
	ContractName string   // Optional contract filter (empty = all contracts for this step)
	EventTypes   []string // Event types: ["violation", "completed", "failed"] (empty = all)
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	ID            string
	Conn          *websocket.Conn
	OwnerID       string
	ThreadIDs     map[string]bool
	Subscriptions map[string]*ClientSubscription  // stepName -> subscription
	pendingAcks   map[string]*PendingNotification // notificationID -> pending
	mu            sync.RWMutex
	sendMu        sync.Mutex // Separate mutex for WebSocket writes
}

// NotificationRouter manages NATS consumer and routes notifications to WebSocket clients
type NotificationRouter struct {
	js              jetstream.JetStream
	consumer        jetstream.Consumer
	podID           string
	clients         map[string]*WebSocketClient // clientID -> client
	threadToClients map[string][]string         // threadID -> clientIDs
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewNotificationRouter creates a new notification router with pooled NATS consumer
func NewNotificationRouter(nc *nats.Conn, podID string) (*NotificationRouter, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create or get the NOTIFICATIONS stream
	stream, err := js.Stream(ctx, "NOTIFICATIONS")
	if err != nil {
		// Stream doesn't exist, create it
		_, err = js.CreateStream(ctx, jetstream.StreamConfig{
			Name:        "NOTIFICATIONS",
			Subjects:    []string{"notifications.thread.>"},
			Retention:   jetstream.WorkQueuePolicy,
			MaxAge:      24 * time.Hour,
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Description: "Thread validation notifications",
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create NOTIFICATIONS stream: %w", err)
		}
		log.Println("✅ Created NOTIFICATIONS stream")
	} else {
		log.Printf("✅ Using existing NOTIFICATIONS stream: %s", stream.CachedInfo().Config.Name)
	}

	// Create durable consumer for this pod (survives pod restarts)
	consumerName := fmt.Sprintf("pod-%s", podID)
	consumer, err := js.CreateOrUpdateConsumer(ctx, "NOTIFICATIONS", jetstream.ConsumerConfig{
		Name:          consumerName,
		Durable:       consumerName,
		FilterSubject: "notifications.thread.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    3,
		DeliverPolicy: jetstream.DeliverNewPolicy,
		Description:   fmt.Sprintf("WebSocket notification consumer for pod %s", podID),
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create NATS consumer: %w", err)
	}

	log.Printf("✅ Created NATS consumer: %s", consumerName)

	router := &NotificationRouter{
		js:              js,
		consumer:        consumer,
		podID:           podID,
		clients:         make(map[string]*WebSocketClient),
		threadToClients: make(map[string][]string),
		ctx:             ctx,
		cancel:          cancel,
	}

	// Start consuming and routing
	router.wg.Add(1)
	go router.consumeAndRoute()

	// Start cleanup goroutine for stale pending ACKs
	router.wg.Add(1)
	go router.cleanupStalePendingAcks()

	return router, nil
}

// RegisterClient registers a WebSocket client with the router
func (r *NotificationRouter) RegisterClient(client *WebSocketClient) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize subscriptions map
	if client.Subscriptions == nil {
		client.Subscriptions = make(map[string]*ClientSubscription)
	}

	r.clients[client.ID] = client
	log.Printf("📱 Registered client: %s (owner: %s)", client.ID, client.OwnerID)
}

// UnregisterClient removes a WebSocket client and NACKs pending notifications
func (r *NotificationRouter) UnregisterClient(clientID string) {
	r.mu.Lock()
	client, exists := r.clients[clientID]
	if !exists {
		r.mu.Unlock()
		return
	}
	delete(r.clients, clientID)

	// Remove from thread mappings
	for threadID, clients := range r.threadToClients {
		for i, cid := range clients {
			if cid == clientID {
				r.threadToClients[threadID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}
	}
	r.mu.Unlock()

	// NACK all pending notifications for this client
	client.mu.Lock()
	for notifID, pending := range client.pendingAcks {
		pending.mu.Lock()
		// Only NACK if this was the only client
		if len(pending.targetClients) == 1 && pending.targetClients[clientID] {
			if err := pending.natsMsg.Nak(); err != nil {
				log.Printf("⚠️ Failed to NACK notification %s: %v", notifID, err)
			} else {
				log.Printf("🔄 Client %s disconnected, NACKed notification %s", clientID, notifID)
			}
		}
		pending.mu.Unlock()
	}
	client.pendingAcks = nil
	client.mu.Unlock()

	log.Printf("📱 Unregistered client: %s", clientID)
}

// SubscribeToThread subscribes a client to a thread's notifications
func (r *NotificationRouter) SubscribeToThread(clientID, threadID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.clients[clientID]
	if !exists {
		log.Printf("⚠️ Client %s not found for thread subscription", clientID)
		return
	}

	client.mu.Lock()
	client.ThreadIDs[threadID] = true
	client.mu.Unlock()

	// Add to thread mapping
	r.threadToClients[threadID] = append(r.threadToClients[threadID], clientID)
	log.Printf("📌 Client %s subscribed to thread %s", clientID, threadID)
}

// consumeAndRoute consumes NATS messages and routes to interested WebSocket clients
func (r *NotificationRouter) consumeAndRoute() {
	defer r.wg.Done()

	log.Printf("🚀 Starting notification router for pod %s", r.podID)

	for {
		select {
		case <-r.ctx.Done():
			log.Printf("🛑 Stopping notification router for pod %s", r.podID)
			return
		default:
			// Fetch batch of messages from NATS
			msgs, err := r.consumer.Fetch(100, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				if err != nats.ErrTimeout {
					log.Printf("⚠️ NATS fetch error: %v", err)
				}
				continue
			}

			r.processMessages(msgs)
		}
	}
}

// processMessages processes a batch of NATS messages
func (r *NotificationRouter) processMessages(msgs jetstream.MessageBatch) {
	for msg := range msgs.Messages() {
		var notification models.ValidationNotification
		if err := json.Unmarshal(msg.Data(), &notification); err != nil {
			log.Printf("⚠️ Failed to parse notification: %v", err)
			msg.Ack() // ACK malformed messages to avoid redelivery
			continue
		}

		r.routeNotification(&notification, msg)
	}
}

// routeNotification routes a notification to interested clients
func (r *NotificationRouter) routeNotification(notification *models.ValidationNotification, msg jetstream.Msg) {
	r.mu.RLock()
	clientIDs := r.threadToClients[notification.ThreadID]
	r.mu.RUnlock()

	if len(clientIDs) == 0 {
		// No clients on this pod care about this thread
		log.Printf("📭 No clients for thread %s, ACKing immediately", notification.ThreadID)
		msg.Ack()
		return
	}

	// Parse recipients list from notification payload
	var notifWithRecipients struct {
		models.ValidationNotification
		Recipients []string `json:"recipients"`
	}
	if err := json.Unmarshal(msg.Data(), &notifWithRecipients); err != nil {
		log.Printf("⚠️ Failed to parse notification recipients: %v", err)
		msg.Ack() // ACK to avoid redelivery
		return
	}

	// Send to all interested clients on this pod
	delivered := false
	for _, clientID := range clientIDs {
		r.mu.RLock()
		client, exists := r.clients[clientID]
		r.mu.RUnlock()

		if !exists {
			continue
		}

		// Verify client's ownerID is in recipients list (permission check)
		if !containsString(notifWithRecipients.Recipients, client.OwnerID) {
			log.Printf("🔒 Client %s (owner: %s) not in recipients list, skipping notification %s",
				clientID, client.OwnerID, notification.NotificationID)
			continue
		}

		// Check if client is subscribed to this notification (contract filtering)
		if !r.shouldSendToClient(client, notification) {
			log.Printf("🔕 Client %s not subscribed to step=%s contract=%s, skipping notification %s",
				clientID, notification.StepName, notification.ContractName, notification.NotificationID)
			continue
		}

		if err := client.sendNotification(notification); err != nil {
			log.Printf("⚠️ Failed to send notification to client %s: %v", clientID, err)
			continue
		}

		// Track pending ACK
		client.storePendingAck(notification.NotificationID, msg)
		delivered = true
		log.Printf("📤 Sent notification %s to client %s (step=%s, contract=%s)",
			notification.NotificationID, clientID, notification.StepName, notification.ContractName)
	}

	if !delivered {
		// Failed to deliver to any client
		log.Printf("⚠️ Failed to deliver notification %s, NACKing", notification.NotificationID)
		msg.Nak()
	}
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// shouldSendToClient checks if notification should be sent to this client based on subscriptions
func (r *NotificationRouter) shouldSendToClient(client *WebSocketClient, notification *models.ValidationNotification) bool {
	client.mu.RLock()
	defer client.mu.RUnlock()

	// CRITICAL: No subscriptions = receive NOTHING
	if client.Subscriptions == nil || len(client.Subscriptions) == 0 {
		return false
	}

	// Check if client is subscribed to this step
	sub, exists := client.Subscriptions[notification.StepName]
	if !exists {
		return false
	}

	// Check contract filter
	if sub.ContractName != "" && sub.ContractName != notification.ContractName {
		return false
	}

	// Check event type filter
	if len(sub.EventTypes) > 0 {
		var eventType string
		if notification.Status == "violated" {
			eventType = "violation"
		} else if notification.StepStatus == "success" && notification.Status == "passed" {
			eventType = "completed"
		} else if notification.StepStatus == "failed" || notification.StepStatus == "error" {
			eventType = "failed"
		}

		matched := false
		for _, et := range sub.EventTypes {
			if et == eventType {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// sendNotification sends a notification to the WebSocket client
func (c *WebSocketClient) sendNotification(notification *models.ValidationNotification) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	message := map[string]interface{}{
		"action":       "notification",
		"notification": notification,
	}

	return c.Conn.WriteJSON(message)
}

// storePendingAck stores a pending ACK for a notification
func (c *WebSocketClient) storePendingAck(notifID string, msg jetstream.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pendingAcks == nil {
		c.pendingAcks = make(map[string]*PendingNotification)
	}

	// Check if we already have this pending notification
	if pending, exists := c.pendingAcks[notifID]; exists {
		pending.mu.Lock()
		pending.targetClients[c.ID] = true
		pending.mu.Unlock()
		return
	}

	// Create new pending notification
	pending := &PendingNotification{
		natsMsg:        msg,
		notificationID: notifID,
		targetClients:  map[string]bool{c.ID: true},
		ackedClients:   make(map[string]bool),
		sentAt:         time.Now(),
	}

	c.pendingAcks[notifID] = pending
}

// HandleClientAck handles an ACK from a WebSocket client
func (c *WebSocketClient) HandleClientAck(notifID string) error {
	c.mu.Lock()
	pending, exists := c.pendingAcks[notifID]
	c.mu.Unlock()

	if !exists {
		// Notification not found - might be duplicate ACK (idempotent)
		log.Printf("⚠️ Notification %s not found for client %s (duplicate ACK?)", notifID, c.ID)
		return nil
	}

	pending.mu.Lock()
	pending.ackedClients[c.ID] = true

	// ACK NATS when ANY client ACKs (at-least-once delivery)
	shouldAck := len(pending.ackedClients) == 1
	pending.mu.Unlock()

	if shouldAck {
		if err := pending.natsMsg.Ack(); err != nil {
			log.Printf("⚠️ NATS ACK failed for notification %s: %v", notifID, err)
			return err
		}
		log.Printf("✅ NATS message ACKed for notification %s", notifID)

		// Remove from pending (all clients have been notified)
		c.mu.Lock()
		delete(c.pendingAcks, notifID)
		c.mu.Unlock()
	}

	return nil
}

// cleanupStalePendingAcks periodically cleans up stale pending ACKs
func (r *NotificationRouter) cleanupStalePendingAcks() {
	defer r.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.cleanupStale()
		}
	}
}

// cleanupStale removes stale pending ACKs (older than 60 seconds)
func (r *NotificationRouter) cleanupStale() {
	r.mu.RLock()
	clients := make([]*WebSocketClient, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.mu.RUnlock()

	now := time.Now()
	for _, client := range clients {
		client.mu.Lock()
		for notifID, pending := range client.pendingAcks {
			pending.mu.Lock()
			if now.Sub(pending.sentAt) > 60*time.Second {
				// Stale - NACK for redelivery
				if err := pending.natsMsg.Nak(); err != nil {
					log.Printf("⚠️ Failed to NACK stale notification %s: %v", notifID, err)
				} else {
					log.Printf("🔄 Stale notification %s NACKed for redelivery", notifID)
				}
				delete(client.pendingAcks, notifID)
			}
			pending.mu.Unlock()
		}
		client.mu.Unlock()
	}
}

// Stop gracefully stops the notification router
func (r *NotificationRouter) Stop() {
	log.Printf("🛑 Stopping notification router for pod %s", r.podID)
	r.cancel()
	r.wg.Wait()
	log.Printf("✅ Notification router stopped for pod %s", r.podID)
}
