package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/utils"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:      func(r *http.Request) bool { return true },
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
}

type WebSocketHandler struct {
	threadService        *service.ThreadService
	stepEventService     *service.StepEventService
	invitationService    *service.InvitationTokenService
	notificationConsumer *service.NotificationConsumer
	notificationRouter   *NotificationRouter
	valkeyClient         interfaces.ValkeyClient
	sessions             sync.Map
}

type Session struct {
	conn                *websocket.Conn
	clientID            string
	ownerID             string
	companyID           string
	threadIDs           []string
	notificationHandler *WebSocketNotificationHandler
	mu                  sync.Mutex
}

func NewWebSocketHandler(threadService *service.ThreadService, stepEventService *service.StepEventService, invitationService *service.InvitationTokenService, notificationConsumer *service.NotificationConsumer, notificationRouter *NotificationRouter, valkeyClient interfaces.ValkeyClient) *WebSocketHandler {
	return &WebSocketHandler{
		threadService:        threadService,
		stepEventService:     stepEventService,
		invitationService:    invitationService,
		notificationConsumer: notificationConsumer,
		notificationRouter:   notificationRouter,
		valkeyClient:         valkeyClient,
	}
}

// Helper function to check if threadID exists in slice

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	// Generate unique client ID
	clientID := uuid.New().String()
	session := &Session{
		conn:     conn,
		clientID: clientID,
	}

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		action, _ := msg["action"].(string)
		response := h.handleMessage(action, msg, session)

		if err := conn.WriteJSON(response); err != nil {
			break
		}

		// Check if this was a close connection request
		if action == "closeConnection" {
			break
		}
	}

	if session.ownerID != "" {
		h.sessions.Delete(session.ownerID)
		h.threadService.HandleClose(session.ownerID)

		// Unsubscribe from all notifications (old consumer)
		h.unsubscribeFromNotifications(session)

		// Unregister from notification router (new router)
		if h.notificationRouter != nil {
			h.notificationRouter.UnregisterClient(session.clientID)
		}
	}
}

func (h *WebSocketHandler) handleMessage(action string, msg map[string]interface{}, session *Session) interface{} {
	startTime := time.Now()
	msgBytes, _ := json.Marshal(msg)

	var response interface{}
	switch action {
	case "connect":
		var req models.ConnectRequest
		json.Unmarshal(msgBytes, &req)
		resp := h.threadService.HandleConnect(&req)
		if resp.Status == "success" {
			session.mu.Lock()
			session.ownerID = resp.OwnerID     // Use ownerID from response, not request
			session.companyID = resp.CompanyID // Set companyID from response
			session.mu.Unlock()
			h.sessions.Store(resp.OwnerID, session) // Use ownerID from response

			// Register client with notification router
			if h.notificationRouter != nil {
				wsClient := &WebSocketClient{
					ID:          session.clientID,
					Conn:        session.conn,
					OwnerID:     session.ownerID,
					ThreadIDs:   make(map[string]bool),
					pendingAcks: make(map[string]*PendingNotification),
				}
				h.notificationRouter.RegisterClient(wsClient)
			}
		}
		response = resp

	case "startThread":
		var req models.StartThreadRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleStartThread(&req, session.ownerID, session.companyID)

		// Add created thread to session's threadIDs
		if startResp, ok := response.(*models.StartThreadResponse); ok && startResp.Status == "success" {
			session.mu.Lock()
			if !utils.Contains(session.threadIDs, startResp.ThreadID) {
				session.threadIDs = append(session.threadIDs, startResp.ThreadID)
			}
			session.mu.Unlock()

			// Subscribe to notifications for this thread (old consumer)
			h.subscribeToNotifications(session, startResp.ThreadID, "owner")

			// Subscribe via notification router (new router)
			if h.notificationRouter != nil {
				h.notificationRouter.SubscribeToThread(session.clientID, startResp.ThreadID)
			}
		}

	case "recordThreadEvent":
		var req models.RecordEventRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleRecordEvent(&req, session.ownerID, session.companyID)

	case "closeConnection":
		resp := h.threadService.HandleClose(session.ownerID)
		// Response will be sent, then connection will close gracefully in main loop
		response = resp

	case "inviteParty":
		var req models.InvitePartyRequest
		json.Unmarshal(msgBytes, &req)
		response = h.handleInviteParty(session, &req)

	case "joinThread":
		var req models.JoinThreadRequest
		json.Unmarshal(msgBytes, &req)
		response = h.handleJoinThread(session, &req)

	case "ack_notification":
		var ackMsg NotificationACKMessage
		json.Unmarshal(msgBytes, &ackMsg)
		response = h.handleNotificationAck(session, &ackMsg)

	default:
		response = models.ErrorResponse{
			Action:  "error",
			Status:  "error",
			Message: "Unknown action: " + action,
		}
	}

	// Log timing metrics
	duration := time.Since(startTime)
	log.Printf("[%s] Request processed in %v", action, duration)

	return response
}

