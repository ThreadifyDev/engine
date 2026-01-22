package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/models"
)

// ClientSubscription represents a client's subscription to step notifications
type ClientSubscription struct {
	StepName     string   // The step name to subscribe to
	ContractName string   // Optional contract filter (empty = all contracts for this step)
	EventTypes   []string // Event types: ["violation", "completed", "failed"] (empty = all)
}

// Session represents a client session
type Session struct {
	ID            string
	OwnerID       string
	MaxInFlight   int
	Conn          *websocket.Conn
	Subscriptions map[string]*ClientSubscription // stepName -> subscription
	mu            sync.RWMutex
	sendMu        *sync.Mutex // Pointer to shared connection-level mutex for WebSocket writes
}

// WebSocketClient is an alias for Session for backward compatibility
type WebSocketClient = Session

// NotificationRouter manages owner-based NATS consumers and routes notifications
type NotificationRouter struct {
	nc                 *nats.Conn // NATS connection for manual ACK
	js                 jetstream.JetStream
	sessions           map[string]*Session            // sessionID -> session
	consumers          map[string]jetstream.Consumer  // ownerID -> consumer (CHANGED: owner-based)
	sessionsByOwner    map[string][]string            // ownerID -> []sessionID
	subscriptionIndex  map[string]map[string][]string // ownerID -> "step@contract" -> []sessionID
	ownerContexts      map[string]context.Context     // ownerID -> context
	ownerCancels       map[string]context.CancelFunc  // ownerID -> cancel
	ownerConsumerCount map[string]int                 // ownerID -> count (rate limiting)
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	natsConfig         *config.NATSConfig // NATS configuration
}

// NewNotificationRouter creates a new session-based notification router
func NewNotificationRouter(nc *nats.Conn, natsConfig *config.NATSConfig) (*NotificationRouter, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Verify NOTIFICATIONS stream exists (created by NATS client initialization)
	_, err = js.Stream(ctx, "NOTIFICATIONS")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("NOTIFICATIONS stream not found: %w", err)
	}

	// NotificationRouter initialized

	return &NotificationRouter{
		nc:                 nc,
		js:                 js,
		sessions:           make(map[string]*Session),
		consumers:          make(map[string]jetstream.Consumer),
		sessionsByOwner:    make(map[string][]string),
		subscriptionIndex:  make(map[string]map[string][]string),
		ownerContexts:      make(map[string]context.Context),
		ownerCancels:       make(map[string]context.CancelFunc),
		ownerConsumerCount: make(map[string]int),
		ctx:                ctx,
		cancel:             cancel,
		natsConfig:         natsConfig,
	}, nil
}

// HandleConnect creates or reuses owner-based NATS consumer and starts router goroutine
func (r *NotificationRouter) HandleConnect(sessionID, ownerID string, maxInFlight int, conn *websocket.Conn, connMutex *sync.Mutex) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check rate limit (per owner, not per session)
	if r.ownerConsumerCount[ownerID] >= 100 {
		metrics.RateLimitExceeded.WithLabelValues(ownerID).Inc()
		return fmt.Errorf("rate limit exceeded: owner %s has 100 active consumers", ownerID)
	}

	// Create session with shared mutex from WSSession
	session := &Session{
		ID:            sessionID,
		OwnerID:       ownerID,
		MaxInFlight:   maxInFlight,
		Conn:          conn,
		Subscriptions: make(map[string]*ClientSubscription),
		sendMu:        connMutex, // Use mutex from WSSession to prevent concurrent writes
	}

	// Add to maps
	r.sessions[sessionID] = session
	r.sessionsByOwner[ownerID] = append(r.sessionsByOwner[ownerID], sessionID)

	// Check if consumer exists for this owner
	if _, exists := r.consumers[ownerID]; !exists {
		// First session for this owner - create consumer
		consumerName := fmt.Sprintf("owner-%s", ownerID)

		// Use configured value or calculate from clients
		maxAckPending := r.natsConfig.ConsumerMaxAckPending
		if maxAckPending == 0 {
			// If not configured, use sum of client maxInFlight values
			maxAckPending = maxInFlight
		}

		consumer, err := r.js.CreateOrUpdateConsumer(r.ctx, "NOTIFICATIONS", jetstream.ConsumerConfig{
			Name:              consumerName,
			Durable:           consumerName,
			FilterSubjects:    []string{}, // Empty initially, updated on subscribe
			AckPolicy:         jetstream.AckExplicitPolicy,
			MaxAckPending:     maxAckPending,
			AckWait:           time.Duration(r.natsConfig.ConsumerAckWaitSeconds) * time.Second,
			MaxDeliver:        r.natsConfig.ConsumerMaxDeliver,
			DeliverPolicy:     jetstream.DeliverAllPolicy,
			InactiveThreshold: 0,
			Description:       fmt.Sprintf("Owner consumer for %s", ownerID),
		})
		if err != nil {
			return fmt.Errorf("failed to create consumer: %w", err)
		}

		r.consumers[ownerID] = consumer
		r.ownerConsumerCount[ownerID] = 1

		// Create owner-specific context
		ctx, cancel := context.WithCancel(r.ctx)
		r.ownerContexts[ownerID] = ctx
		r.ownerCancels[ownerID] = cancel

		// Start router goroutine (ONE per owner)
		go r.routeNotificationsForOwner(ownerID)

		// Metrics
		metrics.ConsumerCreated.WithLabelValues(ownerID).Inc()
		metrics.ActiveConsumers.WithLabelValues(ownerID).Set(1)

		// Created NATS consumer for owner
	} else {
		// Consumer exists, update MaxAckPending (sum of all sessions)
		r.updateConsumerMaxAckPending(ownerID)
		// Session joined existing consumer
	}

	return nil
}

