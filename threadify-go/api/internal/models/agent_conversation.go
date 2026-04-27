package models

import "time"

// StreamHandler is a callback function for streaming SSE events back to the client.
type StreamHandler func(eventType, data string)

const (
	// Skill Types
	SkillAuto    = "auto"
	SkillSupport = "support"
	SkillDesign  = "design"

	// SSE Event Types
	EventChunk        = "chunk"
	EventSystem       = "system"
	EventToolCall     = "tool_call"
	EventConversation = "conversation"
	EventDone         = "done"
	EventError        = "error"
	EventTokens       = "tokens"
	EventMessageCount = "message_count"

	// Context Keys
	ContextKeySummary = "conversation_summary"

	// Roles
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"

	// Tool Status
	ToolStatusSuccess = "Context saved successfully"
)

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

type AgentContext struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	ContextKey     string    `json:"context_key"`
	ContextValue   string    `json:"context_value"`
	CreatedAt      time.Time `json:"created_at"`
}
