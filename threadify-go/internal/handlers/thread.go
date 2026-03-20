package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/perf"
	"github.com/threadify/engine/internal/service"
)

const (
	ActionConnect           = "connect"
	ActionStartThread       = "startThread"
	ActionRecordThreadEvent = "recordThreadEvent"
	ActionAddRefs           = "addRefs"
	ActionCloseConnection   = "closeConnection"
	ActionInviteParty       = "inviteParty"
	ActionJoinThread        = "joinThread"
	ActionAckNotification   = "ack_notification"
	ActionSubscribe         = "subscribe"
	ActionUnsubscribe       = "unsubscribe"
	ActionCloseThread       = "closeThread"
	ActionThreadEnd         = "threadEnd"
)

const (
	StatusSuccess = "success"
	StatusError   = "error"

	ThreadStatusCancelled = string(models.ThreadStatusCancelled)
	ThreadStatusCompleted = string(models.ThreadStatusCompleted)

	defaultWebSocketReadLimitBytes = int64(2 * 1024 * 1024) // 2MB safety cap before auth/plan resolution
	readLimitOverheadBytes         = int64(64 * 1024)       // JSON envelope overhead allowance
)

var upgrader websocket.Upgrader

type WebSocketHandler struct {
	threadService        *service.ThreadService
	stepEventService     *service.StepEventService
	invitationService    *service.InvitationTokenService
	notificationConsumer *service.NotificationConsumer
	notificationRouter   *NotificationRouter
	planService          interfaces.PlanService
	valkeyClient         interfaces.ValkeyClient
	sessions             sync.Map
	luaScriptManager     interfaces.LuaScriptManager
	rateLimitConfig      *config.RateLimitConfig
	websocketConfig      *config.WebSocketConfig
	logger               *zap.Logger
}

type WSSession struct {
	conn      *websocket.Conn
	sessionID string
	ownerID   string
	companyID string
	threadIDs []string
	ctx       context.Context
	mu        sync.Mutex
	sendMu    sync.Mutex
}

type NotificationACKMessage struct {
	Action         string `json:"action"`
	NotificationID string `json:"notification_id"`
	ThreadID       string `json:"thread_id"`
	Processed      bool   `json:"processed"`
	AckToken       string `json:"ackToken"`
}

func NewWebSocketHandler(
	threadService *service.ThreadService,
	stepEventService *service.StepEventService,
	invitationService *service.InvitationTokenService,
	notificationConsumer *service.NotificationConsumer,
	notificationRouter *NotificationRouter,
	planService interfaces.PlanService,
	valkeyClient interfaces.ValkeyClient,
	luaScriptManager interfaces.LuaScriptManager,
	rateLimitConfig *config.RateLimitConfig,
	websocketConfig *config.WebSocketConfig,
	logger *zap.Logger,
) *WebSocketHandler {
	upgrader = websocket.Upgrader{
		CheckOrigin:       func(r *http.Request) bool { return true },
		HandshakeTimeout:  time.Duration(websocketConfig.HandshakeTimeoutSeconds) * time.Second,
		ReadBufferSize:    websocketConfig.ReadBufferSize,
		WriteBufferSize:   websocketConfig.WriteBufferSize,
		EnableCompression: true,
	}

	return &WebSocketHandler{
		threadService:        threadService,
		stepEventService:     stepEventService,
		invitationService:    invitationService,
		notificationConsumer: notificationConsumer,
		notificationRouter:   notificationRouter,
		planService:          planService,
		valkeyClient:         valkeyClient,
		luaScriptManager:     luaScriptManager,
		rateLimitConfig:      rateLimitConfig,
		websocketConfig:      websocketConfig,
		logger:               logger,
	}
}

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("websocket upgrade error", zap.Error(err))
		return
	}
	defer conn.Close()
	conn.SetReadLimit(defaultWebSocketReadLimitBytes)

	session := &WSSession{
		conn:      conn,
		sessionID: uuid.New().String(),
		ctx:       c.Request.Context(),
	}

	for {
		messageType, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage {
			continue
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			if sendErr := session.SendMessage(models.ErrorResponse{
				Action:  StatusError,
				Status:  StatusError,
				Message: "Invalid JSON payload",
			}); sendErr != nil {
				break
			}
			continue
		}

		action, _ := msg["action"].(string)
		if err := session.SendMessage(h.handleMessage(action, msg, msgBytes, session)); err != nil {
			break
		}
		if action == ActionCloseConnection {
			break
		}
	}

	if session.ownerID != "" {
		h.sessions.Delete(session.ownerID)
		h.threadService.HandleClose(session.ownerID)
		h.unsubscribeFromNotifications(session)

		if h.notificationRouter != nil {
			if err := h.notificationRouter.HandleDisconnect(session.sessionID); err != nil {
				h.logger.Warn("failed to disconnect notification session",
					zap.String("session_id", session.sessionID), zap.Error(err))
			}
		}
	}
}

