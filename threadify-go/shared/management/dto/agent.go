package dto

import "time"

type AgentConversation struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	CompanyID    string    `json:"company_id"`
	Title        string    `json:"title"`
	MessageCount int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AgentMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCalls      *string   `json:"tool_calls,omitempty"`
	ToolCallID     *string   `json:"tool_call_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ChatRequest struct {
	Message        string `json:"message" binding:"required"`
	ConversationID string `json:"conversation_id"`
	Skill          string `json:"skill"`
}

type ChatResponse struct {
	ConversationID string `json:"conversation_id"`
	Title          string `json:"title"`
	Summary        string `json:"summary"`
}

type ConversationsResponse struct {
	Conversations []*AgentConversation `json:"conversations"`
	MaxTokens     int                  `json:"max_tokens"`
	MaxMessages   int                  `json:"max_messages"`
}

type MessagesResponse struct {
	Messages []*AgentMessage `json:"messages"`
}
