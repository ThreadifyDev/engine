package repository

import (
	"context"
	"fmt"
	"threadify-go/api/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type agentRepository struct {
	pool *pgxpool.Pool
}

func NewAgentRepository(pool *pgxpool.Pool) domain.AgentRepository {
	return &agentRepository{pool: pool}
}


func (r *agentRepository) CreateConversation(ctx context.Context, conv *domain.AgentConversation) error {
	query := `
		INSERT INTO agent_conversations (id, user_id, company_id, title, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`
	_, err := r.pool.Exec(ctx, query, conv.ID, conv.UserID, conv.CompanyID, conv.Title)
	return err
}

func (r *agentRepository) CreateConversationWithParent(ctx context.Context, conv *domain.AgentConversation, parentConvID string) error {
	query := `
		INSERT INTO agent_conversations (id, user_id, company_id, title, parent_conversation_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`
	_, err := r.pool.Exec(ctx, query, conv.ID, conv.UserID, conv.CompanyID, conv.Title, parentConvID)
	return err
}

func (r *agentRepository) GetConversations(ctx context.Context, companyID string) ([]domain.AgentConversation, error) {
	query := `SELECT id, user_id, company_id, title, message_count, token_count, created_at, updated_at 
	          FROM agent_conversations 
	          WHERE company_id = $1 
	          ORDER BY updated_at DESC 
	          LIMIT 50`

	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []domain.AgentConversation
	for rows.Next() {
		var conv domain.AgentConversation
		err := rows.Scan(&conv.ID, &conv.UserID, &conv.CompanyID, &conv.Title, &conv.MessageCount, &conv.TokenCount, &conv.CreatedAt, &conv.UpdatedAt)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}

	return conversations, nil
}

func (r *agentRepository) UpdateConversationStats(ctx context.Context, convID string, messageCount, tokenCount int) error {
	query := `UPDATE agent_conversations 
	          SET message_count = $1, token_count = $2, updated_at = NOW() 
	          WHERE id = $3`
	_, err := r.pool.Exec(ctx, query, messageCount, tokenCount, convID)
	return err
}

func (r *agentRepository) GetConversationStats(ctx context.Context, convID string) (messageCount, tokenCount int, err error) {
	query := `SELECT message_count, token_count FROM agent_conversations WHERE id = $1`
	err = r.pool.QueryRow(ctx, query, convID).Scan(&messageCount, &tokenCount)
	return
}

func (r *agentRepository) AddMessage(ctx context.Context, msg *domain.AgentMessage) error {
	query := `
		INSERT INTO agent_messages (id, conversation_id, role, content, tool_calls, tool_call_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	_, err := r.pool.Exec(ctx, query, msg.ID, msg.ConversationID, msg.Role, msg.Content, msg.ToolCalls, msg.ToolCallID)
	// Update conversation updated_at when message is added
	if err == nil {
		_, _ = r.pool.Exec(ctx, `UPDATE agent_conversations SET updated_at = NOW() WHERE id = $1`, msg.ConversationID)
	}
	return err
}

func (r *agentRepository) GetMessages(ctx context.Context, convID string) ([]*domain.AgentMessage, error) {
	query := `
		SELECT id, conversation_id, role, content, tool_calls, tool_call_id, created_at
		FROM agent_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*domain.AgentMessage
	for rows.Next() {
		var m domain.AgentMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, &m)
	}
	return msgs, nil
}

func (r *agentRepository) DeleteConversation(ctx context.Context, convID string, userID string) error {
	// First delete all messages
	_, err := r.pool.Exec(ctx, `DELETE FROM agent_messages WHERE conversation_id = $1`, convID)
	if err != nil {
		return err
	}

	// Then delete the conversation (only if owned by user)
	result, err := r.pool.Exec(ctx, `DELETE FROM agent_conversations WHERE id = $1 AND user_id = $2`, convID, userID)
	if err != nil {
		return err
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("conversation not found or not owned by user")
	}

	return nil
}

func (r *agentRepository) SaveContext(ctx context.Context, agentCtx *domain.AgentContext) error {
	query := `
		INSERT INTO agent_context (id, conversation_id, context_key, context_value, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (id) DO UPDATE SET context_value = $4
	`
	_, err := r.pool.Exec(ctx, query, agentCtx.ID, agentCtx.ConversationID, agentCtx.ContextKey, agentCtx.ContextValue)
	return err
}

func (r *agentRepository) GetContext(ctx context.Context, convID string) ([]*domain.AgentContext, error) {
	query := `
		SELECT id, conversation_id, context_key, context_value, created_at
		FROM agent_context
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contexts []*domain.AgentContext
	for rows.Next() {
		var c domain.AgentContext
		if err := rows.Scan(&c.ID, &c.ConversationID, &c.ContextKey, &c.ContextValue, &c.CreatedAt); err != nil {
			return nil, err
		}
		contexts = append(contexts, &c)
	}
	return contexts, nil
}
