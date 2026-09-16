package domain

import "time"

type StreamHandler func(eventType, data string)

const (
	SkillAuto    = "auto"
	SkillSupport = "support"
	SkillDesign  = "design"

	EventChunk        = "chunk"
	EventSystem       = "system"
	EventToolCall     = "tool_call"
	EventConversation = "conversation"
	EventDone         = "done"
	EventError        = "error"
	EventTokens       = "tokens"
	EventMessageCount = "message_count"

	ContextKeySummary = "conversation_summary"

	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"

	ToolStatusSuccess = "Context saved successfully"
)

type AgentConversation struct {
	ID           string
	UserID       string
	CompanyID    string
	Title        string
	MessageCount int
	TokenCount   int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type AgentMessage struct {
	ID             string
	ConversationID string
	Role           string
	Content        string
	ToolCalls      *string
	ToolCallID     *string
	CreatedAt      time.Time
}

type AgentContext struct {
	ID             string
	ConversationID string
	ContextKey     string
	ContextValue   string
	CreatedAt      time.Time
}
