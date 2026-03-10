package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"

	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

const (
	eventStreamContentType = "text/event-stream"
	cacheControlNoCache    = "no-cache"
	connectionKeepAlive    = "keep-alive"
)

type ChatRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id"`
	Skill          string `json:"skill"` // support, operations, business
}

type AgentHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
	openaiClient       *openai.Client
	agentRepo          *repository.AgentRepository
	maxMessages        int
	maxTokens          int
	summaryMaxTokens   int
	logger             *zap.Logger
}

func NewAgentHandler(
	threadifyEngineURL string,
	apiKey string,
	agentRepo *repository.AgentRepository,
	maxMessages, maxTokens, summaryMaxTokens int,
	logger *zap.Logger,
) *AgentHandler {
	return &AgentHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		openaiClient:       openai.NewClient(apiKey),
		agentRepo:          agentRepo,
		maxMessages:        maxMessages,
		maxTokens:          maxTokens,
		summaryMaxTokens:   summaryMaxTokens,
		logger:             logger,
	}
}

// executeGraphQL calls the internal engine directly
func (h *AgentHandler) executeGraphQL(authHeader, query string, variables map[string]interface{}) (string, error) {
	reqBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", h.threadifyEngineURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set(service.HeaderAuthorization, authHeader)
	req.Header.Set(service.HeaderContentType, service.ContentTypeJSON)
	req.Header.Set(service.HeaderUserAgent, service.UserAgentAPI)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(respBody), nil
}

