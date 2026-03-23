package handlers

import (
	"net/http"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/service"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AgentHandler struct {
	agentSvc *service.AgentService
	logger   *zap.Logger
}

func NewAgentHandler(
	agentSvc *service.AgentService,
	logger *zap.Logger,
) *AgentHandler {
	return &AgentHandler{
		agentSvc: agentSvc,
		logger:   logger,
	}
}

type ChatRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id"`
	Skill          string `json:"skill"`
}

const (
	eventStreamContentType = "text/event-stream"
	cacheControlNoCache    = "no-cache"
	connectionKeepAlive    = "keep-alive"
)

func (h *AgentHandler) Chat(c *gin.Context) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var chatReq ChatRequest
	if err := c.ShouldBindJSON(&chatReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body format"})
		return
	}

	c.Writer.Header().Set("Content-Type", eventStreamContentType)
	c.Writer.Header().Set("Cache-Control", cacheControlNoCache)
	c.Writer.Header().Set("Connection", connectionKeepAlive)

	onEvent := func(eventType, data string) {
		c.SSEvent(eventType, data)
		c.Writer.Flush()
	}

	available, err := h.agentSvc.CheckCredits(c.Request.Context(), authHeader)
	if err != nil {
		h.logger.Error("credit check failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Credit verification failed"})
		return
	}
	if !available {
		h.logger.Warn("insufficient credits, blocking request", zap.String("userID", userID.(string)))
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "Insufficient credits"})
		return
	}

	err = h.agentSvc.ChatStream(
		c.Request.Context(),
		authHeader,
		userID.(string),
		companyID.(string),
		chatReq.ConversationID,
		chatReq.Message,
		chatReq.Skill,
		onEvent,
	)

	if err != nil {
		h.logger.Error("chat stream failed", zap.Error(err))
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
	}
}

func (h *AgentHandler) GetConversations(c *gin.Context) {
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	available, err := h.agentSvc.CheckCredits(c.Request.Context(), c.GetHeader(service.HeaderAuthorization))
	if err != nil {
		h.logger.Warn("failed to check credits for conversation list", zap.Error(err))
		available = true
	}

	if !available {
		c.JSON(http.StatusOK, gin.H{
			"conversations":     []models.AgentConversation{},
			"credits_available": false,
		})
		return
	}

	convs, err := h.agentSvc.GetConversations(userID.(string))
	if err != nil {
		h.logger.Error("failed to load conversations", zap.Error(err), zap.String("userID", userID.(string)))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversations":     convs,
		"credits_available": available,
	})
}

func (h *AgentHandler) GetConversation(c *gin.Context) {
	convID := c.Param("id")
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	msgs, err := h.agentSvc.GetMessagesForUser(userID.(string), convID)
	if err != nil {
		h.logger.Warn("failed to load messages", zap.Error(err), zap.String("conversationID", convID))
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

func (h *AgentHandler) DeleteConversation(c *gin.Context) {
	convID := c.Param("id")
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := h.agentSvc.DeleteConversation(convID, userID.(string))
	if err != nil {
		h.logger.Error("failed to delete conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

func (h *AgentHandler) ContinueConversation(c *gin.Context) {
	parentConvID := c.Param("id")
	userID, exists := c.Get(sharedauth.CtxUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	newID, title, parentID, err := h.agentSvc.ContinueConversation(
		c.Request.Context(),
		userID.(string),
		companyID.(string),
		parentConvID,
	)
	if err != nil {
		h.logger.Error("failed to continue conversation", zap.Error(err), zap.String("parentConversationID", parentConvID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation_id": newID,
		"title":           title,
		"parent_id":       parentID,
	})
}