// HandleDisconnect removes session and cleans up owner consumer if last session
func (r *NotificationRouter) HandleDisconnect(sessionID string) error {
	r.mu.Lock()
	session, exists := r.sessions[sessionID]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ownerID := session.OwnerID
	consumerName := fmt.Sprintf("owner-%s", ownerID)

	// Remove from sessions map
	delete(r.sessions, sessionID)

	// Remove from sessionsByOwner
	sessions := r.sessionsByOwner[ownerID]
	for i, sid := range sessions {
		if sid == sessionID {
			r.sessionsByOwner[ownerID] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}

	// Remove from subscription index
	if ownerSubs, exists := r.subscriptionIndex[ownerID]; exists {
		for key, sessionIDs := range ownerSubs {
			for i, sid := range sessionIDs {
				if sid == sessionID {
					r.subscriptionIndex[ownerID][key] = append(sessionIDs[:i], sessionIDs[i+1:]...)
					break
				}
			}
			// Clean up empty keys
			if len(r.subscriptionIndex[ownerID][key]) == 0 {
				delete(r.subscriptionIndex[ownerID], key)
			}
		}
		// Clean up empty owner map
		if len(r.subscriptionIndex[ownerID]) == 0 {
			delete(r.subscriptionIndex, ownerID)
		}
	}

	// If last session for this owner, delete consumer and stop goroutine
	if len(r.sessionsByOwner[ownerID]) == 0 {
		delete(r.sessionsByOwner, ownerID)

		// Cancel owner context (stops router goroutine)
		if cancel := r.ownerCancels[ownerID]; cancel != nil {
			cancel()
		}
		delete(r.ownerContexts, ownerID)
		delete(r.ownerCancels, ownerID)

		// Delete NATS consumer
		if err := r.js.DeleteConsumer(r.ctx, "NOTIFICATIONS", consumerName); err != nil {
			log.Printf("⚠️ Failed to delete consumer %s: %v", consumerName, err)
		} else {
			// Deleted NATS consumer
		}

		delete(r.consumers, ownerID)
		r.ownerConsumerCount[ownerID]--

		metrics.ConsumerDeleted.WithLabelValues(ownerID).Inc()
		metrics.ActiveConsumers.WithLabelValues(ownerID).Set(0)
	} else {
		// Update consumer (recalculate MaxAckPending and FilterSubjects)
		r.updateConsumerMaxAckPending(ownerID)
	}

	r.mu.Unlock()

	// Session disconnected
	return nil
}

// HandleSubscribe updates the consumer's FilterSubjects to include the new subscription
func (r *NotificationRouter) HandleSubscribe(sessionID, stepName, contract string) error {
	r.mu.RLock()
	session, exists := r.sessions[sessionID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ownerID := session.OwnerID

	// Store subscription in session
	session.mu.Lock()
	if session.Subscriptions == nil {
		session.Subscriptions = make(map[string]*ClientSubscription)
	}
	session.Subscriptions[stepName] = &ClientSubscription{
		StepName:     stepName,
		ContractName: contract,
		EventTypes:   []string{},
	}
	session.mu.Unlock()

	// Build composite key
	subscriptionKey := buildSubscriptionKey(stepName, contract)

	// Update subscription index
	r.mu.Lock()
	if r.subscriptionIndex[ownerID] == nil {
		r.subscriptionIndex[ownerID] = make(map[string][]string)
	}

	sessions := r.subscriptionIndex[ownerID][subscriptionKey]
	if !contains(sessions, sessionID) {
		r.subscriptionIndex[ownerID][subscriptionKey] = append(sessions, sessionID)
	}

	// Update NATS consumer FilterSubjects
	err := r.updateConsumerMaxAckPending(ownerID) // Also updates FilterSubjects
	r.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to update consumer: %w", err)
	}

	// Session subscribed
	return nil
}

// routeNotificationsForOwner is the main router goroutine (ONE per owner)
func (r *NotificationRouter) routeNotificationsForOwner(ownerID string) {
	r.mu.RLock()
	consumer := r.consumers[ownerID]
	ownerCtx := r.ownerContexts[ownerID]
	r.mu.RUnlock()

	if consumer == nil || ownerCtx == nil {
		return
	}

	// Starting notification router

	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		// Parse notification
		var notification models.ValidationNotification
		if err := json.Unmarshal(msg.Data(), &notification); err != nil {
			log.Printf("⚠️ Failed to parse notification: %v", err)
			msg.Ack()
			return
		}

		// Get matching sessions using composite key index
		r.mu.RLock()
		matchingSessions := r.getMatchingSessions(ownerID, notification.StepName, notification.ContractName)
		r.mu.RUnlock()

		if len(matchingSessions) == 0 {
			// No sessions subscribed, ACKing
			msg.Ack()
			return
		}

		// Pick random session (load balance)
		idx := rand.Intn(len(matchingSessions))
		targetSessionID := matchingSessions[idx]

		r.mu.RLock()
		targetSession := r.sessions[targetSessionID]
		r.mu.RUnlock()

		if targetSession == nil {
			log.Printf("⚠️ Target session %s not found, NAKing", targetSessionID)
			msg.Nak()
			return
		}

		// Create ACK token
		metadata, err := msg.Metadata()
		if err != nil {
			log.Printf("⚠️ Failed to get metadata: %v", err)
			msg.Nak()
			return
		}

		ackToken := createAckToken(metadata.Sequence.Stream, msg.Reply())

		// Send to WebSocket
		if err := targetSession.sendNotificationWithAckToken(&notification, ackToken); err != nil {
			log.Printf("⚠️ Failed to send to session %s: %v", targetSessionID, err)
			msg.Nak()
			return
		}

		// Routed notification to session

		// Track metrics
		metrics.NotificationsSent.WithLabelValues(
			notification.OwnerID,
			notification.StepName,
			notification.ContractName,
			notification.Status,
		).Inc()
	})

	if err != nil {
		log.Printf("⚠️ Failed to start consumer for owner %s: %v", ownerID, err)
		return
	}

	// Wait for owner context cancellation
	<-ownerCtx.Done()

	// Stop consuming
	if consumeCtx != nil {
		consumeCtx.Stop()
	}

	// Stopped notification router
}

