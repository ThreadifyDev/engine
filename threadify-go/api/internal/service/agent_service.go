package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

const (
	// AI Models
	ModelGPT4oMini = "gpt-4o-mini"

	// Tool Names
	ToolExecuteGraphQL = "execute_graphql"
	ToolSaveContext    = "save_context"

	// Skill Types
	SkillSupport    = "support"
	SkillOperations = "operations"
	SkillBusiness   = "business"

	// SSE Event Types
	EventChunk        = "chunk"
	EventSystem       = "system"
	EventToolCall     = "tool_call"
	EventConversation = "conversation"
	EventDone         = "done"
	EventError        = "error"

	// Context Keys
	ContextKeySummary = "conversation_summary"

	// Roles
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"

	// Tool Status
	ToolStatusSuccess = "Context saved successfully"

	// GraphQL Operations
	queryCheckCredits = `query CheckCredits($meter: String, $amount: Int) {
		checkCredits(meter: $meter, amount: $amount)
	}`
	mutationRecordUsage = `mutation RecordUsage($tokens: Int!) {
		recordLLMUsage(tokens: $tokens)
	}`
)

const agentSystemPrompt = `WORKFLOW:
1. User asks about threads → call execute_graphql tool
2. Tool returns JSON data → analyze it and respond with a summary
3. User asks follow-up → use the data already in context (don't re-query)

RESPONSE RULES:
- After getting tool data, ALWAYS provide a summary (never say you can't fetch data)
- Be concise - 2-3 sentences max
- Only query fields you need
- Save important findings with save_context tool

GraphQL Schema (EXACT - follow this precisely):

type Query {
  thread(id: ID!): Thread
  threads(
    actor: String
    contractName: String
    contractVersion: Int
    status: String
    startedAfter: String
    startedBefore: String
    limit: Int
    offset: Int
  ): ThreadConnection!
  threadsByRef(
    refKey: String
    refValue: String!
    status: String
    limit: Int
  ): ThreadConnection!
}

QUERY SELECTION RULES (CRITICAL - Follow this decision tree):

1. Use thread(id:) ONLY when user explicitly asks about a thread by UUID:
   - Must have thread-specific keywords: "thread", "analyze thread", "show thread", "find thread"
   - AND the ID matches UUID pattern: 8-4-4-4-12 hex characters
   - Example: "Find thread 5d724fa5-1234-5678-9abc-def012345678" → thread(id: "5d724fa5-1234-5678-9abc-def012345678")
   - Example: "Analyze thread abc-123-def" → thread(id: "abc-123-def")
   - IMPORTANT: If thread(id:) returns null, RETRY with threadsByRef(refValue: "<uuid>") as fallback

2. Use threadsByRef() for ALL business identifiers (including UUID-format external IDs):
   - External system IDs: Stripe (pi_*, cus_*, ch_*), PayPal, AWS, etc.
   - Business IDs: customer_id, order_id, payment_id, transaction_id, invoice_id, etc.
   - Pure UUIDs WITHOUT "thread" keyword → assume it's a business ref, not thread ID
   - If you know the ref key name: threadsByRef(refKey: "customer_id", refValue: "cus_123")
   - If you DON'T know the ref key: threadsByRef(refValue: "cus_123") - searches across ALL ref keys
   - Example: "Find customer cus_123" → threadsByRef(refValue: "cus_123")
   - Example: "Show order ORD-456" → threadsByRef(refValue: "ORD-456")
   - Example: "Find payment pi_ABC123" → threadsByRef(refValue: "pi_ABC123")
   - Example: "Show me 5d724fa5-1234-..." (no "thread" keyword) → threadsByRef(refValue: "5d724fa5-1234-...")

3. Use threads(contractName:) when searching by contract/workflow type:
   - Keywords: "contract", "workflow", "process type"
   - Example: "Show order_processing threads" → threads(contractName: "order_processing")
   - Can combine with status: threads(contractName: "checkout", status: "failed")

4. Use threads(actor:) ONLY when user explicitly mentions who executed:
   - Service names: "ran by payment-service", "where merchant-service was involved"
   - User names: "executed by john@example.com"
   - Example: "Threads run by payment-service" → threads(actor: "payment-service")

DECISION PRIORITY (keyword-based, not pattern-based):
"thread" + UUID → thread(id:) with fallback to threadsByRef()
Business context (customer, order, payment, etc.) → threadsByRef()
Contract/workflow keywords → threads(contractName:)
Actor/service keywords → threads(actor:)

IMPORTANT: When in doubt, use threadsByRef() - it's safer and searches across all refs!

FALLBACK STRATEGY (CRITICAL - Always apply):
If your first query returns NO RESULTS (null, empty array, or totalCount: 0), ALWAYS try an alternative:
1. If thread(id:) returns null → RETRY with threadsByRef(refValue: "<same_id>")
2. If threadsByRef(refKey: "X", refValue: "Y") returns empty → RETRY with threadsByRef(refValue: "Y") (omit refKey to search all refs)
3. If threadsByRef(refValue: "X") returns empty AND value looks like UUID → RETRY with thread(id: "X")
4. If threads(contractName: "X") returns empty → Try threads() without filters (general search)

NEVER tell the user "I couldn't find it" without trying at least ONE fallback query!

type Thread {
  id: ID!
  contractName: String
  contractVersion: Int
  status: String!
  startedAt: String
  completedAt: String
  refs: String  # JSON string of reference key-value pairs
  steps(stepName: String, idempotencyKey: String, status: String): [StepStateInfo!]!
}

type ThreadConnection {
  threads: [Thread!]!
  totalCount: Int!
}


type StepStateInfo {
  stepName: String!
  idempotencyKey: String!
  status: String!
  retryCount: Int
  latestStepID: String
  firstSeenAt: String
  lastUpdatedAt: String
  actor: String
  actorService: String
  history: [StepHistory!]!
}

type StepHistory {
  attempt: Int!
  status: String!
  context: String!
  error: String
  timestamp: String!
}

THREADIFY BUSINESS CONTEXT:
Threadify turns customer requests into live execution graphs. Every customer request is a Thread flowing through your system.

Core Concepts:
1. **Thread** - One customer request/workflow (e.g., order processing, payment flow)
   - Has unique ID, status (running/completed/failed), contract, and steps
   - Can be linked to other threads (parent-child relationships)
   - Can have external refs (e.g., stripe_payment_id, order_id)

2. **Step** - One action in the workflow (e.g., validate_cart, charge_payment)
   - Has stepName, status, context, actor, and execution history
   - Idempotency prevents duplicate execution (stepName + idempotencyKey)
   - Can have sub-steps for granular operations

3. **Contract** - YAML validation rules enforced at runtime
   - Defines valid state transitions (e.g., validate_cart → check_inventory)
   - Prevents race conditions and invalid flows
   - Has entry_points and terminal_steps

4. **Step Statuses:**
   - success: Step completed as expected (e.g., Payment processed)
   - failed: Business logic failure (e.g., Payment declined)
   - error: System/technical error (e.g., Payment gateway timeout)

5. **Context** - Flat key-value pairs for business data (e.g., customer_id, amount, payment_method)
   - Stored in step history for audit trail
   - Used for querying and analysis

6. **Actors** - Users or services that execute steps (tracked for audit/compliance)

QUERY FORMAT (CRITICAL):
All GraphQL queries MUST be wrapped in "query { }" syntax:
CORRECT: query { thread(id: "abc") { status } }
WRONG: thread(id: "abc") { status }

EXAMPLES:

Q: "Find thread 5d724fa5-1234-5678-9abc-def012345678" (has "thread" keyword)
1. Call: execute_graphql(query: 'query { thread(id: "5d724fa5-1234-5678-9abc-def012345678") { status steps { stepName status } } }')
2. If result is null: FALLBACK → execute_graphql(query: 'query { threadsByRef(refValue: "5d724fa5-1234-5678-9abc-def012345678") { threads { id status } } }')
3. Respond: "Thread completed successfully with 15 steps across 3 phases: order_placed, payment_processed, shipment_dispatched."

Q: "Analyze thread abc-123-def" (has "thread" keyword)
1. Call: execute_graphql(query: 'query { thread(id: "abc-123-def") { status contractName steps { stepName status } } }')
2. If null: FALLBACK → execute_graphql(query: 'query { threadsByRef(refValue: "abc-123-def") { threads { id status } } }')
3. Respond: "Thread is active, running order_processing contract with 8 completed steps."

Q: "Show me 5d724fa5-1234-5678-9abc-def012345678" (NO "thread" keyword - could be external ID)
1. Call: execute_graphql(query: 'query { threadsByRef(refValue: "5d724fa5-1234-5678-9abc-def012345678") { threads { id status } } }')
2. If empty: FALLBACK → execute_graphql(query: 'query { thread(id: "5d724fa5-1234-5678-9abc-def012345678") { status } }')
3. Respond: "Found 1 thread with ref 5d724fa5-1234-5678-9abc-def012345678."

Q: "Find customer cus_ABC123" (business context)
1. Call: execute_graphql(query: 'query { threadsByRef(refKey: "customer_id", refValue: "cus_ABC123", limit: 10) { threads { id status } } }')
2. If empty: FALLBACK → execute_graphql(query: 'query { threadsByRef(refValue: "cus_ABC123", limit: 10) { threads { id status } } }')
3. Respond: "Found 2 threads for customer cus_ABC123: one completed, one in progress."

Q: "Show order ORD-456 status"
1. Call: execute_graphql(query: 'query { threadsByRef(refValue: "ORD-456", limit: 5) { threads { id contractName status startedAt completedAt steps { stepName status } } totalCount } }')
2. Respond: "Order ORD-456 is completed with 5 steps: validate_cart, check_inventory, charge_payment, generate_label, send_confirmation."

Q: "What threads handled customer@email.com request?"
1. Call: execute_graphql(query: 'query { threadsByRef(refValue: "customer@email.com", limit: 10) { threads { id contractName status startedAt completedAt } totalCount } }')
2. Respond: "Found 3 threads for customer@email.com: 2 completed (order_fulfillment, payment_processing), 1 active (shipping_notification)."

Q: "Show order_processing threads"
1. Call: execute_graphql(query: 'query { threads(contractName: "order_processing", limit: 10) { threads { id status } } }')
2. Respond: "Found 12 order_processing threads: 10 completed, 2 in progress."

Q: "Threads run by payment-service"
1. Call: execute_graphql(query: 'query { threads(actor: "payment-service", limit: 10) { threads { id contractName } } }')
2. Respond: "Found 8 threads executed by payment-service in the last hour."

Q: "Show failed threads"
1. Call: execute_graphql(query: 'query { threads(status: "failed", limit: 10) { threads { id contractName } } }')
2. Respond: "Found 3 failed threads: order-123, payment-456, shipping-789."

IMPORTANT:
- ALWAYS wrap queries in "query { }"
- Tool results contain the data you need - analyze them
- Never output raw JSON - always summarize
- Use save_context to remember important findings`

