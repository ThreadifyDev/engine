package models

import "time"

type AgentConversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CompanyID string    `json:"company_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AgentMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCalls      *string   `json:"tool_calls"` // JSON string representation
	ToolCallID     *string   `json:"tool_call_id"`
	CreatedAt      time.Time `json:"created_at"`
}
