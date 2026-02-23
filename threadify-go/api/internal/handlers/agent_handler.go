package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
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
}

func NewAgentHandler(threadifyEngineURL string, apiKey string, agentRepo *repository.AgentRepository) *AgentHandler {
	client := openai.NewClient(apiKey)

	return &AgentHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{},
		openaiClient:       client,
		agentRepo:          agentRepo,
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

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Threadify-AI-Agent/1.0")

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
	authHeader := c.GetHeader("Authorization")
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

	// Check conversation limits
	const maxMessages = 50
	const maxTokens = 200000

	if chatReq.ConversationID != "" {
		msgCount, tokenCount, err := h.agentRepo.GetConversationStats(chatReq.ConversationID)
		if err == nil {
			if msgCount >= maxMessages {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation has reached the maximum of 50 messages. Please start a new conversation."})
				return
			}
			if tokenCount >= maxTokens {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation has reached the token limit. Please start a new conversation."})
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
			log.Printf("Failed to create conversation: %v", err)
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
		log.Printf("Failed to save user message: %v", err)
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
}

type Thread {
  id: ID!
  contractName: String
  contractVersion: Int
  status: String!
  startedAt: String
  completedAt: String
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
✅ CORRECT: query { thread(id: "abc") { status } }
❌ WRONG: thread(id: "abc") { status }

EXAMPLES:
Q: "Analyze thread 5d724fa5"
1. Call: execute_graphql(query: 'query { thread(id: "5d724fa5") { status steps { stepName status } } }')
2. Respond: "Thread completed successfully with 15 steps across 3 phases: order_placed, payment_processed, shipment_dispatched."

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
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	// Token tracking
	totalTokens := 0

	// 2. The loop (LangGraph edge condition loop)
	for i := 0; i < 3; i++ { // limit recursion depth
		req := openai.ChatCompletionRequest{
			Model:    "gpt-4o-mini",
			Messages: messages,
			Tools:    tools,
			Stream:   true,
		}

		stream, err := h.openaiClient.CreateChatCompletionStream(context.Background(), req)
		if err != nil {
			log.Printf("[AGENT ERROR] %v", err)
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
				log.Printf("[AGENT ERROR] Stream error: %v", err)
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
					log.Printf("Tool args parsing error: %v", err)
					break
				}

				query, _ := args["query"].(string)
				variables, _ := args["variables"].(map[string]interface{})

				log.Printf("[AGENT] Executing GraphQL tool call for query: %s", query)

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

				// Save both messages to DB (assistant tool call + tool result)
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

				// Save AI tool invocation
				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "assistant",
					Content:        "",
					ToolCalls:      &tcStr,
				})

				// Save Tool Execution Result
				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "tool",
					Content:        engineOutput,
					ToolCallID:     &currentToolId,
				})

				// Loop continues! (jump to next llm call with data)
				continue
			} else if currentToolName == "save_context" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(currentToolArgs), &args); err != nil {
					log.Printf("Tool args parsing error: %v", err)
					break
				}

				key, _ := args["key"].(string)
				value, _ := args["value"].(string)

				log.Printf("[AGENT] Saving context: %s = %s", key, value)

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

				// Save tool call to DB
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
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this conversation"})
		return
	}

	msgs, err := h.agentRepo.GetMessages(convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": msgs})
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
