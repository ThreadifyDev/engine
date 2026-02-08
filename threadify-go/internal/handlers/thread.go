package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/utils"
)

// upgrader will be initialized with config values
var upgrader websocket.Upgrader

type WebSocketHandler struct {
	threadService        *service.ThreadService
	stepEventService     *service.StepEventService
	invitationService    *service.InvitationTokenService
	notificationConsumer *service.NotificationConsumer
	notificationRouter   *NotificationRouter
	valkeyClient         interfaces.ValkeyClient
	sessions             sync.Map
	luaScriptManager     interfaces.LuaScriptManager
	rateLimitConfig      *config.RateLimitConfig
	websocketConfig      *config.WebSocketConfig
}

type WSSession struct {
	conn      *websocket.Conn
	sessionID string
	ownerID   string
	companyID string
	threadIDs []string
	mu        sync.Mutex
	sendMu    sync.Mutex // Protects WebSocket writes
}

// NotificationACKMessage represents a client ACK message
type NotificationACKMessage struct {
	Action         string `json:"action"`
	NotificationID string `json:"notification_id"`
	ThreadID       string `json:"thread_id"`
	Processed      bool   `json:"processed"`
	AckToken       string `json:"ackToken"` // Opaque token for stateless ACK (base64 encoded)
}

func NewWebSocketHandler(threadService *service.ThreadService, stepEventService *service.StepEventService, invitationService *service.InvitationTokenService, notificationConsumer *service.NotificationConsumer, notificationRouter *NotificationRouter, valkeyClient interfaces.ValkeyClient, luaScriptManager interfaces.LuaScriptManager, rateLimitConfig *config.RateLimitConfig, websocketConfig *config.WebSocketConfig) *WebSocketHandler {
	// Initialize upgrader with config values
	upgrader = websocket.Upgrader{
		CheckOrigin:       func(r *http.Request) bool { return true },
		HandshakeTimeout:  time.Duration(websocketConfig.HandshakeTimeoutSeconds) * time.Second,
		ReadBufferSize:    websocketConfig.ReadBufferSize,
		WriteBufferSize:   websocketConfig.WriteBufferSize,
		EnableCompression: true, // Enable per-message compression (reduces bandwidth by 60-80%)
	}

	return &WebSocketHandler{
		threadService:        threadService,
		stepEventService:     stepEventService,
		invitationService:    invitationService,
		notificationConsumer: notificationConsumer,
		notificationRouter:   notificationRouter,
		valkeyClient:         valkeyClient,
		luaScriptManager:     luaScriptManager,
		rateLimitConfig:      rateLimitConfig,
		websocketConfig:      websocketConfig,
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

	// Generate unique session ID
	sessionID := uuid.New().String()
	session := &WSSession{
		conn:      conn,
		sessionID: sessionID,
	}

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		action, _ := msg["action"].(string)
		response := h.handleMessage(action, msg, session)

		// Use session.SendMessage for thread-safe writes
		if err := session.SendMessage(response); err != nil {
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

		// Disconnect from notification router (delete consumer)
		if h.notificationRouter != nil {
			if err := h.notificationRouter.HandleDisconnect(session.sessionID); err != nil {
				log.Printf("Failed to disconnect session %s: %v", session.sessionID, err)
			}
		}
	}
}

func (h *WebSocketHandler) handleMessage(action string, msg map[string]interface{}, session *WSSession) interface{} {
	// Track WebSocket message handling latency
	wsStart := time.Now()
	sessionID := session.sessionID
	if sessionID == "" {
		sessionID = "unknown"
	}
	log.Printf("[PERF] WebSocket START: action=%s | session=%s", action, sessionID)

	defer func() {
		duration := time.Since(wsStart)
		log.Printf("[PERF] WebSocket COMPLETE: action=%s | session=%s | duration=%v", action, sessionID, duration)

		// Record metrics
		metrics.RequestDuration.WithLabelValues(action).Observe(duration.Seconds())
	}()

	msgBytes, _ := json.Marshal(msg)

	// Rate limit authenticated WebSocket messages (skip "connect" action)
	if action != "connect" && session.ownerID != "" && h.rateLimitConfig != nil && h.rateLimitConfig.PerUser.Enabled {
		rateLimitStart := time.Now()
		allowed, err := h.luaScriptManager.CheckUserRateLimit(
			context.Background(),
			session.ownerID,
			h.rateLimitConfig.PerUser.RequestsPerMinute,
			h.rateLimitConfig.PerUser.WindowSeconds,
		)
		rateLimitDuration := time.Since(rateLimitStart)
		log.Printf("[PERF] WebSocket RATE_LIMIT: action=%s | session=%s | duration=%v | allowed=%t", action, sessionID, rateLimitDuration, allowed)

		if err == nil && !allowed {
			log.Printf("[PERF] WebSocket RATE_LIMIT_EXCEEDED: action=%s | session=%s", action, sessionID)
			return models.ErrorResponse{
				Action:  action,
				Status:  "error",
				Message: "Rate limit exceeded. Please slow down.",
			}
		}
	} else {
		log.Printf("[PERF] WebSocket RATE_LIMIT: action=%s | session=%s | SKIPPED", action, sessionID)
	}

	var response interface{}
	switch action {
	case "connect":
		connectStart := time.Now()
		var req models.ConnectRequest
		json.Unmarshal(msgBytes, &req)
		resp := h.threadService.HandleConnect(&req)
		connectDuration := time.Since(connectStart)
		log.Printf("[PERF] WebSocket CONNECT: session=%s | duration=%v | success=%t", sessionID, connectDuration, resp.Status == "success")

		if resp.Status == "success" {
			session.mu.Lock()
			session.ownerID = resp.OwnerID
			session.companyID = resp.CompanyID
			session.mu.Unlock()
			h.sessions.Store(resp.OwnerID, session)

			// Create NATS consumer and start push for this session
			if h.notificationRouter != nil {
				// Use client-specified maxInFlight with validation from config
				maxInFlight := req.MaxInFlight
				if maxInFlight < 1 || maxInFlight > h.websocketConfig.MaxInFlightMax {
					maxInFlight = h.websocketConfig.MaxInFlightDefault
				}
				if err := h.notificationRouter.HandleConnect(session.sessionID, resp.OwnerID, maxInFlight, session.conn, &session.sendMu); err != nil {
					// Failed to create session consumer (non-fatal)
					log.Printf("[PERF] WebSocket NATS_SETUP_FAILED: session=%s | error=%v", sessionID, err)
				} else {
					log.Printf("[PERF] WebSocket NATS_SETUP: session=%s | success", sessionID)
				}
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

			// Record metrics
			metrics.ThreadsCreated.Inc()
			metrics.RequestsTotal.WithLabelValues("startThread", "success").Inc()
		} else {
			metrics.RequestsTotal.WithLabelValues("startThread", "error").Inc()
		}

	case "recordThreadEvent":
		var req models.RecordEventRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleRecordEvent(&req, session.ownerID, session.companyID)

	case "addRefs":
		var req models.AddRefsRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleAddRefs(&req, session.ownerID)

	case "closeConnection":
		// Don't call HandleClose here - it will be called after loop exits
		// Just return success response and let the loop break
		response = &models.CloseConnectionResponse{
			Action:  "closeConnection",
			Status:  "success",
			Message: "Connection will be closed",
		}

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

	case "subscribe":
		var req struct {
			Action     string   `json:"action"`
			StepName   string   `json:"stepName"`
			EventTypes []string `json:"eventTypes"`
		}
		json.Unmarshal(msgBytes, &req)
		response = h.handleSubscribe(session, &req)

	case "unsubscribe":
		var req struct {
			Action   string `json:"action"`
			StepName string `json:"stepName"`
		}
		json.Unmarshal(msgBytes, &req)
		response = h.handleUnsubscribe(session, &req)

	case "closeThread":
		var req struct {
			Action   string `json:"action"`
			ThreadID string `json:"threadId"`
			Status   string `json:"status"`
			Reason   string `json:"reason,omitempty"`
		}
		json.Unmarshal(msgBytes, &req)
		response = h.handleCloseThread(session, &req)

	default:
		response = models.ErrorResponse{
			Action:  "error",
			Status:  "error",
			Message: "Unknown action: " + action,
		}
	}

	// Request processed

	return response
}

func (h *WebSocketHandler) handleInviteParty(session *WSSession, req *models.InvitePartyRequest) interface{} {
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

func (h *WebSocketHandler) handleJoinThread(session *WSSession, req *models.JoinThreadRequest) interface{} {
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
	}

	return response
}

// unsubscribeFromNotifications unsubscribes a session from all thread notifications
func (h *WebSocketHandler) unsubscribeFromNotifications(session *WSSession) {
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
			// Failed to unsubscribe (non-fatal)
		}
	}

	// Notification cleanup handled by NotificationRouter.HandleDisconnect
}

