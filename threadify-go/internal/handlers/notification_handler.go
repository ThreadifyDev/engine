package handlers

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/threadify/engine/internal/models"
)

// WebSocketNotificationHandler handles notifications for a WebSocket connection
type WebSocketNotificationHandler struct {
	conn     *websocket.Conn
	threadID string
	userID   string
	mu       sync.Mutex
}

// NewWebSocketNotificationHandler creates a new WebSocket notification handler
func NewWebSocketNotificationHandler(conn *websocket.Conn, threadID, userID string) *WebSocketNotificationHandler {
	return &WebSocketNotificationHandler{
		conn:     conn,
		threadID: threadID,
		userID:   userID,
	}
}

// HandleNotification sends a notification to the WebSocket client
func (h *WebSocketNotificationHandler) HandleNotification(notification models.ValidationNotification) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Create WebSocket message
	message := map[string]interface{}{
		"action":       "notification",
		"notification": notification,
		"timestamp":    notification.Timestamp,
	}

	// Send to WebSocket
	if err := h.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send notification to WebSocket: %w", err)
	}

	fmt.Printf("[WS-NOTIFICATION] Sent notification %s to user %s on thread %s\n",
		notification.NotificationID, h.userID, h.threadID)

	return nil
}

// SendNotificationBatch sends multiple notifications at once
func (h *WebSocketNotificationHandler) SendNotificationBatch(notifications []models.ValidationNotification) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	message := map[string]interface{}{
		"action":        "notification_batch",
		"notifications": notifications,
		"count":         len(notifications),
	}

	if err := h.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send notification batch to WebSocket: %w", err)
	}

	fmt.Printf("[WS-NOTIFICATION] Sent %d notifications to user %s on thread %s\n",
		len(notifications), h.userID, h.threadID)

	return nil
}

// SendACK sends an acknowledgment back to the client
func (h *WebSocketNotificationHandler) SendACK(notificationID string, success bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	message := map[string]interface{}{
		"action":          "notification_ack",
		"notification_id": notificationID,
		"success":         success,
	}

	if err := h.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send ACK to WebSocket: %w", err)
	}

	return nil
}

// Close closes the handler (cleanup if needed)
func (h *WebSocketNotificationHandler) Close() error {
	// Any cleanup needed
	return nil
}

// NotificationACKMessage represents a client ACK message
type NotificationACKMessage struct {
	Action         string `json:"action"`
	NotificationID string `json:"notification_id"`
	ThreadID       string `json:"thread_id"`
	Processed      bool   `json:"processed"`
}

// ParseACKMessage parses an ACK message from the client
func ParseACKMessage(data []byte) (*NotificationACKMessage, error) {
	var msg NotificationACKMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse ACK message: %w", err)
	}

	if msg.Action != "ack_notification" {
		return nil, fmt.Errorf("invalid action: %s", msg.Action)
	}

	return &msg, nil
}