// Chat handles natural language queries related to thread analysis
func (h *AgentHandler) Chat(c *gin.Context) {
	authHeader := c.GetHeader(service.HeaderAuthorization)
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	if h.openaiClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI service is not configured"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, exists := c.Get("companyID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var chatReq ChatRequest
	if err := c.ShouldBindJSON(&chatReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body format"})
		return
	}

	// Check conversation limits (from config)
	if chatReq.ConversationID != "" {
		msgCount, tokenCount, err := h.agentRepo.GetConversationStats(chatReq.ConversationID)
		if err == nil {
			if msgCount >= h.maxMessages {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Conversation has reached the maximum of %d messages. Please start a new conversation.", h.maxMessages)})
				return
			}
			if tokenCount >= h.maxTokens {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Conversation has reached the token limit of %d. Please start a new conversation.", h.maxTokens)})
				return
			}
		}
	}

	convID := chatReq.ConversationID
	isNewConversation := false

	if convID == "" {
		convID = uuid.New().String()
		isNewConversation = true
		title := chatReq.Message
		if len(title) > 30 {
			title = title[:30] + "..."
		}
		err := h.agentRepo.CreateConversation(&models.AgentConversation{
			ID:        convID,
			UserID:    userID.(string),
			CompanyID: companyID.(string),
			Title:     title,
		})
		if err != nil {
			h.logger.Error("failed to create conversation", zap.Error(err))
		}
	} else {
		// verify existence + permission by loading messages
		// we should actually check if companyID matches
	}

	// Save the user's message
	err := h.agentRepo.AddMessage(&models.AgentMessage{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           "user",
		Content:        chatReq.Message,
	})
	if err != nil {
		h.logger.Error("failed to save user message", zap.Error(err))
	}

	// 1. Initial State (LangGraph pattern) -> Loading History vs Fresh
	// Build skill-specific system prompt
	var roleDescription string
	switch chatReq.Skill {
	case "support":
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
	case "operations":
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
	case "business":
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
		Role: openai.ChatMessageRoleSystem,
		Content: roleDescription + `

WORKFLOW:
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
- Use save_context to remember important findings`,
	}

	messages := []openai.ChatCompletionMessage{systemPrompt}

	// Load saved context for this conversation
	if !isNewConversation {
		contexts, err := h.agentRepo.GetContext(convID)
		if err == nil && len(contexts) > 0 {
			contextSummary := "Previously saved context:\n"
			for _, ctx := range contexts {
				contextSummary += fmt.Sprintf("- %s: %s\n", ctx.ContextKey, ctx.ContextValue)
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: contextSummary,
			})
		}

		history, err := h.agentRepo.GetMessages(convID)
		if err == nil {
			for _, m := range history {
				msg := openai.ChatCompletionMessage{
					Role:    m.Role,
					Content: m.Content,
				}
				if m.ToolCallID != nil && *m.ToolCallID != "" {
					msg.ToolCallID = *m.ToolCallID
				}
				if m.ToolCalls != nil && *m.ToolCalls != "" {
					var rawCalls []openai.ToolCall
					if err := json.Unmarshal([]byte(*m.ToolCalls), &rawCalls); err == nil {
						msg.ToolCalls = rawCalls
					}
				}
				messages = append(messages, msg)
			}
		}
	} else {
		// New conversation, just append current prompt
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: chatReq.Message,
		})
	}

	// Define the GraphQL execution tool based on LangGraph Tool Calling specification
	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "execute_graphql",
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
				Name:        "save_context",
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

	// SSE Header setup
	c.Writer.Header().Set(service.HeaderContentType, eventStreamContentType)
	c.Writer.Header().Set(service.HeaderCacheControl, cacheControlNoCache)
	c.Writer.Header().Set(service.HeaderConnection, connectionKeepAlive)

	// Token tracking
	totalTokens := 0

	// 2. The loop (LangGraph edge condition loop)
	for i := 0; i < 3; i++ { // limit recursion depth
		req := openai.ChatCompletionRequest{
			Model:    "gpt-4o-mini",
			Messages: messages,
			Tools:    tools,
			Stream:   true,
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
		}

		stream, err := h.openaiClient.CreateChatCompletionStream(context.Background(), req)
		if err != nil {
			h.logger.Error("failed to create chat completion stream", zap.Error(err))
			return
		}

		var currentContent, currentToolId, currentToolName, currentToolArgs string
		var hasToolCalls bool

		// Stream Processor
		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				h.logger.Error("agent stream error", zap.Error(err))
				break
			}

			// Track token usage
			if response.Usage != nil {
				totalTokens += response.Usage.TotalTokens
			}

			if len(response.Choices) == 0 {
				continue
			}

			delta := response.Choices[0].Delta

			// Stream text back to the client natively!
			if delta.Content != "" {
				currentContent += delta.Content
				c.SSEvent("chunk", delta.Content)
				c.Writer.Flush()
			}

			// Accumulate Tool Calls
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

		if hasToolCalls {
			// Add tool call to messages for next LLM iteration
			assistantToolMsg := openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
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
			}
			messages = append(messages, assistantToolMsg)

			// Inform user UI that data is being loaded behind the scenes
			c.SSEvent("system", "Querying Threadify Engine...")
			c.Writer.Flush()

			// Tool node layer executes logic
			if currentToolName == "execute_graphql" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(currentToolArgs), &args); err != nil {
					h.logger.Error("tool args parsing error", zap.Error(err))
					break
				}

				query, _ := args["query"].(string)
				variables, _ := args["variables"].(map[string]interface{})

				h.logger.Info("executing GraphQL tool call", zap.String("query", query))

				// Use original proxy
				engineOutput, err := h.executeGraphQL(authHeader, query, variables)
				if err != nil {
					engineOutput = fmt.Sprintf("{\"error\": \"%v\"}", err)
				}

				// Add tool result to messages
				toolResultMsg := openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    engineOutput,
					ToolCallID: currentToolId,
				}
				messages = append(messages, toolResultMsg)

				// Save tool call to DB for debugging/audit (hidden in UI)
				toolCallJSON, _ := json.Marshal([]openai.ToolCall{
					{
						ID:   currentToolId,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      currentToolName,
							Arguments: currentToolArgs,
						},
					},
				})
				tcStr := string(toolCallJSON)

				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "assistant",
					Content:        "",
					ToolCalls:      &tcStr,
				})

				// Save tool result to DB for debugging/audit (hidden in UI)
				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "tool",
					Content:        engineOutput,
					ToolCallID:     &currentToolId,
				})

				// Send tool call info to frontend via SSE for real-time "View Query" button
				toolCallData := map[string]string{
					"query":    query,
					"response": engineOutput,
				}
				toolCallDataJSON, _ := json.Marshal(toolCallData)
				c.SSEvent("tool_call", string(toolCallDataJSON))
				c.Writer.Flush()

				// Loop continues! (LLM will summarize in next iteration)
				continue
			} else if currentToolName == "save_context" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(currentToolArgs), &args); err != nil {
					h.logger.Error("tool args parsing error", zap.Error(err))
					break
				}

				key, _ := args["key"].(string)
				value, _ := args["value"].(string)

				if key == "" || value == "" {
					h.logger.Error("missing key or value")
					break
				}

				h.logger.Info("saving context", zap.String("key", key), zap.String("value", value))

				// Save context to DB
				_ = h.agentRepo.SaveContext(&models.AgentContext{
					ID:             uuid.New().String(),
					ConversationID: convID,
					ContextKey:     key,
					ContextValue:   value,
				})

				// Add tool result to messages
				toolResultMsg := openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    "Context saved successfully",
					ToolCallID: currentToolId,
				}
				messages = append(messages, toolResultMsg)

				// Save tool call to DB for debugging/audit (hidden in UI)
				toolCallJSON, _ := json.Marshal([]openai.ToolCall{
					{
						ID:   currentToolId,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      currentToolName,
							Arguments: currentToolArgs,
						},
					},
				})
				tcStr := string(toolCallJSON)

				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "assistant",
					Content:        "",
					ToolCalls:      &tcStr,
				})

				// Save tool result to DB for debugging/audit (hidden in UI)
				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "tool",
					Content:        "Context saved successfully",
					ToolCallID:     &currentToolId,
				})

				// Loop continues
				continue
			}
		}

		// Save any generated final text
		if currentContent != "" {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: currentContent,
			})

			_ = h.agentRepo.AddMessage(&models.AgentMessage{
				ID:             uuid.New().String(),
				ConversationID: convID,
				Role:           "assistant",
				Content:        currentContent,
				CreatedAt:      time.Now(),
			})
		}

		// If no tool calls, we're done
		if !hasToolCalls {
			break
		}
	}

	// Get current conversation stats
	messageCount, tokenCount, err := h.agentRepo.GetConversationStats(convID)
	if err == nil {
		// Increment message count (user message + assistant message)
		messageCount += 2

		// Add tokens from this request
		tokenCount += totalTokens

		// Update stats in database
		_ = h.agentRepo.UpdateConversationStats(convID, messageCount, tokenCount)

		// Send stats to frontend
		c.SSEvent("tokens", fmt.Sprintf("%d", tokenCount))
		c.SSEvent("message_count", fmt.Sprintf("%d", messageCount))
		c.Writer.Flush() // CRITICAL: Flush immediately for real-time updates
	}

	// Send final payload with conversation info
	c.SSEvent("conversation", convID)
	c.SSEvent("done", true)
	c.Writer.Flush()
}