// StreamHandler is a callback function for streaming SSE events back to the client.
type StreamHandler func(eventType, data string)

type AgentService struct {
	threadifyEngineURL string
	httpClient         *http.Client
	openaiClient       *openai.Client
	agentRepo          repository.AgentRepository
	maxMessages        int
	maxTokens          int
	summaryMaxTokens   int
	logger             *zap.Logger
}

func NewAgentService(
	threadifyEngineURL string,
	openaiAPIKey string,
	agentRepo repository.AgentRepository,
	maxMessages int,
	maxTokens int,
	summaryMaxTokens int,
	logger *zap.Logger,
) *AgentService {
	return &AgentService{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		openaiClient:       openai.NewClient(openaiAPIKey),
		agentRepo:          agentRepo,
		maxMessages:        maxMessages,
		maxTokens:          maxTokens,
		summaryMaxTokens:   summaryMaxTokens,
		logger:             logger,
	}
}

func (s *AgentService) ChatStream(
	ctx context.Context,
	authHeader string,
	userID string,
	companyID string,
	conversationID string,
	message string,
	skill string,
	onEvent StreamHandler,
) error {
	var err error
	if conversationID != "" {
		if err := s.ensureConversationOwnership(ctx, companyID, conversationID); err != nil {
			return err
		}
		msgCount, tokenCount, err := s.agentRepo.GetConversationStats(ctx, conversationID)
		if err == nil {
			if msgCount >= s.maxMessages {
				return fmt.Errorf("conversation has reached the maximum of %d messages", s.maxMessages)
			}
			if tokenCount >= s.maxTokens {
				return fmt.Errorf("conversation has reached the token limit of %d", s.maxTokens)
			}
		}
	}

	convID := conversationID
	isNewConversation := false

	if convID == "" {
		convID = uuid.New().String()
		isNewConversation = true
		title := message
		if len(title) > 30 {
			title = title[:30] + "..."
		}
		err = s.agentRepo.CreateConversation(ctx, &models.AgentConversation{
			ID:        convID,
			UserID:    userID,
			CompanyID: companyID,
			Title:     title,
		})
		if err != nil {
			s.logger.Error("failed to create conversation", zap.Error(err))
		}
	}

	err = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           RoleUser,
		Content:        message,
	})
	if err != nil {
		s.logger.Error("failed to save user message", zap.Error(err))
	}

	messages := s.buildInitialMessages(ctx, convID, isNewConversation, message, skill)

	tools := s.getTools()
	totalTokens := 0
	for i := 0; i < 3; i++ {
		req := openai.ChatCompletionRequest{
			Model:    ModelGPT4oMini,
			Messages: messages,
			Tools:    tools,
			Stream:   true,
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
		}

		stream, err := s.openaiClient.CreateChatCompletionStream(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to create completion stream: %w", err)
		}

		var currentContent, currentToolId, currentToolName, currentToolArgs string
		var hasToolCalls bool

		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("agent stream error: %w", err)
			}

			if response.Usage != nil {
				totalTokens += response.Usage.TotalTokens
			}

			if len(response.Choices) == 0 {
				continue
			}

			delta := response.Choices[0].Delta
			if delta.Content != "" {
				currentContent += delta.Content
				onEvent(EventChunk, delta.Content)
			}

			if len(delta.ToolCalls) > 0 {
				hasToolCalls = true
				tc := delta.ToolCalls[0]
				if tc.ID != "" {
					currentToolId = tc.ID
				}
				if tc.Function.Name != "" {
					currentToolName = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					currentToolArgs += tc.Function.Arguments
				}
			}
		}
		stream.Close()

		if !hasToolCalls {
			if currentContent != "" {
				s.saveAssistantMessage(ctx, convID, currentContent)
			}
			break
		}

		// Handle tool calls
		messages = append(messages, openai.ChatCompletionMessage{
			Role: RoleAssistant,
			ToolCalls: []openai.ToolCall{
				{
					ID:   currentToolId,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      currentToolName,
						Arguments: currentToolArgs,
					},
				},
			},
		})

		switch currentToolName {
		case ToolExecuteGraphQL:
			onEvent(EventSystem, "Querying Threadify Engine...")

			var args struct {
				Query     string                 `json:"query"`
				Variables map[string]interface{} `json:"variables"`
			}
			if err := json.Unmarshal([]byte(currentToolArgs), &args); err != nil {
				s.logger.Error("failed to parse tool arguments",
					zap.String("tool", currentToolName),
					zap.String("raw_args", currentToolArgs),
					zap.Error(err),
				)
				return fmt.Errorf("tool args parsing error: %w (raw: %s)", err, currentToolArgs)
			}

			engineOutput, err := s.executeGraphQL(ctx, authHeader, args.Query, args.Variables)
			if err != nil {
				// Sanitize error - don't expose SQL or internal details
				sanitizedErr := s.sanitizeError(err)
				engineOutput = fmt.Sprintf("{\"error\": \"%s\"}", sanitizedErr)
			}

			messages = append(messages, openai.ChatCompletionMessage{
				Role:       RoleTool,
				Content:    engineOutput,
				ToolCallID: currentToolId,
			})
			s.saveToolMessages(ctx, convID, currentToolId, currentToolName, currentToolArgs, engineOutput)

			toolCallData, _ := json.Marshal(map[string]string{
				"query":    args.Query,
				"response": engineOutput,
			})
			onEvent(EventToolCall, string(toolCallData))
			continue

		case ToolSaveContext:
			var args struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.Unmarshal([]byte(currentToolArgs), &args); err != nil {
				return fmt.Errorf("tool args parsing error: %w", err)
			}
			if args.Key == "" || args.Value == "" {
				return fmt.Errorf("missing key or value for save_context")
			}

			_ = s.agentRepo.SaveContext(ctx, &models.AgentContext{
				ID:             uuid.New().String(),
				ConversationID: convID,
				ContextKey:     args.Key,
				ContextValue:   args.Value,
			})

			toolOutput := ToolStatusSuccess
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       RoleTool,
				Content:    toolOutput,
				ToolCallID: currentToolId,
			})
			s.saveToolMessages(ctx, convID, currentToolId, currentToolName, currentToolArgs, toolOutput)
			continue

		default:
			return fmt.Errorf("unknown tool: %s", currentToolName)
		}
	}

	// 8. Update conversation stats
	messageCount, tokenCount := s.updateStats(ctx, convID, totalTokens)

	// 8a. Record token usage in engine for billing
	if totalTokens > 0 {
		if err := s.recordUsage(ctx, authHeader, totalTokens); err != nil {
			// Log error but don't fail the request since AI response is already delivered
			s.logger.Error("failed to record token usage in engine", zap.Error(err))
		}
	}

	// 9. Send final metadata
	if tokenCount > 0 {
		onEvent("tokens", fmt.Sprintf("%d", tokenCount))
	}
	if messageCount > 0 {
		onEvent("message_count", fmt.Sprintf("%d", messageCount))
	}
	onEvent(EventConversation, convID)
	onEvent(EventDone, "true")

	return nil
}