// handleSubscribe handles subscription requests from clients
// Accepts stepName in format "stepName" or "contract@stepName"
// Accepts eventTypes array: ["violation", "completed", "failed"] or empty for all
func (h *WebSocketHandler) handleSubscribe(session *WSSession, req *struct {
	Action     string   `json:"action"`
	StepName   string   `json:"stepName"`
	EventTypes []string `json:"eventTypes"`
}) interface{} {
	if req.StepName == "" {
		return models.ErrorResponse{
			Action:  "subscribe",
			Status:  "error",
			Message: "Step name is required",
		}
	}

	if h.notificationRouter == nil {
		return models.ErrorResponse{
			Action:  "subscribe",
			Status:  "error",
			Message: "Notification router not available",
		}
	}

	// Parse "contract@stepName" format
	var contractName, stepName string
	if strings.Contains(req.StepName, "@") {
		parts := strings.SplitN(req.StepName, "@", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return models.ErrorResponse{
				Action:  "subscribe",
				Status:  "error",
				Message: "Invalid format. Use 'stepName' or 'contract@stepName'",
			}
		}
		contractName = parts[0]
		stepName = parts[1]
	} else {
		stepName = req.StepName
		contractName = ""
	}

	// Call notification router to update FilterSubjects
	if err := h.notificationRouter.HandleSubscribe(session.sessionID, stepName, contractName); err != nil {
		// Failed to subscribe session
		return models.ErrorResponse{
			Action:  "subscribe",
			Status:  "error",
			Message: fmt.Sprintf("Failed to subscribe: %v", err),
		}
	}

	// Session subscribed to notifications

	return map[string]interface{}{
		"action":  "subscribe",
		"status":  "success",
		"message": fmt.Sprintf("Subscribed to %s", req.StepName),
	}
}