func (h *WebSocketHandler) handleMessage(action string, msg map[string]interface{}, msgBytes []byte, session *WSSession) interface{} {
	wsStart := perf.Now()
	sessionID := session.sessionID

	defer func() {
		duration := perf.Since(wsStart)
		metrics.RequestDuration.WithLabelValues(action).Observe(duration.Seconds())
	}()

	if action != ActionConnect && session.companyID != "" {
		checkCtx, cancel := context.WithTimeout(session.ctx, 2*time.Second)
		account, err := h.planService.GetCurrentLimits(checkCtx, session.companyID)
		cancel()
		if err != nil || account == nil {
			return models.ErrorResponse{
				Action:  action,
				Status:  StatusError,
				Message: "Active subscription required. Please check your billing status.",
			}
		}
	}

	switch action {
	case ActionConnect:
		var req models.ConnectRequest
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		resp := h.threadService.HandleConnect(session.ctx, &req)

		if resp.Status == StatusSuccess {
			session.mu.Lock()
			session.ownerID = resp.OwnerID
			session.companyID = resp.CompanyID
			session.mu.Unlock()
			h.sessions.Store(resp.OwnerID, session)

			checkCtx, cancel := context.WithTimeout(session.ctx, 2*time.Second)
			_, meterErr := h.planService.GetCurrentLimits(checkCtx, resp.CompanyID)
			cancel()
			if meterErr != nil {
				h.logger.Warn("connect: failed to verify subscription after auth", zap.Error(meterErr))
			}

			if h.notificationRouter != nil {
				maxInFlight := req.MaxInFlight
				if maxInFlight < 1 || maxInFlight > h.websocketConfig.MaxInFlightMax {
					maxInFlight = h.websocketConfig.MaxInFlightDefault
				}
				if err := h.notificationRouter.HandleConnect(session.sessionID, resp.OwnerID, maxInFlight, session.conn, &session.sendMu); err != nil {
					perf.LogStructured("WebSocket NATS_SETUP_FAILED", zap.String("session", sessionID), zap.Error(err))
				}
			}
		}
		return resp

	case ActionStartThread:
		var req models.StartThreadRequest
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		resp := h.threadService.HandleStartThread(session.ctx, &req, session.ownerID, session.companyID)
		if resp.Status == StatusSuccess {
			session.mu.Lock()
			if !slices.Contains(session.threadIDs, resp.ThreadID) {
				session.threadIDs = append(session.threadIDs, resp.ThreadID)
			}
			session.mu.Unlock()
			metrics.ThreadsCreated.Inc()
			metrics.RequestsTotal.WithLabelValues(ActionStartThread, StatusSuccess).Inc()
		} else {
			metrics.RequestsTotal.WithLabelValues(ActionStartThread, StatusError).Inc()
		}
		return resp

	case ActionRecordThreadEvent:
		var req models.RecordEventRequest
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		return h.threadService.HandleRecordEvent(session.ctx, &req, session.ownerID, session.companyID)

	case ActionAddRefs:
		var req models.AddRefsRequest
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		return h.threadService.HandleAddRefs(session.ctx, &req, session.ownerID)

	case ActionCloseConnection:
		return &models.CloseConnectionResponse{
			Action:  ActionCloseConnection,
			Status:  StatusSuccess,
			Message: "Connection will be closed",
		}

	case ActionInviteParty:
		var req models.InvitePartyRequest
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		return h.handleInviteParty(session, &req)

	case ActionJoinThread:
		var req models.JoinThreadRequest
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		return h.handleJoinThread(session, &req)

	case ActionAckNotification:
		var ackMsg NotificationACKMessage
		json.Unmarshal(msgBytes, &ackMsg) //nolint:errcheck
		return h.handleNotificationAck(session, &ackMsg)

	case ActionSubscribe:
		var req struct {
			StepName   string   `json:"stepName"`
			EventTypes []string `json:"eventTypes"`
		}
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		return h.handleSubscribe(session, req.StepName)

	case ActionUnsubscribe:
		var req struct {
			StepName string `json:"stepName"`
		}
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		if req.StepName == "" {
			return models.ErrorResponse{Action: ActionUnsubscribe, Status: StatusError, Message: "Step name is required"}
		}
		return map[string]interface{}{"action": ActionUnsubscribe, "status": StatusSuccess, "message": fmt.Sprintf("Unsubscribed from %s", req.StepName)}

	case ActionCloseThread, ActionThreadEnd:
		var req struct {
			ThreadID string `json:"threadId"`
			Status   string `json:"status"`
			Reason   string `json:"reason,omitempty"`
		}
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		return h.handleThreadEnd(session, req.ThreadID, req.Status, req.Reason)

	default:
		return models.ErrorResponse{Action: action, Status: StatusError, Message: "Unknown action: " + action}
	}
}

func (h *WebSocketHandler) handleInviteParty(session *WSSession, req *models.InvitePartyRequest) interface{} {
	if req.Action != ActionInviteParty {
		return models.ErrorResponse{Action: ActionInviteParty, Status: StatusError, Message: "Invalid action"}
	}
	session.mu.Lock()
	threadIDs := make([]string, len(session.threadIDs))
	copy(threadIDs, session.threadIDs)
	session.mu.Unlock()

	resp, err := h.threadService.HandleInviteParty(req, session.ownerID, session.companyID, threadIDs)
	if err != nil {
		return models.ErrorResponse{Action: ActionInviteParty, Status: StatusError, Message: err.Error()}
	}
	return resp
}