// sanitizeError removes sensitive information from errors before showing to users
func (s *AgentService) sanitizeError(err error) string {
	errMsg := err.Error()

	if strings.Contains(errMsg, "GraphQL error") {
		// Extract just the user-facing part if possible
		if strings.Contains(errMsg, "status") {
			return "The query could not be completed. Please try again."
		}
	}

	if strings.Contains(errMsg, "payment required") || strings.Contains(errMsg, "insufficient credits") {
		return "Insufficient credits. Please top up your account to continue."
	}

	// For other errors, return a generic message but log the real error
	s.logger.Error("sanitized error for user", zap.Error(err))
	return "An error occurred while processing your request. Please try again or contact support."
}

func (s *AgentService) executeGraphQL(ctx context.Context, authHeader, query string, variables map[string]interface{}) (string, error) {
	reqBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.threadifyEngineURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set(HeaderAuthorization, authHeader)
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderUserAgent, UserAgentAPI)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusPaymentRequired || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w: %s", ErrPaymentRequired, string(respBody))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("GraphQL error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}

func (s *AgentService) CheckCredits(ctx context.Context, authHeader string) (bool, error) {
	resp, err := s.executeGraphQL(ctx, authHeader, queryCheckCredits, nil)
	if err != nil {
		return false, err
	}

	var gqlResp struct {
		Data struct {
			CheckCredits bool `json:"checkCredits"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp), &gqlResp); err != nil {
		return false, fmt.Errorf("failed to parse credit check response: %w", err)
	}

	return gqlResp.Data.CheckCredits, nil
}

func (s *AgentService) recordUsage(ctx context.Context, authHeader string, tokens int) error {
	_, err := s.executeGraphQL(ctx, authHeader, mutationRecordUsage, map[string]interface{}{"tokens": tokens})
	if err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}
	return nil
}

func (s *AgentService) buildInitialMessages(ctx context.Context, convID string, isNew bool, message string, skill string) []openai.ChatCompletionMessage {
	var roleDescription string
	switch skill {
	case SkillSupport:
		roleDescription = `You are a Customer Support AI for Threadify. Your goal is to help support teams quickly diagnose and resolve customer issues.

FOCUS:
- Find the root cause of customer-reported problems
- Identify failed steps and error messages
- Provide clear explanations for support agents
- Suggest next steps for resolution

RESPONSE STYLE:
- Customer-friendly language
- Clear problem identification
- Actionable troubleshooting steps`
	case SkillOperations:
		roleDescription = `You are an Operations AI for Threadify. Your goal is to monitor workflow execution and identify operational issues.

FOCUS:
- Workflow reliability and completion rates
- Error patterns and failure points
- Process bottlenecks and delays
- Retry patterns and recovery success

RESPONSE STYLE:
- Process-oriented and actionable
- Highlight anomalies and trends
- Include metrics and counts`
	case SkillBusiness:
		roleDescription = `You are a Business Intelligence AI for Threadify. Your goal is to provide insights and analytics on workflow performance.

FOCUS:
- Success rates and conversion metrics
- Workflow completion times
- Business process efficiency
- Trends and patterns over time

RESPONSE STYLE:
- Business-oriented language
- Quantitative insights
- Strategic recommendations`
	default:
		roleDescription = `You are Threadify's thread analyzer. Analyze execution threads using GraphQL queries.`
	}

	systemPrompt := openai.ChatCompletionMessage{
		Role:    RoleSystem,
		Content: roleDescription + "\n\n" + agentSystemPrompt,
	}

	messages := []openai.ChatCompletionMessage{systemPrompt}

	if !isNew {
		// Load context + history from repo
		contexts, _ := s.agentRepo.GetContext(ctx, convID)
		if len(contexts) > 0 {
			msg := "Previously saved context:\n"
			for _, c := range contexts {
				msg += fmt.Sprintf("- %s: %s\n", c.ContextKey, c.ContextValue)
			}
			messages = append(messages, openai.ChatCompletionMessage{Role: RoleSystem, Content: msg})
		}

		history, _ := s.agentRepo.GetMessages(ctx, convID)
		for _, m := range history {
			msg := openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}
			if m.ToolCallID != nil {
				msg.ToolCallID = *m.ToolCallID
			}
			if m.ToolCalls != nil {
				var calls []openai.ToolCall
				_ = json.Unmarshal([]byte(*m.ToolCalls), &calls)
				msg.ToolCalls = calls
			}
			messages = append(messages, msg)
		}
	}

	messages = append(messages, openai.ChatCompletionMessage{Role: RoleUser, Content: message})
	return messages
}

