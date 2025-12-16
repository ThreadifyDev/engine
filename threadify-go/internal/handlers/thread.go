package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	threadService    *service.ThreadService
	stepEventService *service.StepEventService
	sessions         sync.Map
}

type Session struct {
	conn    *websocket.Conn
	ownerID string
	mu      sync.Mutex
}

func NewWebSocketHandler(threadService *service.ThreadService, stepEventService *service.StepEventService) *WebSocketHandler {
	return &WebSocketHandler{
		threadService:    threadService,
		stepEventService: stepEventService,
	}
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
			session.ownerID = req.OwnerID
			session.mu.Unlock()
			h.sessions.Store(req.OwnerID, session)
		}
		response = resp

	case "startThread":
		var req models.StartThreadRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleStartThread(&req, session.ownerID)

	case "recordThreadEvent":
		var req models.RecordEventRequest
		json.Unmarshal(msgBytes, &req)
		response = h.threadService.HandleRecordEvent(&req, session.ownerID)

	case "stepEvent":
		var req models.StepEvent
		json.Unmarshal(msgBytes, &req)
		response = h.handleStepEvent(&req, session.ownerID)

	case "closeConnection":
		resp := h.threadService.HandleClose(session.ownerID)
		// Immediately close the WebSocket connection after sending response
		go func() {
			time.Sleep(100 * time.Millisecond) // Small delay to ensure response is sent
			session.mu.Lock()
			defer session.mu.Unlock()
			session.conn.Close()
		}()
		response = resp

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