// HandleAck handles client ACK using opaque ACK token
func (r *NotificationRouter) HandleAck(ackToken string) error {
	// Decode ACK token to get sequence and reply subject
	sequence, replySubject, err := decodeAckToken(ackToken)
	if err != nil {
		return fmt.Errorf("invalid ACK token: %w", err)
	}

	// Publish ACK directly to NATS reply subject
	err = r.nc.Publish(replySubject, []byte("+ACK"))
	if err != nil {
		log.Printf("❌ Failed to ACK NATS message seq=%d: %v", sequence, err)
		return fmt.Errorf("failed to ACK NATS message: %w", err)
	}

	// ACKed NATS message

	// Track ACK metric (we don't have ownerID here, so track without label)
	metrics.NotificationsAcked.WithLabelValues("unknown").Inc()

	return nil
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

// SendMessage sends any message to the WebSocket client with mutex protection
// This is the ONLY method that should be used for WebSocket writes to prevent concurrent write panics
func (s *Session) SendMessage(message interface{}) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.Conn.WriteJSON(message)
}

// sendNotification sends a notification to the WebSocket client
func (c *WebSocketClient) sendNotification(notification *models.ValidationNotification) error {
	message := map[string]interface{}{
		"action":       "notification",
		"notification": notification,
	}
	return c.SendMessage(message)
}

