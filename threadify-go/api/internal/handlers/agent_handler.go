package handlers

import (
	"errors"
	"net/http"

	iface "threadify-go/api/internal/interfaces"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/models"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	agentSvc iface.AgentService
}

func NewAgentHandler(
	agentSvc iface.AgentService,
) *AgentHandler {
	return &AgentHandler{
		agentSvc: agentSvc,
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

func ctxString(c *gin.Context, key string) (string, bool) {
	raw, exists := c.Get(key)
	if !exists {
		return "", false
	}
	val, ok := raw.(string)
	return val, ok && val != ""
}

func (h *AgentHandler) Chat(c *gin.Context) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	userID, ok := ctxString(c, sharedauth.CtxUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, ok := ctxString(c, sharedauth.CtxCompanyID)
	if !ok {
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

	err := h.agentSvc.ChatStream(
		c.Request.Context(),
		authHeader,
		userID,
		companyID,
		chatReq.ConversationID,
		chatReq.Message,
		chatReq.Skill,
		onEvent,
	)
	if err != nil {
		c.SSEvent(models.EventError, err.Error())
		c.Writer.Flush()
	}
}

// GetConversations lists recent conversations for a user
func (h *AgentHandler) GetConversations(c *gin.Context) {
	_, ok := ctxString(c, sharedauth.CtxUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	companyID, ok := ctxString(c, sharedauth.CtxCompanyID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	convs, err := h.agentSvc.GetConversations(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversations": convs,
	})
}

// GetConversation retrieves a specific conversation with its messages
func (h *AgentHandler) GetConversation(c *gin.Context) {
	convID := c.Param("id")
	_, ok := ctxString(c, sharedauth.CtxUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	companyID, ok := ctxString(c, sharedauth.CtxCompanyID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	msgs, err := h.agentSvc.GetMessagesForUser(c.Request.Context(), companyID, convID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this conversation"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load messages"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

func (h *AgentHandler) DeleteConversation(c *gin.Context) {
	convID := c.Param("id")
	_, ok := ctxString(c, sharedauth.CtxUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, ok := ctxString(c, sharedauth.CtxCompanyID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := h.agentSvc.DeleteConversation(c.Request.Context(), companyID, convID); err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to delete this conversation"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

func (h *AgentHandler) ContinueConversation(c *gin.Context) {
	parentConvID := c.Param("id")
	userID, ok := ctxString(c, sharedauth.CtxUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, ok := ctxString(c, sharedauth.CtxCompanyID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	newConvID, title, summary, err := h.agentSvc.ContinueConversation(
		c.Request.Context(),
		userID,
		companyID,
		parentConvID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation_id": newConvID,
		"title":           title,
		"summary":         summary,
	})
}
