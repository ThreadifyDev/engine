package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WebSocketHandler struct {
	threadService *service.ThreadService
	sessions      sync.Map
}

type Session struct {
	conn    *websocket.Conn
	ownerID string
	mu      sync.Mutex
}

func NewWebSocketHandler(threadService *service.ThreadService) *WebSocketHandler {
	return &WebSocketHandler{
		threadService: threadService,
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
	msgBytes, _ := json.Marshal(msg)

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
		return resp

	case "startThread":
		var req models.StartThreadRequest
		json.Unmarshal(msgBytes, &req)
		return h.threadService.HandleStartThread(&req, session.ownerID)

	case "recordThreadEvent":
		var req models.RecordEventRequest
		json.Unmarshal(msgBytes, &req)
		return h.threadService.HandleRecordEvent(&req, session.ownerID)

	case "closeConnection":
		return h.threadService.HandleClose(session.ownerID)

	default:
		return models.ErrorResponse{
			Action:  "error",
			Status:  "error",
			Message: "Unknown action: " + action,
		}
	}
}
