package handlers

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/metrics"
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

	ThreadStatusCancelled = string(domain.ThreadStatusCancelled)
	ThreadStatusCompleted = string(domain.ThreadStatusCompleted)

	defaultWebSocketReadLimitBytes = int64(2 * 1024 * 1024) // 2MB safety cap before auth/plan resolution
	readLimitOverheadBytes         = int64(64 * 1024)       // JSON envelope overhead allowance
)

var upgrader websocket.Upgrader

type WebSocketHandler struct {
	threadService        domain.ThreadService
	stepEventService     domain.StepEventProcessor
	invitationService    domain.InvitationTokenService
	notificationConsumer domain.NotificationConsumer
	notificationRouter   domain.NotificationRouter
	planService          domain.PlanService
	valkeyClient         domain.ValkeyClient
	sessions             sync.Map
	luaScriptManager     domain.LuaScriptManager
	rateLimitConfig      *config.RateLimitConfig
	websocketConfig      *config.WebSocketConfig
	logger               *zap.Logger
}

type WSSession struct {
	conn      domain.WSConnection
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
	AckToken       string `json:"ack_token"`
}

func dtoConnectResponseFromDomain(r *domain.ConnectResponse) *dto.ConnectResponse {
	if r == nil {
		return &dto.ConnectResponse{}
	}
	return &dto.ConnectResponse{
		Action:           r.Action,
		Status:           r.Status,
		Message:          r.Message,
		OwnerID:          r.OwnerID,
		CompanyID:        r.CompanyID,
		SubscribedEvents: r.SubscribedEvents,
	}
}

func dtoStartThreadResponseFromDomain(r *domain.StartThreadResponse) *dto.StartThreadResponse {
	if r == nil {
		return &dto.StartThreadResponse{}
	}
	return &dto.StartThreadResponse{
		Action:   r.Action,
		Status:   r.Status,
		Message:  r.Message,
		ThreadID: r.ThreadID,
	}
}

func dtoRecordEventResponseFromDomain(r *domain.RecordEventResponse) *dto.RecordEventResponse {
	if r == nil {
		return &dto.RecordEventResponse{}
	}
	return &dto.RecordEventResponse{
		Action:      r.Action,
		Status:      r.Status,
		Message:     r.Message,
		ThreadID:    r.ThreadID,
		StepID:      r.StepID,
		IsDuplicate: r.IsDuplicate,
	}
}

func dtoAddRefsResponseFromDomain(r *domain.AddRefsResponse) *dto.AddRefsResponse {
	if r == nil {
		return &dto.AddRefsResponse{}
	}
	return &dto.AddRefsResponse{
		Action:   r.Action,
		Status:   r.Status,
		Message:  r.Message,
		ThreadID: r.ThreadID,
	}
}

func dtoInvitePartyResponseFromDomain(r *domain.InvitePartyResponse) *dto.InvitePartyResponse {
	if r == nil {
		return &dto.InvitePartyResponse{}
	}
	return &dto.InvitePartyResponse{
		Action:      r.Action,
		Status:      r.Status,
		ThreadToken: r.ThreadToken,
		Role:        r.Role,
		AccessLevel: r.AccessLevel,
		ExpiresAt:   r.ExpiresAt,
		Message:     r.Message,
	}
}

func dtoJoinThreadResponseFromDomain(r *domain.JoinThreadResponse) *dto.JoinThreadResponse {
	if r == nil {
		return &dto.JoinThreadResponse{}
	}
	return &dto.JoinThreadResponse{
		Action:      r.Action,
		Status:      r.Status,
		ThreadID:    r.ThreadID,
		ContractID:  r.ContractID,
		Role:        r.Role,
		AccessLevel: r.AccessLevel,
		Message:     r.Message,
	}
}