func (h *WebSocketHandler) handleInviteParty(session *Session, req *models.InvitePartyRequest) interface{} {
	// Validate request action
	if req.Action != "inviteParty" {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: "Invalid action",
		}
	}

	// Get thread IDs from session
	session.mu.Lock()
	threadIDs := make([]string, len(session.threadIDs))
	copy(threadIDs, session.threadIDs)
	session.mu.Unlock()

	// Call service layer
	response, err := h.threadService.HandleInviteParty(req, session.ownerID, session.companyID, threadIDs)
	if err != nil {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: err.Error(),
		}
	}

	return response
}

func (h *WebSocketHandler) handleJoinThread(session *Session, req *models.JoinThreadRequest) interface{} {
	// Validate request action
	if req.Action != "joinThread" {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: "Invalid action",
		}
	}

	// Call service layer
	response, err := h.threadService.HandleJoinThread(req, session.ownerID, session.companyID)
	if err != nil {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: err.Error(),
		}
	}

	// Update session with thread context (handler layer responsibility)
	if response.Status == "success" {
		session.mu.Lock()
		session.threadIDs = append(session.threadIDs, response.ThreadID)
		session.mu.Unlock()

		// Subscribe to notifications for joined thread
		if h.notificationRouter != nil {
			h.notificationRouter.SubscribeToThread(session.clientID, response.ThreadID)
		}
	}

	return response
}

// subscribeToNotifications subscribes a session to notifications for a thread
func (h *WebSocketHandler) subscribeToNotifications(session *Session, threadID, scope string) {
	if h.notificationConsumer == nil {
		log.Println("[WS-NOTIFICATION] Notification consumer not available")
		return
	}

	// Create notification handler for this session
	handler := NewWebSocketNotificationHandler(session.conn, threadID, session.ownerID)

	// Store handler in session
	session.mu.Lock()
	session.notificationHandler = handler
	session.mu.Unlock()

	// Subscribe to NATS notifications
	if err := h.notificationConsumer.Subscribe(threadID, session.ownerID, scope, handler); err != nil {
		log.Printf("[WS-NOTIFICATION] Failed to subscribe to notifications: %v\n", err)
	} else {
		log.Printf("[WS-NOTIFICATION] Subscribed user %s to thread %s notifications (scope: %s)\n",
			session.ownerID, threadID, scope)
	}
}

// unsubscribeFromNotifications unsubscribes a session from all thread notifications
func (h *WebSocketHandler) unsubscribeFromNotifications(session *Session) {
	if h.notificationConsumer == nil {
		return
	}

	session.mu.Lock()
	threadIDs := session.threadIDs
	ownerID := session.ownerID
	session.mu.Unlock()

	// Unsubscribe from all threads
	for _, threadID := range threadIDs {
		if err := h.notificationConsumer.Unsubscribe(threadID, ownerID); err != nil {
			log.Printf("[WS-NOTIFICATION] Failed to unsubscribe from thread %s: %v\n", threadID, err)
		}
	}

	// Close notification handler
	session.mu.Lock()
	if session.notificationHandler != nil {
		session.notificationHandler.Close()
		session.notificationHandler = nil
	}
	session.mu.Unlock()
}

// handleNotificationAck handles ACK messages from clients
func (h *WebSocketHandler) handleNotificationAck(session *Session, ackMsg *NotificationACKMessage) interface{} {
	if h.notificationRouter == nil {
		return models.ErrorResponse{
			Action:  "ack_notification",
			Status:  "error",
			Message: "Notification router not available",
		}
	}

	// Get the WebSocket client from the router
	h.notificationRouter.mu.RLock()
	client, exists := h.notificationRouter.clients[session.clientID]
	h.notificationRouter.mu.RUnlock()

	if !exists {
		return models.ErrorResponse{
			Action:  "ack_notification",
			Status:  "error",
			Message: "Client not registered",
		}
	}

	// Handle the ACK
	if err := client.HandleClientAck(ackMsg.NotificationID); err != nil {
		log.Printf("[WS-ACK] Error handling ACK for notification %s: %v", ackMsg.NotificationID, err)
		return models.ErrorResponse{
			Action:  "ack_notification",
			Status:  "error",
			Message: err.Error(),
		}
	}

	// Return success response
	return map[string]interface{}{
		"action":          "ack_notification",
		"status":          "success",
		"notification_id": ackMsg.NotificationID,
	}
}