func (s *AgentService) getTools() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        ToolExecuteGraphQL,
				Description: "Execute a GraphQL query against the Threadify Engine to retrieve thread execution data",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {
							"type": "string",
							"description": "The GraphQL query string"
						},
						"variables": {
							"type": "object",
							"description": "Optional variables for the GraphQL query"
						}
					},
					"required": ["query"]
				}`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        ToolSaveContext,
				Description: "Save important context/summary for future reference in this conversation. Use this to remember key findings, thread IDs, or analysis results.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"key": {
							"type": "string",
							"description": "A short key describing what this context is (e.g., 'analyzed_thread', 'failed_threads_summary')"
						},
						"value": {
							"type": "string",
							"description": "The context value to save (e.g., thread ID, summary of findings)"
						}
					},
					"required": ["key", "value"]
				}`),
			},
		},
	}
}

func (s *AgentService) ensureConversationOwnership(ctx context.Context, companyID, convID string) error {
	convs, err := s.agentRepo.GetConversations(ctx, companyID)
	if err != nil {
		return fmt.Errorf("failed to verify conversation ownership: %w", err)
	}
	for _, c := range convs {
		if c.ID == convID {
			return nil
		}
	}
	return ErrForbidden
}

func (s *AgentService) saveToolMessages(ctx context.Context, convID, toolCallID, name, args, output string) {
	calls, _ := json.Marshal([]openai.ToolCall{{ID: toolCallID, Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: name, Arguments: args}}})
	callsStr := string(calls)

	_ = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID: uuid.New().String(), ConversationID: convID, Role: RoleAssistant, ToolCalls: &callsStr,
	})
	_ = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID: uuid.New().String(), ConversationID: convID, Role: RoleTool, Content: output, ToolCallID: &toolCallID,
	})
}

