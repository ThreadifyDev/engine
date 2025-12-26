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
	"github.com/gorilla/websocket"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:      func(r *http.Request) bool { return true },
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
}

type WebSocketHandler struct {
	threadService     *service.ThreadService
	stepEventService  *service.StepEventService
	invitationService *service.InvitationTokenService
	auditService      *service.AuditEventService
	sessions          sync.Map
}

type Session struct {
	conn      *websocket.Conn
	ownerID   string
	companyID string
	threadIDs []string
	mu        sync.Mutex
}

func NewWebSocketHandler(threadService *service.ThreadService, stepEventService *service.StepEventService, invitationService *service.InvitationTokenService, auditService *service.AuditEventService) *WebSocketHandler {
	return &WebSocketHandler{
		threadService:     threadService,
		stepEventService:  stepEventService,
		invitationService: invitationService,
		auditService:      auditService,
	}
}

// Helper function to check if threadID exists in slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	session := &Session{conn: conn}

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
		}
		response = resp

	case "startThread":
		var req models.StartThreadRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleStartThread(&req, session.ownerID, session.companyID)

		// Add created thread to session's threadIDs
		if startResp, ok := response.(*models.StartThreadResponse); ok && startResp.Status == "success" {
			session.mu.Lock()
			if !contains(session.threadIDs, startResp.ThreadID) {
				session.threadIDs = append(session.threadIDs, startResp.ThreadID)
			}
			session.mu.Unlock()
		}

	case "recordThreadEvent":
		var req models.RecordEventRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleRecordEvent(&req, session.ownerID, session.companyID)

	case "stepEvent":
		var req models.StepEvent
		json.Unmarshal(msgBytes, &req)
		response = h.handleStepEvent(&req, session.ownerID)

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

func (h *WebSocketHandler) handleStepEvent(req *models.StepEvent, ownerID string) interface{} {
	// Validate ownership
	if req.ThreadID == "" {
		return models.ErrorResponse{
			Action:  "stepEvent",
			Status:  "error",
			Message: "thread_id is required",
		}
	}

	// Queue for async processing
	if err := h.stepEventService.ProcessStepEvent(*req); err != nil {
		return models.ErrorResponse{
			Action:  "stepEvent",
			Status:  "error",
			Message: fmt.Sprintf("Failed to process step event: %v", err),
		}
	}

	return models.StepEventResponse{
		Action:  "stepEvent",
		Status:  "success",
		Message: "Step event queued for processing",
		StepID:  req.StepID,
	}
}

func (h *WebSocketHandler) handleInviteParty(session *Session, req *models.InvitePartyRequest) interface{} {
	// Validate request
	if req.Action != "inviteParty" {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: "Invalid action",
		}
	}

	// Set default permissions if not provided
	permissions := req.Permissions
	if permissions == "" {
		permissions = "read,write"
	}

	// Validate role
	if err := h.invitationService.ValidateRole(req.Role, nil); err != nil {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: err.Error(),
		}
	}

	// Validate permissions
	if err := h.invitationService.ValidatePermissions(permissions); err != nil {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: err.Error(),
		}
	}

	// Parse expiry
	expiry, err := h.invitationService.ParseExpiry(req.ExpiresIn)
	if err != nil {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: fmt.Sprintf("Invalid expiry format: %v", err),
		}
	}

	// Get thread context from session's threads
	// For now, we'll use the first thread in the session's threadIDs
	// In a real implementation, this might be passed in the request or be the "active" thread
	var threadID string
	session.mu.Lock()
	if len(session.threadIDs) > 0 {
		threadID = session.threadIDs[0] // Use first available thread
	}
	session.mu.Unlock()

	if threadID == "" {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: "No active thread found. Please start a thread first.",
		}
	}

	contractID := "contract-123" // This would come from thread data

	// Create JWT token
	threadToken, err := h.invitationService.CreateToken(threadID, contractID, session.ownerID, req.Role, permissions, expiry)
	if err != nil {
		return models.ErrorResponse{
			Action:  "inviteParty",
			Status:  "error",
			Message: fmt.Sprintf("Failed to create invitation token: %v", err),
		}
	}

	// Log audit event (async, don't fail if audit fails)
	go func() {
		if h.auditService != nil {
			ctx := context.Background()
			h.auditService.LogTokenCreated(ctx, threadID, contractID, session.ownerID, req.Role, permissions)
		}
	}()

	return models.InvitePartyResponse{
		Action:      "inviteParty",
		Status:      "success",
		ThreadToken: threadToken,
		Role:        req.Role,
		Permissions: permissions,
		ExpiresAt:   time.Now().Add(expiry).Unix(),
		Message:     "Invitation token created successfully",
	}
}

func (h *WebSocketHandler) handleJoinThread(session *Session, req *models.JoinThreadRequest) interface{} {
	// Validate request
	if req.Action != "joinThread" {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: "Invalid action",
		}
	}

	if req.ThreadToken == "" {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: "Thread token is required",
		}
	}

	// Validate JWT token and extract claims
	claims, err := h.invitationService.ValidateToken(req.ThreadToken)
	if err != nil {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: fmt.Sprintf("Invalid thread token: %v", err),
		}
	}

	// Store role in Valkey (with in-memory cache)
	err = h.threadService.AssignThreadRole(claims.ThreadID, claims.Role, session.ownerID)
	if err != nil {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: fmt.Sprintf("Failed to assign role: %v", err),
		}
	}

	// Store permissions in Valkey (with in-memory cache)
	permissions := strings.Split(claims.Permissions, ",") // "read,write" -> ["read", "write"]
	err = h.threadService.SetThreadPermissions(claims.ThreadID, session.ownerID, permissions)
	if err != nil {
		return models.ErrorResponse{
			Action:  "joinThread",
			Status:  "error",
			Message: fmt.Sprintf("Failed to set permissions: %v", err),
		}
	}

	// Update session with thread context
	session.mu.Lock()
	if !contains(session.threadIDs, claims.ThreadID) {
		session.threadIDs = append(session.threadIDs, claims.ThreadID)
	}
	session.mu.Unlock()

	// Log audit events (async, don't fail if audit fails)
	go func() {
		if h.auditService != nil {
			ctx := context.Background()
			// Log token usage
			h.auditService.LogTokenUsed(ctx, claims.ExpiresAt.Time, claims.ThreadID, session.ownerID)
			// Log thread joined
			h.auditService.LogThreadJoined(ctx, claims.ThreadID, claims.ContractID, session.ownerID)
		}
	}()

	return models.JoinThreadResponse{
		Action:      "joinThread",
		Status:      "success",
		ThreadID:    claims.ThreadID,
		ContractID:  claims.ContractID,
		Role:        claims.Role,
		Permissions: claims.Permissions,
		Message:     "Successfully joined thread",
	}
}
