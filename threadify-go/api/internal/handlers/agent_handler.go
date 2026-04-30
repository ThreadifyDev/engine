package handlers

import (
	"errors"
	"net/http"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/dto"
	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/service"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	agentSvc ports.AgentService
}

func NewAgentHandler(
	agentSvc ports.AgentService,
) *AgentHandler {
	return &AgentHandler{
		agentSvc: agentSvc,
	}
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

	var chatReq dto.ChatRequest
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

	err := h.agentSvc.ChatStreamEino(
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
		c.SSEvent(domain.EventError, err.Error())
		c.Writer.Flush()
	}
}

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

	dtos := make([]*dto.AgentConversation, len(convs))
	for i, conv := range convs {
		dtos[i] = mapConversationToDTO(&conv)
	}

	c.JSON(http.StatusOK, dto.ConversationsResponse{
		Conversations: dtos,
		MaxTokens:     h.agentSvc.GetMaxTokens(),
		MaxMessages:   h.agentSvc.GetMaxMessages(),
	})
}

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

	dtos := make([]*dto.AgentMessage, len(msgs))
	for i, m := range msgs {
		dtos[i] = mapMessageToDTO(m)
	}

	c.JSON(http.StatusOK, dto.MessagesResponse{Messages: dtos})
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

	c.JSON(http.StatusOK, dto.ChatResponse{
		ConversationID: newConvID,
		Title:          title,
		Summary:        summary,
	})
}

func mapConversationToDTO(c *domain.AgentConversation) *dto.AgentConversation {
	if c == nil {
		return nil
	}
	return &dto.AgentConversation{
		ID:           c.ID,
		UserID:       c.UserID,
		CompanyID:    c.CompanyID,
		Title:        c.Title,
		MessageCount: c.MessageCount,
		TokenCount:   c.TokenCount,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

func mapMessageToDTO(m *domain.AgentMessage) *dto.AgentMessage {
	if m == nil {
		return nil
	}
	return &dto.AgentMessage{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Role:           m.Role,
		Content:        m.Content,
		ToolCalls:      m.ToolCalls,
		ToolCallID:     m.ToolCallID,
		CreatedAt:      m.CreatedAt,
	}
}