func (s *AgentService) saveAssistantMessage(ctx context.Context, convID, content string) {
	_ = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID: uuid.New().String(), ConversationID: convID, Role: RoleAssistant, Content: content, CreatedAt: time.Now(),
	})
}

func (s *AgentService) updateStats(ctx context.Context, convID string, newTokens int) (int, int) {
	msgCount, tokenCount, err := s.agentRepo.GetConversationStats(ctx, convID)
	if err != nil {
		return 0, 0
	}
	msgCount += 2
	tokenCount += newTokens
	_ = s.agentRepo.UpdateConversationStats(ctx, convID, msgCount, tokenCount)
	return msgCount, tokenCount
}

// Passthrough methods for conversation management
func (s *AgentService) GetConversations(ctx context.Context, companyID string) ([]models.AgentConversation, error) {
	return s.agentRepo.GetConversations(ctx, companyID)
}

func (s *AgentService) GetMessagesForUser(ctx context.Context, companyID, convID string) ([]*models.AgentMessage, error) {
	if err := s.ensureConversationOwnership(ctx, companyID, convID); err != nil {
		return nil, err
	}
	return s.agentRepo.GetMessages(ctx, convID)
}

func (s *AgentService) DeleteConversation(ctx context.Context, convID, userID string) error {
	return s.agentRepo.DeleteConversation(ctx, convID, userID)
}