// handleUnsubscribe handles unsubscribe requests from clients (internal cleanup)
func (h *WebSocketHandler) handleUnsubscribe(session *WSSession, req *struct {
	Action   string `json:"action"`
	StepName string `json:"stepName"`
}) interface{} {
	if req.StepName == "" {
		return models.ErrorResponse{
			Action:  "unsubscribe",
			Status:  "error",
			Message: "Step name is required",
		}
	}

	// Unsubscribe not implemented in MVP

	return map[string]interface{}{
		"action":  "unsubscribe",
		"status":  "success",
		"message": fmt.Sprintf("Unsubscribed from %s", req.StepName),
	}
}

// handleNotificationAck handles ACK messages from clients
func (h *WebSocketHandler) handleNotificationAck(session *WSSession, ackMsg *NotificationACKMessage) interface{} {
	if h.notificationRouter == nil {
		return models.ErrorResponse{
			Action:  "ack_notification",
			Status:  "error",
			Message: "Notification router not available",
		}
	}

	// Check if ackToken is provided (stateless ACK with opaque token)
	if ackMsg.AckToken != "" {
		// Stateless ACK using opaque token
		if err := h.notificationRouter.HandleAck(ackMsg.AckToken); err != nil {
			// Error handling ACK
			return models.ErrorResponse{
				Action:  "ack_notification",
				Status:  "error",
				Message: err.Error(),
			}
		}
		// ACKed notification
	} else {
		// Old ACK format (no ackToken)
	}

	// Return success response
	return map[string]interface{}{
		"action":          "ack_notification",
		"status":          "success",
		"notification_id": ackMsg.NotificationID,
	}
}

// handleCloseThread handles thread closure requests
func (h *WebSocketHandler) handleCloseThread(session *WSSession, req *struct {
	Action   string `json:"action"`
	ThreadID string `json:"threadId"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}) interface{} {
	// Validate request
	if req.ThreadID == "" {
		return models.ErrorResponse{
			Action:  "closeThread",
			Status:  "error",
			Message: "Thread ID is required",
		}
	}

	// Default status to 'closed'
	status := req.Status
	if status == "" {
		status = "closed"
	}

	// Validate status
	if status != "closed" && status != "completed" {
		return models.ErrorResponse{
			Action:  "closeThread",
			Status:  "error",
			Message: "Status must be 'closed' or 'completed'",
		}
	}

	// TODO: Check permissions (thread.close)
	// Permission check should be done via thread access - owner and participant roles have thread.close permission

	// Close the thread with current timestamp
	recordedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := h.threadService.CloseThread(
		ctx,
		req.ThreadID,
		session.ownerID,
		"", // Service name - empty for user-initiated close
		status,
		req.Reason,
		recordedAt,
	)
	if err != nil {
		return models.ErrorResponse{
			Action:  "closeThread",
			Status:  "error",
			Message: "Failed to close thread: " + err.Error(),
		}
	}

	// Prepare timestamp field based on status
	timestampField := "closedAt"
	if status == "completed" {
		timestampField = "completedAt"
	}

	// Send success response
	response := map[string]interface{}{
		"action":       "closeThread",
		"status":       "success",
		"threadId":     req.ThreadID,
		"threadStatus": status,
		timestampField: recordedAt.Format(time.RFC3339),
		"message":      "Thread " + status + " successfully",
	}

	return response
}

// SendMessage sends any message to the WebSocket client with mutex protection
// This prevents concurrent write panics when multiple goroutines write to the same connection
func (s *WSSession) SendMessage(message interface{}) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.conn.WriteJSON(message)
}