// sendNotificationWithAckToken sends a notification with ACK token to the WebSocket client
func (c *WebSocketClient) sendNotificationWithAckToken(notification *models.ValidationNotification, ackToken string) error {
	message := map[string]interface{}{
		"action":       "notification",
		"ackToken":     ackToken,
		"notification": notification,
	}
	return c.SendMessage(message)
}

// buildSubscriptionKey creates a composite key for subscription indexing
func buildSubscriptionKey(stepName, contract string) string {
	if contract != "" {
		return fmt.Sprintf("%s@%s", stepName, contract)
	}
	return fmt.Sprintf("%s@*", stepName)
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getMatchingSessions returns sessions subscribed to the given step and contract (O(1) lookup)
func (r *NotificationRouter) getMatchingSessions(ownerID, stepName, contract string) []string {
	if contract == "" {
		contract = "global" // Publisher uses "global" for no contract
	}

	// Lookup specific contract
	specificKey := buildSubscriptionKey(stepName, contract)
	specificSessions := r.subscriptionIndex[ownerID][specificKey]

	// Lookup wildcard
	wildcardKey := buildSubscriptionKey(stepName, "")
	wildcardSessions := r.subscriptionIndex[ownerID][wildcardKey]

	// Combine and return
	return append(append([]string{}, specificSessions...), wildcardSessions...)
}

// calculateMaxAckPending calculates total MaxAckPending for owner (sum of all sessions)
func (r *NotificationRouter) calculateMaxAckPending(ownerID string) int {
	total := 0
	for _, sessionID := range r.sessionsByOwner[ownerID] {
		if session := r.sessions[sessionID]; session != nil {
			total += session.MaxInFlight
		}
	}
	if total == 0 {
		return 10
	}
	return total
}

// updateConsumerMaxAckPending updates consumer with recalculated MaxAckPending and FilterSubjects
func (r *NotificationRouter) updateConsumerMaxAckPending(ownerID string) error {
	allFilters := r.buildUnionFilterSubjects(ownerID)

	// Use configured value or calculate from clients
	maxAckPending := r.natsConfig.ConsumerMaxAckPending
	if maxAckPending == 0 {
		// If not configured, use sum of client maxInFlight values
		maxAckPending = r.calculateMaxAckPending(ownerID)
	}

	_, err := r.js.UpdateConsumer(r.ctx, "NOTIFICATIONS", jetstream.ConsumerConfig{
		Name:              fmt.Sprintf("owner-%s", ownerID),
		Durable:           fmt.Sprintf("owner-%s", ownerID),
		FilterSubjects:    allFilters,
		AckPolicy:         jetstream.AckExplicitPolicy,
		MaxAckPending:     maxAckPending,
		AckWait:           time.Duration(r.natsConfig.ConsumerAckWaitSeconds) * time.Second,
		MaxDeliver:        r.natsConfig.ConsumerMaxDeliver,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: 0,
	})

	return err
}

// buildUnionFilterSubjects builds union of FilterSubjects from all sessions
func (r *NotificationRouter) buildUnionFilterSubjects(ownerID string) []string {
	filterMap := make(map[string]bool)

	for _, sessionID := range r.sessionsByOwner[ownerID] {
		session := r.sessions[sessionID]
		if session == nil {
			continue
		}

		session.mu.RLock()
		for _, sub := range session.Subscriptions {
			var filterSubject string
			if sub.ContractName != "" {
				filterSubject = fmt.Sprintf("notifications.user.%s.%s.%s",
					ownerID, sub.ContractName, sub.StepName)
			} else {
				filterSubject = fmt.Sprintf("notifications.user.%s.*.%s",
					ownerID, sub.StepName)
			}
			filterMap[filterSubject] = true
		}
		session.mu.RUnlock()
	}

	result := make([]string, 0, len(filterMap))
	for filter := range filterMap {
		result = append(result, filter)
	}
	return result
}

// Stop gracefully stops the notification router
func (r *NotificationRouter) Stop() {
	// Stopping notification router
	r.cancel()
	// Notification router stopped
}

// createAckToken creates an opaque ACK token from sequence and reply subject
// Format: base64(sequence:replySubject)
func createAckToken(sequence uint64, replySubject string) string {
	payload := strconv.FormatUint(sequence, 10) + ":" + replySubject
	return base64.URLEncoding.EncodeToString([]byte(payload))
}

// decodeAckToken decodes an ACK token to extract sequence and reply subject
func decodeAckToken(token string) (uint64, string, error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return 0, "", fmt.Errorf("invalid token encoding: %w", err)
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid token format")
	}

	sequence, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid sequence: %w", err)
	}

	return sequence, parts[1], nil
}