func (s *AgentService) ContinueConversation(ctx context.Context, userID, companyID, parentConvID string) (string, string, string, error) {
	convs, err := s.agentRepo.GetConversations(ctx, companyID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load conversations: %w", err)
	}

	owns := false
	var parentTitle string
	for _, conv := range convs {
		if conv.ID == parentConvID {
			owns = true
			parentTitle = conv.Title
			break
		}
	}
	if !owns {
		return "", "", "", fmt.Errorf("not authorized to continue this conversation")
	}

	messages, err := s.agentRepo.GetMessages(ctx, parentConvID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load parent messages: %w", err)
	}

	var conversationHistory []openai.ChatCompletionMessage
	for _, msg := range messages {
		if msg.Role == RoleUser || (msg.Role == RoleAssistant && msg.Content != "") {
			conversationHistory = append(conversationHistory, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	var summary string
	if len(conversationHistory) > 0 {
		summaryReq := openai.ChatCompletionRequest{
			Model: ModelGPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    RoleSystem,
					Content: "You are a helpful assistant that creates concise summaries of conversations. Preserve all important facts, decisions, code snippets, thread IDs, technical details, and context. Be comprehensive but concise.",
				},
				{
					Role:    RoleUser,
					Content: "Summarize the following conversation, preserving all important context and details:",
				},
			},
			MaxTokens: s.summaryMaxTokens,
		}
		summaryReq.Messages = append(summaryReq.Messages, conversationHistory...)

		summaryResp, err := s.openaiClient.CreateChatCompletion(ctx, summaryReq)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to generate summary: %w", err)
		}
		summary = summaryResp.Choices[0].Message.Content
	}

	newConvID := uuid.New().String()
	newConv := &models.AgentConversation{
		ID:        newConvID,
		UserID:    userID,
		CompanyID: companyID,
		Title:     parentTitle + " (continued)",
	}

	if err := s.agentRepo.CreateConversationWithParent(ctx, newConv, parentConvID); err != nil {
		return "", "", "", fmt.Errorf("failed to create conversation: %w", err)
	}

	if summary != "" {
		summaryCtx := &models.AgentContext{
			ID:             uuid.New().String(),
			ConversationID: newConvID,
			ContextKey:     ContextKeySummary,
			ContextValue:   summary,
		}
		if err := s.agentRepo.SaveContext(ctx, summaryCtx); err != nil {
			s.logger.Error("failed to save summary context", zap.Error(err), zap.String("newConversationID", newConvID))
		}
	}

	return newConvID, newConv.Title, summary, nil
}