func NewWebSocketHandler(
	threadService domain.ThreadService,
	stepEventService domain.StepEventProcessor,
	invitationService domain.InvitationTokenService,
	notificationConsumer domain.NotificationConsumer,
	notificationRouter domain.NotificationRouter,
	planService domain.PlanService,
	valkeyClient domain.ValkeyClient,
	luaScriptManager domain.LuaScriptManager,
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

func (s *WSSession) enforceCredits(planSvc domain.PlanService, action string) *dto.ErrorResponse {
	if action == ActionConnect || action == ActionCloseThread || action == ActionThreadEnd || action == ActionCloseConnection || s.companyID == "" {
		return nil
	}

	checkCtx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	_, err := planSvc.CheckBalancePositive(checkCtx, s.companyID)
	cancel()

	if err != nil {
		if errors.Is(err, service.ErrNoAccount) {
			return &dto.ErrorResponse{
				Action:  action,
				Status:  StatusError,
				Message: "Payment required: Set up a billing account to continue using the service.",
			}
		}
		if errors.Is(err, service.ErrInsufficientCredit) {
			return &dto.ErrorResponse{
				Action:  action,
				Status:  StatusError,
				Message: "Insufficient credits",
				Details: err.Error(),
			}
		}
		return &dto.ErrorResponse{
			Action:  action,
			Status:  StatusError,
			Message: "failed to verify credits",
			Details: err.Error(),
		}
	}
	return nil
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
			if sendErr := session.SendMessage(h.newErrorResponse("error", "Invalid JSON payload", err.Error())); sendErr != nil {
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

func (h *WebSocketHandler) newErrorResponse(action, message, details string) dto.ErrorResponse {
	return dto.ErrorResponse{
		Action:  action,
		Status:  StatusError,
		Message: message,
		Details: details,
	}
}

func (h *WebSocketHandler) handleMessage(action string, msg map[string]interface{}, msgBytes []byte, session *WSSession) interface{} {
	wsStart := perf.Now()
	sessionID := session.sessionID

	defer func() {
		duration := perf.Since(wsStart)
		metrics.RequestDuration.WithLabelValues(action).Observe(duration.Seconds())
	}()

	if errResp := session.enforceCredits(h.planService, action); errResp != nil {
		return *errResp
	}

	switch action {
	case ActionConnect:
		var req dto.ConnectRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			return h.newErrorResponse(ActionConnect, "Invalid request format", err.Error())
		}

		resp := h.threadService.HandleConnect(session.ctx, &domain.ConnectCmd{
			Action:           req.Action,
			ApiKey:           req.ApiKey,
			ServiceName:      req.ServiceName,
			SubscribedEvents: req.SubscribedEvents,
			MaxInFlight:      req.MaxInFlight,
		})

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
				h.logger.Error("connect: failed to verify credit account", zap.Error(meterErr))
			}

			if h.notificationRouter != nil {
				h.handleSubscribe(session, "global", req.SubscribedEvents)
				maxInFlight := req.MaxInFlight
				if maxInFlight < 1 || maxInFlight > h.websocketConfig.MaxInFlightMax {
					maxInFlight = h.websocketConfig.MaxInFlightDefault
				}
				if err := h.notificationRouter.HandleConnect(session.sessionID, resp.OwnerID, maxInFlight, session.conn, &session.sendMu); err != nil {
					perf.LogStructured("WebSocket NATS_SETUP_FAILED", zap.String("session", sessionID), zap.Error(err))
				}
			}
		}
		return dtoConnectResponseFromDomain(resp)

	case ActionStartThread:
		var req dto.StartThreadRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			return h.newErrorResponse(ActionStartThread, "Invalid request format", err.Error())
		}

		resp := h.threadService.HandleStartThread(session.ctx, &domain.StartThreadCmd{
			Action:       req.Action,
			Label:        req.Label,
			ContractName: req.ContractName,
			Role:         req.Role,
			Refs:         req.Refs,
		}, session.ownerID, session.companyID)
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
		return dtoStartThreadResponseFromDomain(resp)

	case ActionRecordThreadEvent:
		var req dto.RecordEventRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			return h.newErrorResponse(ActionRecordThreadEvent, "Invalid request format", err.Error())
		}

		subSteps := make([]domain.SubStepCmd, len(req.SubSteps))
		for i, s := range req.SubSteps {
			subSteps[i] = domain.SubStepCmd{
				Name:       s.Name,
				Status:     s.Status,
				Payload:    s.Payload,
				RecordedAt: s.RecordedAt,
			}
		}

		resp := h.threadService.HandleRecordEvent(session.ctx, &domain.RecordEventCmd{
			Action:            req.Action,
			ThreadID:          req.ThreadID,
			StepName:          req.StepName,
			Type:              req.Type,
			StartedAt:         req.StartedAt,
			FinishedAt:        req.FinishedAt,
			Context:           req.Context,
			Refs:              req.Refs,
			Status:            req.Status,
			ServiceName:       req.ServiceName,
			IdempotencyKey:    req.IdempotencyKey,
			ThreadifyMetadata: req.ThreadifyMetadata,
			SubSteps:          subSteps,
		}, session.ownerID, session.companyID)
		return dtoRecordEventResponseFromDomain(resp)

	case ActionAddRefs:
		var req dto.AddRefsRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			return h.newErrorResponse(ActionAddRefs, "Invalid request format", err.Error())
		}

		resp := h.threadService.HandleAddRefs(session.ctx, &domain.AddRefsCmd{
			Action:   req.Action,
			ThreadID: req.ThreadID,
			Refs:     req.Refs,
		}, session.ownerID)
		return dtoAddRefsResponseFromDomain(resp)

	case ActionCloseConnection:
		return &dto.CloseConnectionResponse{
			Action:  ActionCloseConnection,
			Status:  StatusSuccess,
			Message: "Connection will be closed",
		}

	case ActionInviteParty:
		var req dto.InvitePartyRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			return h.newErrorResponse(ActionInviteParty, "Invalid request format", err.Error())
		}

		return h.handleInviteParty(session, &domain.InvitePartyCmd{
			Action:      req.Action,
			Role:        req.Role,
			AccessLevel: req.AccessLevel,
			ExpiresIn:   req.ExpiresIn,
		})

	case ActionJoinThread:
		var req dto.JoinThreadRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			return h.newErrorResponse(ActionJoinThread, "Invalid request format", err.Error())
		}
		return h.handleJoinThread(session, &domain.JoinThreadCmd{
			Action:      req.Action,
			ThreadToken: req.ThreadToken,
			ThreadID:    req.ThreadID,
			Role:        req.Role,
		})

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
		return h.handleSubscribe(session, req.StepName, req.EventTypes)

	case ActionUnsubscribe:
		var req struct {
			StepName string `json:"stepName"`
		}
		json.Unmarshal(msgBytes, &req) //nolint:errcheck
		if req.StepName == "" {
			return h.newErrorResponse(ActionUnsubscribe, "Step name is required", "")
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
		return h.newErrorResponse(action, "Unknown action: "+action, "")
	}
}

