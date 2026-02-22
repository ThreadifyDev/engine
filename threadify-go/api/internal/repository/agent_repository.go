package repository

import (
	"database/sql"
	"fmt"
	"threadify-go/api/internal/models"
)

type AgentRepository struct {
	db *sql.DB
}

func NewAgentRepository(db *sql.DB) *AgentRepository {
	return &AgentRepository{db: db}
}

func (r *AgentRepository) CreateConversation(conv *models.AgentConversation) error {
	query := `
		INSERT INTO agent_conversations (id, user_id, company_id, title, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`
	_, err := r.db.Exec(query, conv.ID, conv.UserID, conv.CompanyID, conv.Title)
	return err
}

func (r *AgentRepository) GetConversations(userID string) ([]*models.AgentConversation, error) {
	query := `
		SELECT id, user_id, company_id, title, created_at, updated_at
		FROM agent_conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []*models.AgentConversation
	for rows.Next() {
		var c models.AgentConversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.CompanyID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		convs = append(convs, &c)
	}
	return convs, nil
}

func (r *AgentRepository) AddMessage(msg *models.AgentMessage) error {
	query := `
		INSERT INTO agent_messages (id, conversation_id, role, content, tool_calls, tool_call_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	_, err := r.db.Exec(query, msg.ID, msg.ConversationID, msg.Role, msg.Content, msg.ToolCalls, msg.ToolCallID)
	// Update conversation updated_at when message is added
	if err == nil {
		_, _ = r.db.Exec(`UPDATE agent_conversations SET updated_at = NOW() WHERE id = $1`, msg.ConversationID)
	}
	return err
}

func (r *AgentRepository) GetMessages(convID string) ([]*models.AgentMessage, error) {
	query := `
		SELECT id, conversation_id, role, content, tool_calls, tool_call_id, created_at
		FROM agent_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(query, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*models.AgentMessage
	for rows.Next() {
		var m models.AgentMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, &m)
	}
	return msgs, nil
}

func (r *AgentRepository) DeleteConversation(convID string, userID string) error {
	// First delete all messages
	_, err := r.db.Exec(`DELETE FROM agent_messages WHERE conversation_id = $1`, convID)
	if err != nil {
		return err
	}

	// Then delete the conversation (only if owned by user)
	result, err := r.db.Exec(`DELETE FROM agent_conversations WHERE id = $1 AND user_id = $2`, convID, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("conversation not found or not owned by user")
	}

	return nil
}
