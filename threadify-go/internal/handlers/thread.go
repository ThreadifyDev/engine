package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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
	threadService     *service.ThreadService
	stepEventService  *service.StepEventService
	invitationService *service.InvitationTokenService
	valkeyClient      interfaces.ValkeyClient
	sessions          sync.Map
}

type Session struct {
	conn      *websocket.Conn
	ownerID   string
	companyID string
	threadIDs []string
	mu        sync.Mutex
}

func NewWebSocketHandler(threadService *service.ThreadService, stepEventService *service.StepEventService, invitationService *service.InvitationTokenService, valkeyClient interfaces.ValkeyClient) *WebSocketHandler {
	return &WebSocketHandler{
		threadService:     threadService,
		stepEventService:  stepEventService,
		invitationService: invitationService,
		valkeyClient:      valkeyClient,
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
			if !utils.Contains(session.threadIDs, startResp.ThreadID) {
				session.threadIDs = append(session.threadIDs, startResp.ThreadID)
			}
			session.mu.Unlock()
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
	}

	return response
}