// GetConversations lists recent conversations for a user
func (h *AgentHandler) GetConversations(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	convs, err := h.agentRepo.GetConversations(userID.(string))
	if err != nil {
		h.logger.Error("failed to load conversations", zap.Error(err), zap.String("userID", userID.(string)))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"conversations": convs})
}

// GetConversation returns a specific conversation history
func (h *AgentHandler) GetConversation(c *gin.Context) {
	convID := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Quick authorization - ensure user owns the conversation
	convs, err := h.agentRepo.GetConversations(userID.(string))
	if err != nil {
		h.logger.Error("failed to load conversations for authorization", zap.Error(err), zap.String("userID", userID.(string)))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversation history"})
		return
	}

	owns := false
	for _, conv := range convs {
		if conv.ID == convID {
			owns = true
			break
		}
	}

	if !owns {
		h.logger.Warn("user attempted to access unauthorized conversation", zap.String("userID", userID.(string)), zap.String("conversationID", convID))
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this conversation"})
		return
	}

	msgs, err := h.agentRepo.GetMessages(convID)
	if err != nil {
		h.logger.Error("failed to load messages for conversation", zap.Error(err), zap.String("conversationID", convID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

// ContinueConversation creates a new conversation with context from a parent conversation
func (h *AgentHandler) ContinueConversation(c *gin.Context) {
	parentConvID := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, exists := c.Get("companyID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Verify user owns the parent conversation
	convs, err := h.agentRepo.GetConversations(userID.(string))
	if err != nil {
		h.logger.Error("failed to load conversations for parent verification", zap.Error(err), zap.String("userID", userID.(string)))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversations"})
		return
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
		h.logger.Warn("user attempted to continue unauthorized conversation", zap.String("userID", userID.(string)), zap.String("parentConversationID", parentConvID))
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to continue this conversation"})
		return
	}

	// Get messages from parent conversation to generate summary
	messages, err := h.agentRepo.GetMessages(parentConvID)
	if err != nil {
		h.logger.Error("failed to load parent messages for summarization", zap.Error(err), zap.String("parentConversationID", parentConvID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load parent messages"})
		return
	}

	// Build conversation history for summarization
	var conversationHistory []openai.ChatCompletionMessage
	for _, msg := range messages {
		if msg.Role == "user" || (msg.Role == "assistant" && msg.Content != "") {
			conversationHistory = append(conversationHistory, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Generate summary using LLM
	var summary string
	if len(conversationHistory) > 0 {
		summaryReq := openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "system",
					Content: "You are a helpful assistant that creates concise summaries of conversations. Preserve all important facts, decisions, code snippets, thread IDs, technical details, and context. Be comprehensive but concise.",
				},
				{
					Role:    "user",
					Content: "Summarize the following conversation, preserving all important context and details:",
				},
			},
			MaxTokens: h.summaryMaxTokens,
		}
		summaryReq.Messages = append(summaryReq.Messages, conversationHistory...)

		summaryResp, err := h.openaiClient.CreateChatCompletion(context.Background(), summaryReq)
		if err != nil {
			h.logger.Error("failed to generate summary", zap.Error(err), zap.String("parentConversationID", parentConvID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate conversation summary"})
			return
		}
		summary = summaryResp.Choices[0].Message.Content
	}

	// Create new conversation with parent reference
	newConvID := uuid.New().String()
	newConv := &models.AgentConversation{
		ID:        newConvID,
		UserID:    userID.(string),
		CompanyID: companyID.(string),
		Title:     parentTitle + " (continued)",
	}

	err = h.agentRepo.CreateConversationWithParent(newConv, parentConvID)
	if err != nil {
		h.logger.Error("failed to create new conversation with parent", zap.Error(err), zap.String("parentConversationID", parentConvID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	// Store summary as context
	if summary != "" {
		summaryCtx := &models.AgentContext{
			ID:             uuid.New().String(),
			ConversationID: newConvID,
			ContextKey:     "conversation_summary",
			ContextValue:   summary,
		}
		if err := h.agentRepo.SaveContext(summaryCtx); err != nil {
			h.logger.Error("failed to save summary context", zap.Error(err), zap.String("newConversationID", newConvID))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation_id": newConvID,
		"title":           newConv.Title,
		"parent_id":       parentConvID,
	})
}

// DeleteConversation deletes a conversation and all its messages
func (h *AgentHandler) DeleteConversation(c *gin.Context) {
	convID := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := h.agentRepo.DeleteConversation(convID, userID.(string))
	if err != nil {
		if err.Error() == "conversation not found or not owned by user" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}