func (h *WebSocketHandler) handleInviteParty(session *WSSession, req *domain.InvitePartyCmd) interface{} {
	if req.Action != ActionInviteParty {
		return h.newErrorResponse(ActionInviteParty, "Invalid action", "")
	}
	session.mu.Lock()
	threadIDs := make([]string, len(session.threadIDs))
	copy(threadIDs, session.threadIDs)
	session.mu.Unlock()

	resp, err := h.threadService.HandleInviteParty(session.ctx, req, session.ownerID, session.companyID, threadIDs)
	if err != nil {
		return h.newErrorResponse(ActionInviteParty, "Invite failed", err.Error())
	}
	return dtoInvitePartyResponseFromDomain(resp)
}

func (h *WebSocketHandler) handleJoinThread(session *WSSession, req *domain.JoinThreadCmd) interface{} {
	if req.Action != ActionJoinThread {
		return h.newErrorResponse(ActionJoinThread, "Invalid action", "")
	}
	resp, err := h.threadService.HandleJoinThread(session.ctx, req, session.ownerID, session.companyID)
	if err != nil {
		return h.newErrorResponse(ActionJoinThread, "Join failed", err.Error())
	}
	if resp.Status == StatusSuccess {
		session.mu.Lock()
		session.threadIDs = append(session.threadIDs, resp.ThreadID)
		session.mu.Unlock()
	}
	return dtoJoinThreadResponseFromDomain(resp)
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

func (h *WebSocketHandler) handleSubscribe(session *WSSession, stepNameRaw string, eventTypes []string) interface{} {
	if stepNameRaw == "" {
		stepNameRaw = "global"
	}
	if h.notificationRouter == nil {
		return h.newErrorResponse(ActionSubscribe, "Notification router not available", "")
	}

	var contractName, stepName string
	if strings.Contains(stepNameRaw, "@") {
		parts := strings.SplitN(stepNameRaw, "@", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return h.newErrorResponse(ActionSubscribe, "Invalid format. Use 'stepName' or 'contract@stepName'", "")
		}
		contractName, stepName = parts[0], parts[1]
	} else {
		stepName = stepNameRaw
	}

	if err := h.notificationRouter.HandleSubscribe(session.sessionID, stepName, contractName, eventTypes); err != nil {
		return h.newErrorResponse(ActionSubscribe, fmt.Sprintf("Failed to subscribe: %v", err), "")
	}
	return map[string]interface{}{"action": ActionSubscribe, "status": StatusSuccess, "message": fmt.Sprintf("Subscribed to %s", stepNameRaw)}
}

func (h *WebSocketHandler) handleNotificationAck(session *WSSession, ackMsg *NotificationACKMessage) interface{} {
	if h.notificationRouter == nil {
		return h.newErrorResponse(ActionAckNotification, "Notification router not available", "")
	}
	if ackMsg.AckToken != "" {
		if err := h.notificationRouter.HandleAck(ackMsg.AckToken); err != nil {
			return h.newErrorResponse(ActionAckNotification, err.Error(), "")
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
		return h.newErrorResponse(ActionThreadEnd, "Thread ID is required", "")
	}
	if status != "" && status != ThreadStatusCancelled && status != ThreadStatusCompleted {
		return h.newErrorResponse(ActionThreadEnd, "Status must be 'cancelled' or 'completed'", "")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	recordedAt := time.Now()
	if err := h.threadService.EndThread(ctx, threadID, session.ownerID, service.ActorServiceRuleEngine, status, reason, recordedAt); err != nil {
		return h.newErrorResponse(ActionThreadEnd, "Failed to end thread: "+err.Error(), "")
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