func (h *WebSocketHandler) handleJoinThread(session *WSSession, req *models.JoinThreadRequest) interface{} {
	if req.Action != ActionJoinThread {
		return models.ErrorResponse{Action: ActionJoinThread, Status: StatusError, Message: "Invalid action"}
	}
	resp, err := h.threadService.HandleJoinThread(req, session.ownerID, session.companyID)
	if err != nil {
		return models.ErrorResponse{Action: ActionJoinThread, Status: StatusError, Message: err.Error()}
	}
	if resp.Status == StatusSuccess {
		session.mu.Lock()
		session.threadIDs = append(session.threadIDs, resp.ThreadID)
		session.mu.Unlock()
	}
	return resp
}

func (h *WebSocketHandler) unsubscribeFromNotifications(session *WSSession) {
	if h.notificationConsumer == nil {
		return
	}
	session.mu.Lock()
	threadIDs := session.threadIDs
	ownerID := session.ownerID
	session.mu.Unlock()

	for _, threadID := range threadIDs {
		if err := h.notificationConsumer.Unsubscribe(threadID, ownerID); err != nil {
			h.logger.Warn("failed to unsubscribe from thread notifications",
				zap.String("thread_id", threadID), zap.String("owner_id", ownerID), zap.Error(err))
		}
	}
}

func (h *WebSocketHandler) handleSubscribe(session *WSSession, stepNameRaw string) interface{} {
	if stepNameRaw == "" {
		return models.ErrorResponse{Action: ActionSubscribe, Status: StatusError, Message: "Step name is required"}
	}
	if h.notificationRouter == nil {
		return models.ErrorResponse{Action: ActionSubscribe, Status: StatusError, Message: "Notification router not available"}
	}

	var contractName, stepName string
	if strings.Contains(stepNameRaw, "@") {
		parts := strings.SplitN(stepNameRaw, "@", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return models.ErrorResponse{Action: ActionSubscribe, Status: StatusError, Message: "Invalid format. Use 'stepName' or 'contract@stepName'"}
		}
		contractName, stepName = parts[0], parts[1]
	} else {
		stepName = stepNameRaw
	}

	if err := h.notificationRouter.HandleSubscribe(session.sessionID, stepName, contractName); err != nil {
		return models.ErrorResponse{Action: ActionSubscribe, Status: StatusError, Message: fmt.Sprintf("Failed to subscribe: %v", err)}
	}
	return map[string]interface{}{"action": ActionSubscribe, "status": StatusSuccess, "message": fmt.Sprintf("Subscribed to %s", stepNameRaw)}
}

func (h *WebSocketHandler) handleNotificationAck(session *WSSession, ackMsg *NotificationACKMessage) interface{} {
	if h.notificationRouter == nil {
		return models.ErrorResponse{Action: ActionAckNotification, Status: StatusError, Message: "Notification router not available"}
	}
	if ackMsg.AckToken != "" {
		if err := h.notificationRouter.HandleAck(ackMsg.AckToken); err != nil {
			return models.ErrorResponse{Action: ActionAckNotification, Status: StatusError, Message: err.Error()}
		}
	}
	return map[string]interface{}{
		"action":          ActionAckNotification,
		"status":          StatusSuccess,
		"notification_id": ackMsg.NotificationID,
	}
}

func (h *WebSocketHandler) handleThreadEnd(session *WSSession, threadID, status, reason string) interface{} {
	if threadID == "" {
		return models.ErrorResponse{Action: ActionThreadEnd, Status: StatusError, Message: "Thread ID is required"}
	}
	if status != "" && status != ThreadStatusCancelled && status != ThreadStatusCompleted {
		return models.ErrorResponse{Action: ActionThreadEnd, Status: StatusError, Message: "Status must be 'cancelled' or 'completed'"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	recordedAt := time.Now()
	if err := h.threadService.EndThread(ctx, threadID, session.ownerID, "", status, reason, recordedAt); err != nil {
		return models.ErrorResponse{Action: ActionThreadEnd, Status: StatusError, Message: "Failed to end thread: " + err.Error()}
	}

	finalStatus := status
	if finalStatus == "" {
		finalStatus = ThreadStatusCancelled
	}
	timestampField := "cancelledAt"
	if finalStatus == ThreadStatusCompleted {
		timestampField = "completedAt"
	}

	return map[string]interface{}{
		"action":       ActionThreadEnd,
		"status":       StatusSuccess,
		"threadId":     threadID,
		"threadStatus": finalStatus,
		timestampField: recordedAt.Format(time.RFC3339),
		"message":      "Thread " + finalStatus + " successfully",
	}
}

func (s *WSSession) SendMessage(message interface{}) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.conn.WriteJSON(message)
}
